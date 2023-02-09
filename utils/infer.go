package utils

import (
	"MOSS_backend/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type MessageModel struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
}

func Infer(input string) (output string, duration float64, err error) {
	startTime := time.Now()
	request := MessageModel{Message: input}
	data, err := json.Marshal(request)
	if err != nil {
		return "", 0, fmt.Errorf("error marshal request data: %s", err)
	}
	res, err := http.DefaultClient.Post(config.Config.InferenceUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", 0, fmt.Errorf("error sending request to inference server: %s", err)
	}

	data, err = io.ReadAll(res.Body)
	if err != nil {
		return "", 0, fmt.Errorf("error reading response body: %s", err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	var response MessageModel
	err = json.Unmarshal(data, &response)
	if err != nil {
		return "", 0, fmt.Errorf("error unmarshal response data: %s", err)
	}
	duration = float64(time.Now().Sub(startTime)) / 1000_000_000
	if response.Code != 200 {
		return "", 0, &HttpError{
			Message: fmt.Sprintf("%s; duration: %f s", response.Message, duration),
			Code:    response.Code,
		}
	} else {
		return response.Message, duration, nil
	}
}
