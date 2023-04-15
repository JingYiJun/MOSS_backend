package tools

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

type keyNotExistError struct {
	Results Map
}

func (e keyNotExistError) Error() string {
	return fmt.Sprintf("`result` in results does not exist. results: %v", e.Results)
}

type resultNotStringError struct {
	Results Map
}

func (e resultNotStringError) Error() string {
	return fmt.Sprintf("`result` in results is not a string type. results: %v", e.Results)
}

type calculateTask struct {
	taskModel
	results      Map
	resultString string
}

var _ task = (*calculateTask)(nil)

var calculateHttpClient = http.Client{Timeout: 20 * time.Second}

func (t *calculateTask) postprocess() *ResultModel {
	if t.err != nil {
		return NoneResultModel
	}
	return &ResultModel{
		Result: t.resultString,
		ExtraData: &ExtraDataModel{
			Type:    "calculate",
			Request: t.args,
			Data:    t.results,
		},
		ProcessedExtraData: &ExtraDataModel{
			Type:    t.action,
			Request: t.args,
			Data:    t.resultString,
		},
	}
}

func (t *calculateTask) request() {
	data, _ := json.Marshal(map[string]any{"text": t.args})
	res, err := calculateHttpClient.Post(config.Config.ToolsCalculateUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		utils.Logger.Error("post calculate(tools) error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post calculate(tools) status code error: " + strconv.Itoa(res.StatusCode))
		t.err = ErrGeneric
		return
	}

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post calculate(tools) response read error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}

	var results map[string]any
	err = json.Unmarshal(responseData, &results)
	if err != nil {
		utils.Logger.Error("post calculate(tools) response unmarshal error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}
	calculateResult, exist := results["result"]
	if !exist {
		utils.Logger.Error("post calculate(tools) response format error: ", zap.Error(keyNotExistError{Results: results}))
		t.err = ErrGeneric
		return
	}
	resultsString, ok := calculateResult.(string)
	if !ok {
		utils.Logger.Error("post calculate(tools) response format error: ", zap.Error(resultNotStringError{Results: results}))
		t.err = ErrGeneric
		return
	}
	if _, err := strconv.ParseFloat(resultsString, 32); err != nil {
		utils.Logger.Error("post calculate(tools) response not number error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}

	t.results = results
	t.resultString = resultsString
}
