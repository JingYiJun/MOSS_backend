package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
	"MOSS_backend/utils/tools"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InferResponseModel struct {
	Status     int    `json:"status"` // 1 for output, 0 for end, -1 for error, -2 for sensitive
	StatusCode int    `json:"status_code,omitempty"`
	Output     string `json:"output"`
	Stage      string `json:"stage,omitempty"`
}

type responseChannel struct {
	ch     chan InferResponseModel
	closed atomic.Bool
}

var InferResponseChannel sync.Map

var inferHttpClient = http.Client{Timeout: 1 * time.Minute}

type InferWsContext struct {
	c                *websocket.Conn
	connectionClosed *atomic.Bool
}

func InferCommon(
	record *Record,
	prefix string,
	user *User,
	ctx *InferWsContext,
) (
	err error,
) {
	var (
		innerErr                 error
		request                  = map[string]any{}
		uuidText                 string
		wg                       sync.WaitGroup
		inferUrl                 string
		innerThoughtsPostprocess bool
		defaultPluginConfig      map[string]bool
		pluginConfig             = map[string]bool{}
		configObject             Config
		rawContentBuilder        strings.Builder
	)

	// load config
	err = LoadConfig(&configObject)
	if err != nil {
		return err
	}
	inferUrl = configObject.ModelConfig[0].Url
	innerThoughtsPostprocess = configObject.ModelConfig[0].InnerThoughtsPostprocess
	defaultPluginConfig = configObject.ModelConfig[0].DefaultPluginConfig
	for i := range configObject.ModelConfig {
		if configObject.ModelConfig[i].ID == user.ModelID {
			inferUrl = configObject.ModelConfig[i].Url
			innerThoughtsPostprocess = configObject.ModelConfig[i].InnerThoughtsPostprocess
			defaultPluginConfig = configObject.ModelConfig[i].DefaultPluginConfig
			break
		}
	}

	// load params from db
	err = LoadParamToMap(request)
	if err != nil {
		return err
	}

	// load user plugin config, if not exist, fill with default
	for key, value := range defaultPluginConfig {
		if v, ok := user.PluginConfig[key]; ok {
			request[key] = v && value
			pluginConfig[key] = v && value
		} else {
			request[key] = value
			pluginConfig[key] = value
		}
	}

	// prefix replace
	cleanedPrefix := resultsRegexp.ReplaceAllString(prefix, "<|Results|>: None<eor>")

	/* first infer */
	// generate request
	input := mossSpecialTokenRegexp.ReplaceAllString(record.Request, " ") // replace special token
	firstFormattedInput := fmt.Sprintf("<|Human|>: %s<eoh>\n", input)
	request["x"] = fmt.Sprintf(
		"%s%s<|Inner Thoughts|>:",
		cleanedPrefix,
		firstFormattedInput, // <|Human|>: xxx<eoh>\n
	)

	// construct data to send
	data, _ := json.Marshal(request)
	inferTriggerResults, err := inferTrigger(data, inferUrl) // block here
	if err != nil {
		return err
	}

	/* middle process */
	// check if first output is valid
	firstFormattedNewGenerations := "<|Inner Thoughts|>:" + inferTriggerResults.NewGeneration
	if !firstGenerationsFormatRegexp.MatchString(firstFormattedNewGenerations) {
		Logger.Error(
			"error format first output",
			zap.String("new_generations", firstFormattedNewGenerations),
		)
		return unknownError
	}

	// replace invalid <|Commands|> <eo\w> to <eoc>
	commandsOutputSlice := commandsRegexp.FindStringSubmatch(firstFormattedNewGenerations)
	if len(commandsOutputSlice) != 3 {
		Logger.Error("error format first output", zap.String("new_generations", firstFormattedNewGenerations))
		return unknownError
	} else if commandsOutputSlice[2] != "<eoc>" { // replace <|Commands|> <eo\w> to <eoc>
		Logger.Error(
			"error <|Commands|> not end with <eoc>",
			zap.String("new_generations", firstFormattedNewGenerations),
		)
		firstFormattedNewGenerations = commandsRegexp.ReplaceAllString(firstFormattedNewGenerations, "<|Commands|>:$1<eoc>")
	}

	// replace invalid <|Inner Thoughts|> <eo\w> to <eot>
	innerThoughtsOutputSlice := innerThoughtsRegexp.FindStringSubmatch(firstFormattedNewGenerations)
	if len(innerThoughtsOutputSlice) != 3 {
		Logger.Error("error format first output", zap.String("new_generations", firstFormattedNewGenerations))
		return unknownError
	} else if innerThoughtsOutputSlice[2] != "<eot>" {
		Logger.Error(
			"error <|Inner Thoughts|> not end with <eot>",
			zap.String("new_generations", firstFormattedNewGenerations),
		)
		firstFormattedNewGenerations = innerThoughtsRegexp.ReplaceAllString(firstFormattedNewGenerations, "<|Inner Thoughts|>:$1<eot>")
	}
	// get first output Commands and InnerThoughts
	rawInnerThoughts := strings.Trim(innerThoughtsOutputSlice[1], " ")
	rawCommand := strings.Trim(commandsOutputSlice[1], " ")

	// get results from tools
	var results *tools.ResultTotalModel
	var newCommandString string
	if ctx != nil {
		results, newCommandString, err = tools.Execute(ctx.c, rawCommand, pluginConfig)
	} else {
		results, newCommandString, err = tools.Execute(nil, rawCommand, pluginConfig)
	}

	// invalid commands output => log & replace inner thoughts
	if err != nil {
		if errors.Is(err, tools.ErrInvalidCommandFormat) {
			Logger.Error(
				`error commands format`,
				zap.String("command", rawCommand),
			)
		}
		if innerThoughtsPostprocess {
			firstFormattedNewGenerations = innerThoughtsRegexp.ReplaceAllString(firstFormattedNewGenerations, "<|Inner Thoughts|>: None<eot>")
			rawInnerThoughts = "None"
		}
	}
	// valid/invalid commands output replace <|Commands|>
	firstFormattedNewGenerations = commandsRegexp.ReplaceAllString(
		firstFormattedNewGenerations,
		"<|Commands|>: "+newCommandString+"<eoc>",
	) // there is a space after colon

	if ctx != nil && ctx.connectionClosed.Load() {
		return interruptError
	}

	/* second infer */

	// generate new formatted text and uuid
	uuidText = strings.ReplaceAll(uuid.NewString(), "-", "")
	secondFormattedInput := fmt.Sprintf("<|Results|>: %s<eor>\n", results.Result)
	request["x"] = fmt.Sprintf(
		"%s%s%s\n%s<|MOSS|>:",
		cleanedPrefix,                // context
		firstFormattedInput,          // <|Human|>: xxx<eoh>\n
		firstFormattedNewGenerations, // <|Inner Thoughts|>: xxx<eot>\n<|Commands|>: xxx<eoc>
		secondFormattedInput,         // <|Results|>: xxx<eor>\n
	)

	if ctx != nil {
		wg.Add(1)
		// start a new listener
		go func() {
			innerErr = inferListener(record, uuidText, user, *ctx, "MOSS")
			wg.Done()
		}()

		request["url"] = config.Config.CallbackUrl + "?uuid=" + uuidText
	}

	// infer
	data, _ = json.Marshal(request)
	inferTriggerResults, err = inferTrigger(data, inferUrl)
	if err != nil {
		return err
	}

	if ctx != nil {
		wg.Wait()
		if innerErr != nil {
			return innerErr
		}
		if ctx.connectionClosed.Load() {
			return interruptError
		}
	}

	// second output check format
	secondFormattedNewGenerations := "<|MOSS|>:" + inferTriggerResults.NewGeneration
	if !secondGenerationsFormatRegexp.MatchString(secondFormattedNewGenerations) {
		Logger.Error(
			"error format second output",
			zap.String("new_generations", secondFormattedNewGenerations),
		)
		return unknownError
	}

	// replace invalid <|MOSS|> <eo\w> to <eom>
	mossOutputSlice := mossRegexp.FindStringSubmatch(secondFormattedNewGenerations)
	if len(mossOutputSlice) != 3 {
		Logger.Error("error format second output", zap.String("new_generations", secondFormattedNewGenerations))
		return unknownError
	} else if mossOutputSlice[2] != "<eom>" {
		Logger.Error(
			"error <|MOSS|> not end with <eom>",
			zap.String("new_generations", secondFormattedNewGenerations),
		)
		secondFormattedNewGenerations = mossRegexp.ReplaceAllString(secondFormattedNewGenerations, "<|MOSS|>:$1<eom>")
	}

	// save to record
	record.Prefix = inferTriggerResults.Output + "\n" // save record prefix for next inference
	record.Response = strings.Trim(mossOutputSlice[1], " ")
	record.Duration = inferTriggerResults.Duration
	record.ExtraData = results.ExtraData
	record.ProcessedExtraData = results.ProcessedExtraData
	record.InnerThoughts = rawInnerThoughts

	rawContentBuilder.WriteString(firstFormattedInput)
	rawContentBuilder.WriteString(firstFormattedNewGenerations)
	rawContentBuilder.WriteString("\n")
	rawContentBuilder.WriteString(secondFormattedInput)
	rawContentBuilder.WriteString(secondFormattedNewGenerations)
	rawContentBuilder.WriteString("\n")
	record.RawContent = rawContentBuilder.String()

	// end
	if ctx != nil {
		err = ctx.c.WriteJSON(InferResponseModel{Status: 0})
		if err != nil {
			return fmt.Errorf("write end status error: %v", err)
		}
	}
	return nil
}

