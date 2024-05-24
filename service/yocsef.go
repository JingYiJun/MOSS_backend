package service

import (
	"MOSS_backend/config"
	"MOSS_backend/models"
	"MOSS_backend/utils"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type InferYocsefRequest struct {
	Question    string     `json:"question,omitempty"`
	ChatHistory [][]string `json:"chat_history,omitempty"`
}

var yocsefHttpClient = &http.Client{}

func InferYocsef(
	ctx context.Context,
	w utils.JSONWriter,
	prompt string,
	records models.RecordModels,
) (
	model *models.DirectRecord,
	err error,
) {
	if config.Config.YocsefInferenceUrl == "" {
		return nil, errors.New("yocsef 推理模型暂不可用")
	}

	var chatHistory = make([][]string, len(records))
	for i, record := range records {
		chatHistory[i] = []string{record.Request, record.Response}
	}

	var request = map[string]any{
		"input": map[string]any{
			"question":     prompt,
			"chat_history": chatHistory,
		},
	}
	requestData, err := json.Marshal(request)
	if err != nil {
		return
	}

	// server send event
	req, err := http.NewRequest("POST", config.Config.YocsefInferenceUrl, bytes.NewBuffer(requestData))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	res, err := yocsefHttpClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	var reader = bufio.NewReader(res.Body)
	var resultBuilder strings.Builder
	var nowOutput string
	var detectedOutput string

	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(string(line), "event") {
			continue
		}
		if strings.HasPrefix(string(line), "data") {
			line = line[6:]
		}
		line = bytes.Trim(line, " \n\r")
		if len(line) == 0 {
			continue
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		var response map[string]any
		err = json.Unmarshal(line, &response)
		if err != nil {
			return nil, err
		}

		var ok bool
		nowOutput, ok = response["content"].(string)
		if !ok {
			continue
		}
		resultBuilder.WriteString(nowOutput)
		nowOutput = resultBuilder.String()

		var endDelimiter = "<|im_end|>"
		if strings.Contains(nowOutput, endDelimiter) {
			nowOutput = strings.Split(nowOutput, endDelimiter)[0]
			break
		}

		before, _, found := utils.CutLastAny(nowOutput, ",.?!\n，。？！")
		if !found || before == detectedOutput {
			continue
		}
		detectedOutput = before

		err = w.WriteJSON(InferResponseModel{
			Status: 1,
			Output: nowOutput,
			Stage:  "MOSS",
		})
		if err != nil {
			return nil, err
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if nowOutput != detectedOutput {
		_ = w.WriteJSON(InferResponseModel{
			Status: 1,
			Output: nowOutput,
			Stage:  "MOSS",
		})
	}

	err = w.WriteJSON(InferResponseModel{
		Status: 0,
		Output: nowOutput,
		Stage:  "MOSS",
	})

	var record = models.DirectRecord{Request: prompt, Response: nowOutput}
	return &record, nil
}

type InferResponseModel struct {
	Status     int    `json:"status"` // 1 for output, 0 for end, -1 for error, -2 for sensitive
	StatusCode int    `json:"status_code,omitempty"`
	Output     string `json:"output,omitempty"`
	Stage      string `json:"stage,omitempty"`
}
