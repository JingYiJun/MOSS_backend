package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

func Infer(input string, records Records) (output string, duration float64, err error) {
	return InferMosec(input, records.ToRecordModel())
}

func InferAsync(
	ctx context.Context,
	input string,
	records Records,
	outputChan chan<- InferResponseModel,
	errChan chan<- error,
	duration *float64) {

	// get formatted text
	formattedText := InferPreprocess(input, records.ToRecordModel())

	// make uuid, store channel into map
	uuidText := strings.ReplaceAll(uuid.NewString(), "-", "")
	ch := make(chan InferResponseModel, 100)
	InferResponseChannel.Store(uuidText, ch)
	defer InferResponseChannel.Delete(uuidText)

	request := map[string]any{"x": formattedText, "url": config.Config.CallbackUrl + "?uuid=" + uuidText}

	// get params
	var params []Param
	err := DB.Find(&params).Error
	if err != nil {
		errChan <- err
		return
	}
	for _, param := range params {
		request[param.Name] = param.Value
	}
	data, _ := json.Marshal(request)

	if config.Config.Debug {
		log.Printf("send infer request: %v\n", string(data))
	}

	// send infer request
	_, err = http.Post(config.Config.TestInferenceUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Println(err)
		errChan <- InternalServerError()
		return
	}

	startTime := time.Now()

	for {
		select {
		case response := <-ch:
			if config.Config.Debug {
				log.Println("receive response from uuid channel")
				log.Println(response)
			}
			switch response.Status {
			case 1: // ok
				outputChan <- response
			case 0: // end
				*duration = float64(time.Since(startTime)) / 1000_000_000
				close(outputChan)
				return
			case -1: // error
				errChan <- InternalServerError(response.Output)
				return
			}
		case <-ctx.Done():
			errChan <- InternalServerError("Internal Server Timeout")
			return
		}
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
	} else {
		log.Printf("now uuid: %v", uuidText)
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
			runtime.Gosched()
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
		}

		if inferResponse.Status == 0 {
			return
		}

		runtime.Gosched()
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
