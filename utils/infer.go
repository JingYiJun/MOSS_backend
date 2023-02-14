package utils

import (
	"MOSS_backend/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type RecordModel struct {
	Request  string `json:"request"`
	Response string `json:"response"`
}

type Param struct {
	ID    int     `json:"-"`
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type Params []Param

type InferRequest struct {
	Records []RecordModel `json:"records,omitempty"`
	Message string        `json:"message"`
	Params  Params        `json:"params,omitempty"`
}

type InferResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func Infer(request InferRequest) (output string, duration float64, err error) {
	startTime := time.Now()
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

	if res.StatusCode != 200 {
		log.Println("error from infer server: " + string(data))
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    res.StatusCode,
		}
	}

	var response InferResponse
	err = json.Unmarshal(data, &response)
	if err != nil {
		return "", 0, fmt.Errorf("error unmarshal response data: %s", err)
	}
	duration = float64(time.Since(startTime)) / 1000_000_000
	if response.Code != 200 {
		log.Println(response.Message)
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    response.Code,
		}
	} else {
		return response.Message, duration, nil
	}
}
