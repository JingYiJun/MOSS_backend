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
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func Infer(input string, records Records) (output string, duration float64, err error) {
	return InferMosec(input, records.ToRecordModel())
}

func InferAsync(c *websocket.Conn, input string, records Records, newRecord *Record, interruptChan chan any) (err error) {

	// get formatted text
	formattedText := InferPreprocess(input, records.ToRecordModel())

	// make uuid, store channel into map
	uuidText := strings.ReplaceAll(uuid.NewString(), "-", "")
	outputChan := make(chan InferResponseModel, 100)
	InferResponseChannel.Store(uuidText, outputChan)
	defer InferResponseChannel.Delete(uuidText)

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

	go inferTrigger(data, errChan)

	startTime := time.Now()

	var ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var nowOutput string
	var detectedOutput string

	for {
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
				if sensitive.IsSensitive(detectedOutput) {
					newRecord.ResponseSensitive = true
					// log new record
					newRecord.Response = detectedOutput
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
					Output: detectedOutput,
				})
				if err != nil {
					return fmt.Errorf("write response error: %v", err)
				}
			case 0: // end
				if nowOutput != detectedOutput {
					if sensitive.IsSensitive(nowOutput) {
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

func inferTrigger(data []byte, errChan chan error) {
	var (
		err error
		rsp *http.Response
	)
	defer func() {
		if err != nil {
			errChan <- err
		}
	}()
	rsp, err = http.Post(config.Config.InferenceUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Println(err)
		err = InternalServerError("inference server error")
		return
	}

	defer func() {
		_ = rsp.Body.Close()
	}()

	if rsp.StatusCode == 400 {
		err = BadRequest("The maximum context length is exceeded")
		return
	} else if rsp.StatusCode >= 500 {
		err = InternalServerError()
		return
	}
}

type InferResponseModel struct {
	Status     int    `json:"status"` // 1 for output, 0 for end, -1 for error, -2 for sensitive
	StatusCode int    `json:"status_code,omitempty"`
	Output     string `json:"output"`
}

var InferResponseChannel sync.Map

func ReceiveInferResponse(c *websocket.Conn) {
	var (
		message []byte
		err     error
	)

	uuidText := c.Query("uuid")
	if uuidText == "" {
		_ = c.WriteJSON(InferResponseModel{Status: -1, StatusCode: 400, Output: "Bad Request"})
		return
	}

	for {
		if _, message, err = c.ReadMessage(); err != nil {
			log.Printf("receive error: %s\n", err)
			break
		}

		if config.Config.Debug {
			log.Printf("receive message from inference, uuid: %v: %s\n", uuidText, string(message))
		}

		var inferResponse InferResponseModel
		err = json.Unmarshal(message, &inferResponse)
		if err != nil {
			log.Printf("error message type: %s\n, error: %s", string(message), err)
			continue
		}

		// continue if sending a heartbeat package
		if inferResponse.Status == 2 {
			continue
		}

		// process end with 0xfffd
		runeSlice := []rune(inferResponse.Output)
		for len(runeSlice) > 0 && runeSlice[len(runeSlice)-1] == 0xfffd {
			runeSlice = runeSlice[:len(runeSlice)-1]
		}

		// process output end
		output := string(runeSlice)
		output = strings.Trim(output, " ")
		output, _ = strings.CutSuffix(output, "<")
		output, _ = strings.CutSuffix(output, "<e")
		output, _ = strings.CutSuffix(output, "<eo")
		output, _ = strings.CutSuffix(output, "<eoa")
		output, _ = strings.CutSuffix(output, "<eoh")
		output, _ = strings.CutSuffix(output, "<eoa>")
		output, _ = strings.CutSuffix(output, "<eoh>")
		inferResponse.Output = output

		if config.Config.Debug {
			log.Printf("recieve output: %v\n", inferResponse.Output)
		}

		if ch, ok := InferResponseChannel.Load(uuidText); ok {
			ch.(chan InferResponseModel) <- inferResponse
		} else {
			log.Printf("invalid uuid: %s\n", uuidText)
			return
		}

		if inferResponse.Status == 0 {
			return
		}
	}
}

func InferPreprocess(input string, records []RecordModel) (formattedText string) {
	const prefix = `MOSS is an AI assistant developed by the FudanNLP Lab and Shanghai AI Lab. Below is a conversation between MOSS and human.`

	var builder strings.Builder
	builder.WriteString(prefix)
	for _, record := range records {
		builder.WriteString(fmt.Sprintf(" [Human]: %s<eoh> [MOSS]: %s<eoa>", record.Request, record.Response))
	}
	builder.WriteString(fmt.Sprintf(" [Human]: %s<eoh> [MOSS]:", input))
	return builder.String()
}

func InferMosec(input string, records []RecordModel) (string, float64, error) {
	formattedText := InferPreprocess(input, records)

	request := map[string]any{"x": formattedText}

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

	startTime := time.Now()
	rsp, err := http.Post(config.Config.InferenceUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("error post to infer server: %s\n", err)
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    rsp.StatusCode,
		}
	}
	duration := float64(time.Since(startTime)) / 1000_000_000

	defer func() {
		_ = rsp.Body.Close()
	}()
	data, _ = io.ReadAll(rsp.Body)
	output := string(data)
	if rsp.StatusCode != 200 {
		log.Printf("error response from inference server, status code: %d, output: %v\n", rsp.StatusCode, output)
		if rsp.StatusCode == 400 {
			return "", 0, &HttpError{
				Code:    400,
				Message: "The maximum context length is exceeded",
			}
		}
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    rsp.StatusCode,
		}
	}

	index := strings.LastIndex(output, "[MOSS]:")
	if index == -1 {
		log.Printf("error find \"[MOSS]:\" from inference server, output: %v\n", output)
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    rsp.StatusCode,
		}
	}
	output = output[index+7:]
	output = strings.Trim(output, " ")
	output, _ = strings.CutSuffix(output, "<eoa>")
	output, _ = strings.CutSuffix(output, "<eoh>")
	output = strings.Trim(output, " ")
	return output, duration, nil
}