func Infer(record *Record, prefix string, user *User) (err error) {
	return InferCommon(record, prefix, user, nil)
}

func InferAsync(
	c *websocket.Conn,
	prefix string,
	record *Record,
	user *User,
) (
	err error,
) {
	var (
		interruptChan    = make(chan any)   // frontend interrupt channel
		connectionClosed = new(atomic.Bool) // connection closed flag
		errChan          = make(chan error) // error transmission channel
		successChan      = make(chan any)   // success infer flag
	)
	connectionClosed.Store(false)      // initialize
	defer connectionClosed.Store(true) // if this closed, stop all goroutines

	// wait for interrupt
	go interrupt(
		c,
		interruptChan,
		connectionClosed,
	)

	// wait for infer
	go func() {
		innerErr := inferLogicPath(
			record,
			prefix,
			user,
			InferWsContext{
				c:                c,
				connectionClosed: connectionClosed,
			},
		)
		if innerErr != nil {
			errChan <- innerErr
		} else {
			close(successChan)
		}
	}()

	for {
		select {
		case <-interruptChan:
			return NoStatus("client interrupt")
		case err = <-errChan:
			return err
		case <-successChan:
			return nil
		}
	}
}

// inferLogicPath hand out inference tasks
func inferLogicPath(
	record *Record,
	prefix string,
	user *User,
	ctx InferWsContext,
) error {
	return InferCommon(record, prefix, user, &ctx)
}

