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

func Infer(message string, records []RecordModel) (string, float64, error) {
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

	output, _ = strings.CutPrefix(output, input)
	output, _ = strings.CutSuffix(output, "<eoa>")
	output = strings.Trim(output, " ")
	return output, duration, nil
}
