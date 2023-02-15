package utils

import (
	"MOSS_backend/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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

func InferTriton(input string, records []RecordModel, params Params) (output string, duration float64, err error) {
	startTime := time.Now()
	data, _ := json.Marshal(InferRequest{
		Records: records,
		Message: input,
		Params:  params,
	})
	res, err := http.DefaultClient.Post(config.Config.OldInferenceUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Println(err)
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    500,
		}
	}

	data, err = io.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    500,
		}
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
		log.Printf("error unmarshal response data: %s", err)
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    500,
		}
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

func InferMosec(message string, records []RecordModel) (string, float64, error) {
	const prefix = `MOSS is an AI assistant developed by the FudanNLP Lab and Shanghai AI Lab. Below is a conversation between MOSS and human.`

	var builder strings.Builder
	builder.WriteString(prefix)
	for _, record := range records {
		builder.WriteString(fmt.Sprintf(" [Human]: %s<eoh> [MOSS]: %s<eoa>", record.Request, record.Response))
	}
	builder.WriteString(fmt.Sprintf(" [Human]: %s<eoh> [MOSS]:", message))
	input := builder.String()

	data, _ := json.Marshal(map[string]any{"x": input})

	startTime := time.Now()
	rsp, err := http.Post(config.Config.InferenceUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Println(err)
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
		log.Println(output)
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    rsp.StatusCode,
		}
	}

	index := strings.LastIndex(output, "[MOSS]: ")
	if index != -1 {
		log.Println("error find [MOSS]:")
		return "", 0, &HttpError{
			Message: "Internal Server Error",
			Code:    rsp.StatusCode,
		}
	}
	output = output[index+8:]
	output = strings.Trim(output, " ")
	output, _ = strings.CutSuffix(output, "<eoa>")
	output = strings.Trim(output, " ")
	return output, duration, nil
}