// inferListener listen from output channel
func inferListener(
	record *Record,
	uuidText string,
	user *User,
	ctx InferWsContext,
	stage string,
) error {
	var err error

	// make store channel into map
	outputChan := make(chan InferResponseModel, 100)
	responseCh := &responseChannel{ch: outputChan}
	InferResponseChannel.Store(uuidText, responseCh)
	defer func() {
		InferResponseChannel.Delete(uuidText)
	}()

	startTime := time.Now()

	var timer = time.NewTimer(11 * time.Second)

	var nowOutput string
	var detectedOutput string

	for {
		if ctx.connectionClosed.Load() {
			return nil
		}
		if responseCh.closed.Load() {
			return InternalServerError()
		}
		select {
		case response := <-outputChan:
			timer.Reset(11 * time.Second)
			if config.Config.Debug {
				log.Println("receive response from output channel")
				log.Println(response)
			}
			switch response.Status {
			case 1: // ok
				if config.Config.Debug {
					log.Printf("receive response from output channal: %v\nsensitive checking\n", response.Output)
				}

				nowOutput = response.Output
				before, _, found := CutLastAny(nowOutput, ",.?!\n，。？！")
				if !found || before == detectedOutput {
					continue
				}
				detectedOutput = before

				// output sensitive check
				if sensitive.IsSensitive(detectedOutput, user) {
					record.ResponseSensitive = true
					// log new record
					record.Response = detectedOutput
					record.Duration = float64(time.Since(startTime)) / 1000_000_000
					var banned bool
					banned, err = user.AddUserOffense(UserOffenseMoss)
					if err != nil {
						return err
					}
					if banned {
						err = ctx.c.WriteJSON(InferResponseModel{
							Status: -2, // banned
							Output: OffenseMessage,
						})
					} else {
						err = ctx.c.WriteJSON(InferResponseModel{
							Status: -2, // sensitive
							Output: DefaultResponse,
						})
					}
					if err != nil {
						return fmt.Errorf("write sensitive error: %v", err)
					}

					// if sensitive, jump out and record
					return ErrSensitive
				}

				err = ctx.c.WriteJSON(InferResponseModel{
					Status: 1,
					Output: detectedOutput,
					Stage:  stage,
				})
				if err != nil {
					return fmt.Errorf("write response error: %v", err)
				}
			case 0: // end
				if nowOutput != detectedOutput {
					if sensitive.IsSensitive(nowOutput, user) {
						record.ResponseSensitive = true
						// log new record
						record.Response = nowOutput
						record.Duration = float64(time.Since(startTime)) / 1000_000_000
						var banned bool
						banned, err = user.AddUserOffense(UserOffenseMoss)
						if err != nil {
							return err
						}
						if banned {
							err = ctx.c.WriteJSON(InferResponseModel{
								Status: -2, // banned
								Output: OffenseMessage,
							})
						} else {
							err = ctx.c.WriteJSON(InferResponseModel{
								Status: -2, // sensitive
								Output: DefaultResponse,
							})
						}
						if err != nil {
							return fmt.Errorf("write sensitive error: %v", err)
						}

						// if sensitive, jump out and record
						return ErrSensitive
					}
				}
				err = ctx.c.WriteJSON(InferResponseModel{
					Status: 1,
					Output: nowOutput,
					Stage:  stage,
				})
				if err != nil {
					return fmt.Errorf("write response error: %v", err)
				}
				return nil
			case -1: // error
				return InternalServerError(response.Output)
			}
		case <-timer.C:
			return InternalServerError("Internal Server Timeout")
		}
	}
}

