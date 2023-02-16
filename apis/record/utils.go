package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"bytes"
	"context"
	"encoding/json"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"log"
	"net/http"
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
	uuidText := uuid.New()
	ch := make(chan InferResponseModel, 0)
	InferResponseChannel.Store(uuidText, ch)

	data, _ := json.Marshal(map[string]any{"x": formattedText, "uuid": uuidText})

	// send infer request
	_, err := http.Post(config.Config.TestInferenceUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		errChan <- err
		return
	}

	startTime := time.Now()
	defer func() {
		*duration = float64(time.Since(startTime)) / 1000_000_000
	}()

	for {
		select {
		case response := <-ch:
			switch response.Status {
			case 1: // ok
				outputChan <- response
			case 0: // end
				InferResponseChannel.Delete(uuidText)
				close(outputChan)
				return
			case -1: // error
				InferResponseChannel.Delete(uuidText)
				errChan <- &HttpError{Code: 500, Message: response.Output}
				return
			}
		case <-ctx.Done():
			InferResponseChannel.Delete(uuidText)
			errChan <- &HttpError{Code: 500, Message: "Internal Server Timeout"}
			return
		}
	}
}

type InferResponseModel struct {
	Status     int    `json:"status"` // 1 for output, 0 for end, -1 for error, -2 for sensitive
	StatusCode int    `json:"status_code,omitempty"`
	UUID       string `json:"uuid,omitempty"`   // uuid格式，36位
	Offset     int    `json:"offset,omitempty"` // 第几个字符
	Output     string `json:"output"`
}

var InferResponseChannel sync.Map

func ReceiveInferResponse(c *websocket.Conn) {
	var (
		message []byte
		err     error
	)

	for {
		if _, message, err = c.ReadMessage(); err != nil {
			log.Printf("receive error: %s\n", err)
			break
		}

		var inferResponse InferResponseModel
		err = json.Unmarshal(message, &inferResponse)
		if err != nil {
			log.Printf("error message type: %s\n, error: %s", string(message), err)
			return
		}

		if ch, ok := InferResponseChannel.Load(inferResponse.UUID); ok {
			ch.(chan InferResponseModel) <- inferResponse
		} else {
			log.Printf("invalid uuid: %s\n", inferResponse.UUID)
		}
	}
}
