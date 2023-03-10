package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
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

func Infer(input string, records Records) (output string, duration float64, err error) {
	return InferMosec(InferPreprocess(input, records.ToRecordModel()))
}

func InferAsync(
	c *websocket.Conn,
	input string,
	records []RecordModel,
	newRecord *Record,
	user *User,
) (
	err error,
) {
	var (
		interruptChan    = make(chan any)
		connectionClosed = new(atomic.Bool)
	)
	connectionClosed.Store(false)

	go interrupt(c, interruptChan, connectionClosed)

	// get formatted text
	formattedText := InferPreprocess(input, records)

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
	var params []Param
	err = DB.Find(&params).Error
	if err != nil {
		return err
	}
	for _, param := range params {
		request[param.Name] = param.Value
	}
	data, _ := json.Marshal(request)

	if config.Config.Debug {
		log.Printf("send infer request: %v\n", string(data))
	}

	errChan := make(chan error)

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
				before, _, found := CutLastAny(nowOutput, ",.?!\n????????????")
				if !found || before == detectedOutput {
					continue
				}
				detectedOutput = before

				// output sensitive check
				if sensitive.IsSensitive(detectedOutput, user) {
					newRecord.ResponseSensitive = true
					// log new record
					newRecord.Response = detectedOutput
					newRecord.Duration = float64(time.Since(startTime)) / 1000_000_000
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
						newRecord.ResponseSensitive = true
						// log new record
						newRecord.Response = nowOutput
						newRecord.Duration = float64(time.Since(startTime)) / 1000_000_000
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

				newRecord.Response = nowOutput
				newRecord.Duration = float64(time.Since(startTime)) / 1000_000_000
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

func InferPreprocess(input string, records []RecordModel) (formattedText string) {
	const prefix = `MOSS is an AI assistant developed by the FudanNLP Lab and Shanghai AI Lab. Below is a conversation between MOSS and human.`

	// cut end flag for special cases
	for i := range records {
		records[i].Request = cutEndFlag(records[i].Request)
		records[i].Response = cutEndFlag(records[i].Response)
	}

	var builder strings.Builder
	builder.WriteString(prefix)
	for _, record := range records {
		if record.Request != "" && record.Response != "" {
			builder.WriteString(fmt.Sprintf(" [Human]: %s<eoh> [MOSS]: %s<eoa>", record.Request, record.Response))
		}
	}
	builder.WriteString(fmt.Sprintf(" [Human]: %s<eoh> [MOSS]:", input))
	return builder.String()
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

func InferMosec(formattedText string) (string, float64, error) {
	request := Map{"x": formattedText}

	// get params
	var params []Param
	err := DB.Find(&params).Error
	if err != nil {
		return "", 0, err
	}
	for _, param := range params {
		request[param.Name] = param.Value
	}
	data, _ := json.Marshal(request)

	output, duration, err := inferTrigger(data)
	if err != nil {
		return "", 0, err
	}

	index := strings.LastIndex(output, "[MOSS]:")
	if index == -1 {
		log.Printf("error find \"[MOSS]:\" from inference server, output: \"%v\"\n", output)
		return "", 0, InternalServerError()
	}
	output = output[index+7:]
	return cutEndFlag(output), duration, nil
}