type InferTriggerResponse struct {
	Output        string  `json:"output"`
	NewGeneration string  `json:"new_generation"`
	Duration      float64 `json:"duration"`
}

func inferTrigger(data []byte, inferUrl string) (i *InferTriggerResponse, err error) {
	startTime := time.Now()
	rsp, err := inferHttpClient.Post(inferUrl, "application/json", bytes.NewBuffer(data)) // take the ownership of data
	if err != nil {
		Logger.Error(
			"post inference error",
			zap.Error(err),
		)
		return nil, InternalServerError("inference server error")
	}

	defer func() {
		_ = rsp.Body.Close()
	}()

	response, err := io.ReadAll(rsp.Body)
	if err != nil {
		Logger.Error("fail to read response body", zap.Error(err))
		return nil, InternalServerError()
	}

	latency := int(time.Since(startTime))
	duration := float64(latency) / 1000_000_000

	// add stats
	defer inferLimiter.AddStats(err == nil)

	if rsp.StatusCode != 200 {
		Logger.Error(
			"inference error",
			zap.Int("latency", latency),
			zap.Int("status code", rsp.StatusCode),
			zap.ByteString("body", response),
		)
		if rsp.StatusCode == 400 {
			return nil, maxInputExceededFromInferError
		} else if rsp.StatusCode == 560 {
			return nil, unknownError
		} else if rsp.StatusCode >= 500 {
			return nil, InternalServerError()
		} else {
			return nil, unknownError
		}
	} else {
		var responseStruct struct {
			Pred                   string `json:"pred"`
			NewGenerations         string `json:"new_generations"`
			InputTokenNum          int    `json:"input_token_num"`
			NewGenerationsTokenNum int    `json:"new_generations_token_num"`
		}
		err = json.Unmarshal(response, &responseStruct)
		if err != nil {
			responseString := string(response)
			if responseString == "400" {
				return nil, maxInputExceededFromInferError
			} else if responseString == "560" {
				return nil, unknownError
			} else {
				Logger.Error(
					"unable to unmarshal response from infer",
					zap.ByteString("response", response),
					zap.Error(err),
				)
			}
			return nil, InternalServerError()
		} else {
			Logger.Info(
				"inference success",
				zap.ByteString("request", data),
				zap.Int("latency", latency),
				zap.String("pred", responseStruct.Pred),
				zap.String("new_generations", responseStruct.NewGenerations),
				zap.Int("input_token_num", responseStruct.InputTokenNum),
				zap.Int("new_generations_token_num", responseStruct.NewGenerationsTokenNum),
				zap.Float64("average", float64(latency)/float64(responseStruct.NewGenerationsTokenNum)),
			)
			return &InferTriggerResponse{
				Output:        responseStruct.Pred,
				NewGeneration: responseStruct.NewGenerations,
				Duration:      duration,
			}, nil
		}
	}
}

