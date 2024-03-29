package record

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sashabaranov/go-openai"

	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
	"MOSS_backend/utils/tools"

	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InferResponseModel struct {
	Status     int    `json:"status"` // 1 for output, 0 for end, -1 for error, -2 for sensitive
	StatusCode int    `json:"status_code,omitempty"`
	Output     string `json:"output,omitempty"`
	Stage      string `json:"stage,omitempty"`
}

type responseChannel struct {
	ch     chan InferResponseModel
	closed atomic.Bool
}

var InferResponseChannel sync.Map

var inferHttpClient = http.Client{Timeout: 120 * time.Second}

type InferWsContext struct {
	c                *websocket.Conn
	connectionClosed *atomic.Bool
}

func InferOpenAI(
	record *Record,
	postRecord RecordModels,
	model *ModelConfig,
	user *User,
	ctx *InferWsContext,
) (
	err error,
) {
	defer func() {
		if v := recover(); v != nil {
			Logger.Error("infer openai panicked", zap.Any("error", v))
			err = unknownError
		}
	}()

	openaiConfig := openai.DefaultConfig("")
	openaiConfig.BaseURL = model.Url
	client := openai.NewClientWithConfig(openaiConfig)

	var messages = make([]openai.ChatCompletionMessage, 0, len(postRecord)+2)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "system",
		Content: model.OpenAISystemPrompt,
	})
	messages = append(messages, postRecord.ToOpenAIMessages()...)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: record.Request,
	})
	request := openai.ChatCompletionRequest{
		Model:    model.OpenAIModelName,
		Messages: messages,
		Stop:     []string{model.EndDelimiter},
	}

	if ctx == nil {
		// openai client may panic when status code is 400
		response, err := client.CreateChatCompletion(
			context.Background(),
			request,
		)
		if err != nil {
			return err
		}

		if len(response.Choices) == 0 {
			return unknownError
		}

		record.Response = response.Choices[0].Message.Content
	} else {
		// streaming
		if config.Config.Debug {
			Logger.Info("openai streaming",
				zap.String("model", model.OpenAIModelName),
				zap.String("url", model.Url),
			)
		}

		stream, err := client.CreateChatCompletionStream(
			context.Background(),
			request,
		)
		if err != nil {
			return err
		}
		defer stream.Close()

		startTime := time.Now()

		var resultBuilder strings.Builder
		var nowOutput string
		var detectedOutput string

		for {
			if ctx.connectionClosed.Load() {
				return interruptError
			}
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}

			if len(response.Choices) == 0 {
				return unknownError
			}

			resultBuilder.WriteString(response.Choices[0].Delta.Content)
			nowOutput = resultBuilder.String()

			if model.EndDelimiter != "" && strings.Contains(nowOutput, model.EndDelimiter) {
				nowOutput = strings.Split(nowOutput, model.EndDelimiter)[0]
				break
			}

			before, _, found := CutLastAny(nowOutput, ",.?!\n，。？！")
			if !found || before == detectedOutput {
				continue
			}
			detectedOutput = before
			if model.EnableSensitiveCheck {
				err = sensitiveCheck(ctx.c, record, detectedOutput, startTime, user)
				if err != nil {
					return err
				}
			}

			_ = ctx.c.WriteJSON(InferResponseModel{
				Status: 1,
				Output: detectedOutput,
				Stage:  "MOSS",
			})
		}
		if nowOutput != detectedOutput {
			if model.EnableSensitiveCheck {
				err = sensitiveCheck(ctx.c, record, nowOutput, startTime, user)
				if err != nil {
					return err
				}
			}

			_ = ctx.c.WriteJSON(InferResponseModel{
				Status: 1,
				Output: nowOutput,
				Stage:  "MOSS",
			})
		}

		record.Response = nowOutput
		record.Duration = float64(time.Since(startTime)) / 1000_000_000
		_ = ctx.c.WriteJSON(InferResponseModel{
			Status: 0,
			Output: nowOutput,
			Stage:  "MOSS",
		})
	}

	return nil
}

func InferCommon(
	record *Record,
	prefix string,
	postRecords RecordModels,
	user *User,
	param map[string]float64,
	ctx *InferWsContext,
) (
	err error,
) {
	// metrics
	userInferRequestOnFlight.Inc()
	defer userInferRequestOnFlight.Dec()

	// load model config
	modelID := user.ModelID
	if modelID == 0 {
		modelID = config.Config.DefaultModelID
	}
	model, err := LoadModelConfigByID(user.ModelID)
	if err != nil {
		model, err = LoadModelConfigByID(config.Config.DefaultModelID)
		if err != nil {
			return err
		}
		modelID = config.Config.DefaultModelID
	}

	// dispatch
	if model.APIType == APITypeOpenAI {
		return InferOpenAI(record, postRecords, model, user, ctx)
	} else {
		return InferMOSS(record, prefix, user, model, param, ctx)
	}
}

