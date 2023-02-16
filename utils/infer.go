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

	data, _ := json.Marshal(map[string]any{"x": formattedText})

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
