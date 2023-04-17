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
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var endContentRegexp = regexp.MustCompile(`<[es]o\w>`)

var resultsRegexp = regexp.MustCompile(`<[|]Results[|]>:[\s\S]+?<eor>`) // not greedy

var commandsRegexp = regexp.MustCompile(`<\|Commands\|>:([\s\S]+?)<eo\w>`)

var mossRegexp = regexp.MustCompile(`<\|MOSS\|>:([\s\S]+?)<eo\w>`)

var innerThoughtsRegexp = regexp.MustCompile(`<\|Inner Thoughts\|>:([\s\S]+?)<eo\w>`)

//var maxLengthExceededError = BadRequest("The maximum context length is exceeded").WithMessageType(MaxLength)

var maxInputExceededError = BadRequest("单次输入限长为 1000 字符。Input no more than 1000 characters").WithMessageType(MaxLength)

var maxInputExceededFromInferError = BadRequest("单次输入超长，请减少字数并重试。Input max length exceeded, please reduce length and try again").WithMessageType(MaxLength)

var unknownError = InternalServerError("未知错误，请刷新或等待一分钟后再试。Unknown error, please refresh or wait a minute and try again")

var sensitiveError = errors.New("sensitive")

var interruptError = NoStatus("client interrupt")

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
		wg1                      sync.WaitGroup
		wg2                      sync.WaitGroup
		inferUrl                 string
		innerThoughtsPostprocess bool
		configObject             Config
	)
	err = LoadConfig(&configObject)
	if err != nil {
		return err
	}
	inferUrl = configObject.ModelConfig[0].Url
	innerThoughtsPostprocess = configObject.ModelConfig[0].InnerThoughtsPostprocess
	for i := range configObject.ModelConfig {
		if configObject.ModelConfig[i].ID == user.ModelID {
			inferUrl = configObject.ModelConfig[i].Url
			innerThoughtsPostprocess = configObject.ModelConfig[i].InnerThoughtsPostprocess
			break
		}
	}

	// load params from db
	err = LoadParamToMap(request)
	if err != nil {
		return err
	}

	// load user plugin config
	for key, value := range user.PluginConfig {
		request[key] = value
	}

	cleanedPrefix := resultsRegexp.ReplaceAllString(prefix, "<|Results|>: None<eor>")

	/* first infer */
	// generate request
	uuidText = strings.ReplaceAll(uuid.NewString(), "-", "")
	formattedText := InferPreprocess(record.Request, cleanedPrefix)
	request["x"] = formattedText

	if ctx != nil {
		wg1.Add(1)
		// start a listener
		go func() {
			innerErr = inferListener(
				record,
				uuidText,
				user,
				*ctx,
				"Inner Thoughts",
			)
			wg1.Done()
		}()

		request["url"] = config.Config.CallbackUrl + "?uuid=" + uuidText
	}

	// construct data to send
	data, _ := json.Marshal(request)
	output, duration, err := inferTrigger(data, inferUrl) // block here

	if ctx != nil {
		wg1.Wait()
		if innerErr != nil {
			return innerErr
		}
		if ctx.connectionClosed.Load() {
			return interruptError
		}
	}

	// middle process
	firstOutput := strings.Trim(output, " \t\n")

	humanIndex := strings.LastIndex(firstOutput, "<|Human|>:")
	if humanIndex == -1 {
		Logger.Error(`error find "<|Human|>:"`, zap.String("output", firstOutput))
		return InternalServerError()
	}
	firstRawOutput := firstOutput[humanIndex:]
	subsetIndex := commandsRegexp.FindStringSubmatchIndex(firstRawOutput)
	// subsetIndex: [CommandsStructStartIndex CommandsStructEndIndex, CommandsContentStartIndex, CommandsContentEndIndex]

	if len(subsetIndex) < 4 {
		Logger.Error(`error find "<|Commands|>:"`, zap.String("output", firstOutput))
		return InternalServerError()
	}

	firstRawOutput = firstRawOutput[:subsetIndex[1]]
	commandContent := strings.Trim(firstRawOutput[subsetIndex[2]:subsetIndex[3]], " \n")

	// replace <|Commands|> <eo\w> to <eoc>
	if !strings.HasSuffix(firstRawOutput, "<eoc>") {
		Logger.Error(
			"error <|Commands|> not end with <eoc>",
			zap.String("output", firstOutput),
			zap.String("raw_output", firstRawOutput),
		)
	}
	firstRawOutput = commandsRegexp.ReplaceAllString(firstRawOutput, "<|Commands|>:$1<eoc>")

	// get results from tools
	var results *tools.ResultTotalModel
	if ctx != nil {
		results, err = tools.Execute(ctx.c, commandContent)
	} else {
		results, err = tools.Execute(nil, commandContent)
	}

	// replace invalid commands output
	if errors.Is(err, tools.ErrInvalidCommandFormat) {
		Logger.Error(
			`error commands format`,
			zap.String("command", commandContent),
		)
		firstRawOutput = commandsRegexp.ReplaceAllString(firstRawOutput, "<|Commands|>: None<eoc>")
		if innerThoughtsPostprocess {
			firstRawOutput = innerThoughtsRegexp.ReplaceAllString(firstRawOutput, "<|Inner Thoughts|>: None<eot>")
		}
	}

	if ctx != nil && ctx.connectionClosed.Load() {
		return interruptError
	}

	/* second infer */

	// generate new formatted text and uuid
	uuidText = strings.ReplaceAll(uuid.NewString(), "-", "")
	formattedText = InferWriteResult(results.Result, cleanedPrefix+firstRawOutput+"\n")
	request["x"] = formattedText

	if ctx != nil {
		wg2.Add(1)
		// start a new listener
		go func() {
			innerErr = inferListener(
				record,
				uuidText,
				user,
				*ctx,
				"MOSS",
			)
			wg2.Done()
		}()

		request["url"] = config.Config.CallbackUrl + "?uuid=" + uuidText
	}

	// infer
	data, _ = json.Marshal(request)
	secondOutput, duration, err := inferTrigger(data, inferUrl)
	if err != nil {
		return err
	}

	if ctx != nil {
		wg2.Wait()
		if innerErr != nil {
			return innerErr
		}
		if ctx.connectionClosed.Load() {
			return interruptError
		}
	}

	// cut out this turn
	humanIndex = strings.LastIndex(secondOutput, "<|Human|>:")
	if humanIndex == -1 {
		Logger.Error(`error find "<|Human|>:"`, zap.String("output", secondOutput))
		return InternalServerError()
	}
	secondRawOutput := secondOutput[humanIndex:]

	// get moss output
	mossOutputSlice := mossRegexp.FindStringSubmatch(secondRawOutput)
	if len(mossOutputSlice) < 2 {
		Logger.Error(`error find "<|MOSS|>:"`, zap.String("output", secondOutput))
		return InternalServerError()
	}

	// save to record
	record.Prefix = secondOutput + "\n" // save record prefix for next inference
	record.Response = strings.Trim(mossOutputSlice[1], " ")
	record.Duration = duration
	record.ExtraData = results.ExtraData
	record.ProcessedExtraData = results.ProcessedExtraData
	record.RawContent = secondRawOutput

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
					return sensitiveError
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
						return sensitiveError
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