func ReceiveInferResponse(c *websocket.Conn) {
	var (
		message []byte
		err     error
	)

	defer func() {
		if something := recover(); something != nil {
			Logger.Error("receive infer response panicked", zap.Any("error", something))
		}
	}()

	uuidText := c.Query("uuid")
	if uuidText == "" {
		_ = c.WriteJSON(InferResponseModel{Status: -1, StatusCode: 400, Output: "Bad Request"})
		return
	}

	value, ok := InferResponseChannel.Load(uuidText)
	if !ok {
		Logger.Error("receive from infer invalid uuid", zap.String("uuid", uuidText))
		_ = c.WriteJSON(InferResponseModel{Status: -1, StatusCode: 400, Output: "Bad Request"})
		return
	}
	ch := value.(*responseChannel)

	for {
		if _, message, err = c.ReadMessage(); err != nil {
			if ch.closed.Load() {
				_ = c.WriteJSON(InferResponseModel{Status: 0})
				return
			} else {
				Logger.Error("receive from infer error", zap.Error(err))
				ch.closed.Store(true)
			}
			return
		}
		if ch.closed.Load() {
			_ = c.WriteJSON(InferResponseModel{Status: 0})
			return
		}

		if config.Config.Debug {
			log.Printf("receive message from inference, uuid: %v: %s\n", uuidText, string(message))
		}

		var inferResponse InferResponseModel
		err = json.Unmarshal(message, &inferResponse)
		if err != nil {
			log.Printf("receive from infer error message type: %s\n, error: %s", string(message), err)
			continue
		}

		// continue if sending a heartbeat package
		if inferResponse.Status == 2 {
			continue
		}

		// post process
		inferResponse.Output = InferPostprocess(inferResponse.Output)

		if config.Config.Debug {
			log.Printf("recieve output: %v\n", inferResponse.Output)
		}

		// may panic
		ch.ch <- inferResponse

		if inferResponse.Status == 0 {
			_ = c.WriteJSON(InferResponseModel{Status: 0})
			return
		}
	}
}

func InferPostprocess(output string) (tidyOutput string) {
	// process end with 0xfffd
	runeSlice := []rune(output)
	for len(runeSlice) > 0 && runeSlice[len(runeSlice)-1] == 0xfffd {
		runeSlice = runeSlice[:len(runeSlice)-1]
	}

	output = strings.Trim(string(runeSlice), " ")

	output = cutSuffix(output, "<", "<e", "<eo", "<eom", "<eom>")

	// not cut
	return output
}

func cutSuffix(s string, suffix ...string) string {
	for _, suf := range suffix {
		s, _ = strings.CutSuffix(s, suf)
	}
	return s
}
