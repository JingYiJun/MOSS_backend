package record

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slices"

	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"

	"github.com/gofiber/websocket/v2"
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
	c                JsonReaderWriter
	connectionClosed *atomic.Bool
}

var UrlConfig struct {
	pythonUrl string
	retrievalUrl string
	documentUrl string
	bingapiUrl string
}


func InferMoss2(
	record *Record,
	postRecord RecordModels,
	model *ModelConfig,
	user *User,
	ctx *InferWsContext,
) (
	err error,
) {
	//TODO init UrlConfig in main
	defer func() {
		if v := recover(); v != nil {
			Logger.Error("infer moss2 panicked", zap.Any("error", v))
			err = unknownError
		}
	}()
	// Moss2 adopts the same format as OpenAI
	moss2Config := openai.DefaultConfig("")
	moss2Config.BaseURL = model.Url
	client := openai.NewClientWithConfig(moss2Config)

	var messages = make([]openai.ChatCompletionMessage, 0, len(postRecord) + 2)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:   "system",
		Content: model.OpenAISystemPrompt,
	})
	messages = append(messages, postRecord.ToOpenAIMessages()...)
	messages = append(messages, openai.ChatCompletionMessage{
		Role: 	"user",
		Content: record.Request,
	})
	request := openai.ChatCompletionRequest{
		Model:		model.OpenAIModelName,
		Messages: 	messages,
		Stop:		model.EndDelimiter,
	}

	if ctx == nil {
		// TODO: when will ctx become nil and how to handle the situation
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
		// streaming, Done
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

			if slices.Contains(model.EndDelimiter, MossEnd) && strings.Contains(nowOutput, MossEnd) {
				// if MossEnd is found, break the loop
				nowOutput = strings.Split(nowOutput, MossEnd)[0]
				break
			}

			if slices.Contains(model.EndDelimiter, FuncCallEnd) && strings.Contains(nowOutput, FuncCallEnd) {
				// if FuncCallEnd is found, call tool apis
				var funcCallResult string
				
				err = GetFuncCallResult(nowOutput, &funcCallResult)
				if err != nil {
					return err
				}
				
				// TODO: Do we need this? or ignore <im_start>func_ret and simply send the funcCallResult
				message := openai.ChatCompletionMessage{
					Role: "func_ret",
					Content: funcCallResult,
				}
				
				request = openai.ChatCompletionRequest{
					Model: model.OpenAIModelName,
					Messages: []openai.ChatCompletionMessage{message},
					Stop:		model.EndDelimiter,
				}
				// close old stream
				stream.Close()
				stream, err = client.CreateChatCompletionStream(
					context.Background(),
					request,
				)
				if err != nil {
					return err
				}
				// erase the content of fun_call
				nowOutput = strings.Split(nowOutput, FuncCallStart)[0]
				resultBuilder.Reset()
				resultBuilder.WriteString(nowOutput)
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
				Stage: "MOSS",
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
				Stage: "MOSS",
			})
		}
	
		record.Response = nowOutput
		record.Duration = float64(time.Since(startTime)) / 1000_000_000
		_ = ctx.c.WriteJSON(InferResponseModel{
			Status: 0,
			Output: nowOutput,
			Stage: "MOSS",
		})

	}
	return nil
}

func GetFuncCallResult(
	nowOutput string,
	result *string,
)(
	err error,
) {
	defer func() {
		if v := recover(); v != nil {
			Logger.Error("infer funcCall panicked", zap.Any("error", v))
			err = unknownError
		}
	}()

	funcCallMatches := funcCallRegexp.FindStringSubmatch(nowOutput)
	if len(funcCallMatches) == 0 {
		return errors.New("no code found")
	}
	
	// get the last match funcCall
	lastCodeStr := funcCallMatches[len(funcCallMatches) - 1]
	var data map[string]interface{}
	err = json.Unmarshal([]byte(lastCodeStr), &data)

	if err != nil {
		return err
	}

	var inputContent string
	var body map[string]interface{}
	var url string
	// handle different kinds of func call
	switch data["name"] {
		case "python": {
			parameters := data["parameters"].(map[string]interface{})
			codeContent := parameters["code"].(string)
			if inputC, ok := parameters["input"]; ok {
				inputContent = inputC.(string)
			}
			url = UrlConfig.pythonUrl
			body = map[string]interface{} {
				"type": "python", 
				"code": codeContent,
				"input": inputContent,

			}
			
		} 
		break
		case "retrieval": {
			url = UrlConfig.retrievalUrl
			parameters := data["parameters"].(map[string]interface{})
			query := parameters["query"].(string)
			body = map[string]interface{} {
				"query": query,
				"top_k": 5,
			}
			
		}
		break
		case "document": {
			url = UrlConfig.documentUrl
			parameters := data["parameters"].(map[string]interface{})
			query := parameters["question"].(string)

			body = map[string]interface{} {
				"query": query,
				"top_k": 5,
			}
		}
		break
		case "bingapi": {
			url = UrlConfig.bingapiUrl
			parameters := data["parameters"].(map[string]interface{})
			query := parameters["question"].(string)
			topK := 2
			if val, exists := parameters["top_k"]; exists {
				topK = val.(int)
				delete(parameters, "top_k")
			}

			body = map[string]interface{} {
				"query": query,
				"top_k": topK,
			}


		}
		break
		default: {
			return errors.New("unknown function call")
		}
			
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	requestBody := bytes.NewBuffer(jsonBody)
	response, err := http.Post(url, "application/json", requestBody)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	*result = string(responseBody)
	return nil

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
		Stop:     model.EndDelimiter,
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

			if slices.Contains(model.EndDelimiter, MossEnd) && strings.Contains(nowOutput, MossEnd) {
				// if MossEnd is found, break the loop
				nowOutput = strings.Split(nowOutput, MossEnd)[0]
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
	if model.APIType == APITypeMOSS2 {
		return InferMoss2(record, postRecords, model, user, ctx)
	} else if model.APIType == APITypeMOSS{
		return InferOpenAI(record, postRecords, model, user, ctx)
	} else {
		return errors.New("unknown API type")
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



func sensitiveCheck(
	c JsonReaderWriter,
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