func inferTrigger(data []byte, inferUrl string) (string, float64, error) {
	startTime := time.Now()
	rsp, err := inferHttpClient.Post(inferUrl, "application/json", bytes.NewBuffer(data)) // take the ownership of data
	if err != nil {
		Logger.Error(
			"post inference error",
			zap.Error(err),
		)
		return "", 0, InternalServerError("inference server error")
	}

	defer func() {
		_ = rsp.Body.Close()
	}()

	response, err := io.ReadAll(rsp.Body)
	if err != nil {
		Logger.Error("fail to read response body", zap.Error(err))
		return "", 0, InternalServerError()
	}

	latency := int(time.Since(startTime))
	duration := float64(latency) / 1000_000_000

	if rsp.StatusCode != 200 {
		Logger.Error(
			"inference error",
			zap.Int("latency", latency),
			zap.Int("status code", rsp.StatusCode),
			zap.ByteString("body", response),
		)
		if rsp.StatusCode == 400 {
			return "", duration, maxInputExceededFromInferError
		} else if rsp.StatusCode == 560 {
			return "", duration, unknownError
		} else if rsp.StatusCode >= 500 {
			return "", duration, InternalServerError()
		} else {
			return "", duration, unknownError
		}
	} else {
		var responseStruct struct {
			Pred                   string `json:"pred"`
			InputTokenNum          int    `json:"input_token_num"`
			NewGenerationsTokenNum int    `json:"new_generations_token_num"`
		}
		err = json.Unmarshal(response, &responseStruct)
		if err != nil {
			responseString := string(response)
			if responseString == "400" {
				return "", duration, maxInputExceededFromInferError
			} else if responseString == "560" {
				return "", duration, unknownError
			} else {
				Logger.Error(
					"unable to unmarshal response from infer",
					zap.ByteString("response", response),
					zap.Error(err),
				)
			}
			return "", duration, InternalServerError()
		} else {
			Logger.Info(
				"inference success",
				zap.ByteString("request", data),
				zap.Int("latency", latency),
				zap.String("pred", responseStruct.Pred),
				zap.Int("input_token_num", responseStruct.InputTokenNum),
				zap.Int("new_generations_token_num", responseStruct.NewGenerationsTokenNum),
				zap.Float64("average", float64(latency)/float64(responseStruct.NewGenerationsTokenNum)),
			)
			return responseStruct.Pred, duration, nil
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

func InferPreprocess(input, prefix string) (formattedText string) {
	return prefix + fmt.Sprintf("<|Human|>: %s<eoh>\n", input)
}

func InferWriteResult(results, prefix string) string {
	return prefix + fmt.Sprintf("<|Results|>: %s<eor>\n<|MOSS|>:", results)
}

func InferPostprocess(output string) (tidyOutput string) {
	// process end with 0xfffd
	runeSlice := []rune(output)
	for len(runeSlice) > 0 && runeSlice[len(runeSlice)-1] == 0xfffd {
		runeSlice = runeSlice[:len(runeSlice)-1]
	}

	// not cut
	return output
}