func InferMOSS(
	record *Record,
	prefix string,
	user *User,
	model *ModelConfig,
	param map[string]float64,
	ctx *InferWsContext,
) (
	err error,
) {
	var (
		innerErr          error
		request           = map[string]any{}
		uuidText          string
		wg                sync.WaitGroup
		pluginConfig      = map[string]bool{}
		rawContentBuilder strings.Builder
	)

	if model == nil {
		return errors.New("model is nil")
	}

	// load params from db
	err = LoadParamToMap(request)
	if err != nil {
		return err
	}

	for key, value := range param {
		request[key] = value
	}

	// session_id
	request["session_id"] = record.ChatID

	// load user plugin config, if not exist, fill with default
	for key, value := range model.DefaultPluginConfig {
		if v, ok := user.PluginConfig[key]; ok {
			request[key] = v && value
			pluginConfig[key] = v && value
		} else {
			request[key] = false
			pluginConfig[key] = false
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

	if ctx != nil {
		uuidText = uuid.NewString()

		// start a new(fake) listener
		go func() {
			_ = inferListener(record, uuidText, user, *ctx, "Inner Thoughts")
		}()

		request["url"] = model.CallbackUrl + "?uuid=" + uuidText
	}

	// construct data to send
	data, _ := json.Marshal(request)
	inferTriggerResults, err := inferTrigger(data, model.Url) // block here
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
		if model.InnerThoughtsPostprocess {
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
	var secondFormattedInput string
	if results.Result == "None" {
		secondFormattedInput = fmt.Sprintf("<|Results|>: %s<eor>\n", results.Result)
	} else {
		secondFormattedInput = fmt.Sprintf("<|Results|>:\n%s<eor>\n", results.Result)
	}
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

		request["url"] = model.CallbackUrl + "?uuid=" + uuidText
	}

	// infer
	data, _ = json.Marshal(request)
	inferTriggerResults, err = inferTrigger(data, model.Url)
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

func Infer(
	record *Record,
	prefix string,
	postRecord RecordModels,
	user *User,
	param map[string]float64,
) (
	err error,
) {
	return InferCommon(
		record,
		prefix,
		postRecord,
		user,
		param,
		nil,
	)
}

func InferAsync(
	c *websocket.Conn,
	prefix string,
	record *Record,
	postRecord RecordModels,
	user *User,
	param map[string]float64,
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
		innerErr := InferCommon(
			record,
			prefix,
			postRecord,
			user,
			param,
			&InferWsContext{
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
	var inferListenerTimeLimit = 90 * time.Second
	var timer = time.NewTimer(inferListenerTimeLimit)

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
			timer.Reset(inferListenerTimeLimit)
			if config.Config.Debug {
				log.Println("receive response from output channel")
				log.Println(response)
			}
			// send only if stage "MOSS"
			if stage != "MOSS" {
				continue
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

				err = sensitiveCheck(ctx.c, record, detectedOutput, startTime, user)
				if err != nil {
					return err
				}

				_ = ctx.c.WriteJSON(InferResponseModel{
					Status: 1,
					Output: detectedOutput,
					Stage:  stage,
				})
			case 0: // end
				if nowOutput != detectedOutput {
					err = sensitiveCheck(ctx.c, record, nowOutput, startTime, user)
					if err != nil {
						return err
					}
					_ = ctx.c.WriteJSON(InferResponseModel{
						Status: 1,
						Output: nowOutput,
						Stage:  stage,
					})
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

func sensitiveCheck(
	c *websocket.Conn,
	record *Record,
	output string,
	startTime time.Time,
	user *User,
) error {
	if config.Config.Debug {
		Logger.Info("sensitive checking", zap.String("output", output))
	}

	if sensitive.IsSensitive(output, user) {
		record.ResponseSensitive = true
		// log new record
		record.Response = output
		record.Duration = float64(time.Since(startTime)) / 1000_000_000

		banned, err := user.AddUserOffense(UserOffenseMoss)
		if err != nil {
			return err
		}

		var outputMessage string
		if banned {
			outputMessage = OffenseMessage
		} else {
			outputMessage = DefaultResponse
		}

		_ = c.WriteJSON(InferResponseModel{
			Status: -2, // banned
			Output: outputMessage,
		})

		// if sensitive, jump out and record
		return ErrSensitive
	}
	return nil
}

type InferTriggerResponse struct {
	Output        string  `json:"output"`
	NewGeneration string  `json:"new_generation"`
	Duration      float64 `json:"duration"`
}

func inferTrigger(data []byte, inferUrl string) (i *InferTriggerResponse, err error) {

	var statusCode int
	// metrics
	inferOnFlightCounter.Inc()
	defer func() {
		inferOnFlightCounter.Dec()
		inferStatusCounter.WithLabelValues(strconv.Itoa(statusCode)).Inc()
	}()

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

	statusCode = rsp.StatusCode
	if rsp.StatusCode != 200 {
		inferLimiter.AddStats(false)
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
			inferLimiter.AddStats(false)
			responseString := string(response)
			if responseString == "400" {
				statusCode = 400
				return nil, maxInputExceededFromInferError
			} else if responseString == "560" {
				statusCode = 560
				return nil, unknownError
			} else {
				statusCode = 500
				Logger.Error(
					"unable to unmarshal response from infer",
					zap.ByteString("response", response),
					zap.Error(err),
				)
				return nil, InternalServerError()
			}
		} else {
			inferLimiter.AddStats(true)
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
			log.Printf("receive from infer error message type: %s\n, error: %v", string(message), err)
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
