package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
	"MOSS_backend/utils/tools"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var endContentRegexp = regexp.MustCompile(`<[es]o\w>`)

var maxLengthExceededError = BadRequest("The maximum context length is exceeded").WithMessageType(MaxLength)

var unknownError = InternalServerError("unknown error, please try again")

func Infer(record *Record, prefix string) (err error) {
	formattedText := InferPreprocess(record.Request, prefix)
	request := Map{"x": formattedText}

	// get params
	err = LoadParamToMap(request)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(request)

	output, duration, err := inferTrigger(data)
	if err != nil {
		return err
	}

	output = strings.Trim(output, " \t\n")
	if strings.HasSuffix(output, "<eoc>") {
		// output end with <|Command|>:xxx<eoc>

		outputCommand := strings.TrimSuffix(output, "<eoc>")
		index := strings.LastIndex(output, "<|Commands|>:")
		if index == -1 {
			log.Printf("error find \"<|Commands|>:\" from inference server, output: \"%v\"\n", output)
			return InternalServerError()
		}
		outputCommand = strings.Trim(outputCommand[index+13:], " ")

		// get results from tools
		results := tools.Post(outputCommand)

		// generate new formatted text
		formattedText = InferWriteResult(results, output+"\n")
		request["x"] = formattedText
		data, _ = json.Marshal(request)

		output, duration, err = inferTrigger(data)
		if err != nil {
			return err
		}
	}
	// save record prefix for next inference
	record.Prefix = output
	// output end with others
	index := strings.LastIndex(output, "<|MOSS|>:")
	if index == -1 {
		log.Printf("error find \"<|MOSS|>:\" from inference server, output: \"%v\"\n", output)
		return InternalServerError()
	}
	output = output[index+9:]
	record.Response = cutEndFlag(output)
	record.Duration = duration
	return nil
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
	)
	connectionClosed.Store(false) // initialize

	go interrupt(c, interruptChan, connectionClosed) // wait for interrupt

	// get formatted text
	formattedText := InferPreprocess(record.Request, prefix)

	// make uuid, store channel into map
	uuidText := strings.ReplaceAll(uuid.NewString(), "-", "")
	outputChan := make(chan InferResponseModel, 100)
	responseCh := &responseChannel{ch: outputChan}
	responseCh.closed.Store(false)
	InferResponseChannel.Store(uuidText, responseCh)
	defer func() {
		responseCh.closed.Store(true)
		InferResponseChannel.Delete(uuidText)
		connectionClosed.Store(true)
	}()

	request := map[string]any{"x": formattedText, "url": config.Config.CallbackUrl + "?uuid=" + uuidText}

	// get params
	err = LoadParamToMap(request)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(request)

	if config.Config.Debug {
		log.Printf("send infer request: %v\n", string(data))
	}

	go func() {
		if _, _, innerErr := inferTrigger(data); innerErr != nil {
			errChan <- innerErr
		}
	}()

	startTime := time.Now()

	var ctx, cancel = context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	var nowOutput string
	var detectedOutput string

	for {
		if connectionClosed.Load() {
			if _, ok := <-interruptChan; !ok {
				return NoStatus("client interrupt")
			}
			return nil
		}
		select {
		case response := <-outputChan:
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
						err = c.WriteJSON(InferResponseModel{
							Status: -2, // banned
							Output: OffenseMessage,
						})
					} else {
						err = c.WriteJSON(InferResponseModel{
							Status: -2, // sensitive
							Output: DefaultResponse,
						})
					}
					if err != nil {
						return fmt.Errorf("write sensitive error: %v", err)
					}

					// if sensitive, jump out and record
					return nil
				}

				err = c.WriteJSON(InferResponseModel{
					Status: 1,
					Output: detectedOutput,
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
						err = c.WriteJSON(InferResponseModel{
							Status: -2, // sensitive
							Output: DefaultResponse,
						})
						if err != nil {
							return fmt.Errorf("write sensitive error: %v", err)
						}

						// if sensitive, jump out and record
						return nil
					}

					err = c.WriteJSON(InferResponseModel{
						Status: 1,
						Output: nowOutput,
					})
					if err != nil {
						return fmt.Errorf("write response error: %v", err)
					}
				}
				err = c.WriteJSON(InferResponseModel{Status: 0})
				if err != nil {
					return fmt.Errorf("write end status error: %v", err)
				}

				record.Response = nowOutput
				record.Duration = float64(time.Since(startTime)) / 1000_000_000
				return nil
			case -1: // error
				return InternalServerError(response.Output)
			}
		case <-ctx.Done():
			return InternalServerError("Internal Server Timeout")
		case <-interruptChan:
			return NoStatus("client interrupt")
		case err = <-errChan:
			return err
		}
	}
}

func inferTrigger(data []byte) (string, float64, error) {
	startTime := time.Now()
	rsp, err := http.Post(config.Config.InferenceUrl, "application/json", bytes.NewBuffer(data)) // take the ownership of data
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
			return "", duration, maxLengthExceededError
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
				return "", duration, maxLengthExceededError
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

type InferResponseModel struct {
	Status     int    `json:"status"` // 1 for output, 0 for end, -1 for error, -2 for sensitive
	StatusCode int    `json:"status_code,omitempty"`
	Output     string `json:"output"`
}

type responseChannel struct {
	ch     chan InferResponseModel
	closed atomic.Bool
}

var InferResponseChannel sync.Map

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
	return prefix + fmt.Sprintf("<|Results|>: %s<eor>\n", results)
}

func InferPostprocess(output string) (tidyOutput string) {
	// process end with 0xfffd
	runeSlice := []rune(output)
	for len(runeSlice) > 0 && runeSlice[len(runeSlice)-1] == 0xfffd {
		runeSlice = runeSlice[:len(runeSlice)-1]
	}

	// process output end
	output = string(runeSlice)
	output = strings.Trim(output, " ")
	output, _ = strings.CutSuffix(output, "<")
	output, _ = strings.CutSuffix(output, "<e")
	output, _ = strings.CutSuffix(output, "<eo")
	output, _ = strings.CutSuffix(output, "<eoa")
	output, _ = strings.CutSuffix(output, "<eoh")

	// cut end or <eo*> inside output
	return cutEndFlag(output)
}

func cutEndFlag(content string) string {
	loc := endContentRegexp.FindIndex([]byte(content))
	if loc != nil {
		content = content[:loc[0]]
	}
	return strings.Trim(content, " ")
}
