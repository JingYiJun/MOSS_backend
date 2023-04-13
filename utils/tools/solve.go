package tools

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"bytes"
	"encoding/json"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
	"time"
)

type solveTask struct {
	taskModel
	results      Map
	resultString string
}

var _ task = (*solveTask)(nil)

func (t *solveTask) postprocess() *ResultModel {
	if t.err != nil {
		return NoneResultModel
	}
	return &ResultModel{
		Result: t.resultString,
		ExtraData: &ExtraDataModel{
			Type:    "solve",
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

var solveHttpClient = http.Client{Timeout: 20 * time.Second}

func (t *solveTask) request() {
	data, _ := json.Marshal(map[string]any{"text": t.args})
	res, err := solveHttpClient.Post(config.Config.ToolsSolveUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		utils.Logger.Error("post solve(tools) error: ", zap.Error(err))
		t.err = defaultError
		return
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post solve(tools) status code error: " + strconv.Itoa(res.StatusCode))
		t.err = defaultError
		return
	}

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post solve(tools) response read error: ", zap.Error(err))
		t.err = defaultError
		return
	}

	var results map[string]any
	err = json.Unmarshal(responseData, &results)
	if err != nil {
		utils.Logger.Error("post solve(tools) response unmarshal error: ", zap.Error(err))
		t.err = defaultError
		return
	}

	solveResult, exist := results["result"]
	if !exist {
		utils.Logger.Error("post solve(tools) response format error: ", zap.Error(keyNotExistError{Results: results}))
		t.err = defaultError
		return
	}
	resultsString, ok := solveResult.(string)
	if !ok {
		utils.Logger.Error("post solve(tools) response format error: ", zap.Error(resultNotStringError{Results: results}))
		t.err = defaultError
		return
	}
	if resultsString == `[ERROR]` || resultsString == "" {
		utils.Logger.Warn("post solve(tools) request no solution")
		t.err = defaultError
		return
	}

	t.results = results
	t.resultString = resultsString
}

//func solve(request string) (string, map[string]any) {
//	data, _ := json.Marshal(map[string]any{"text": request})
//	res, err := solveHttpClient.Post(config.Config.ToolsSolveUrl, "application/json", bytes.NewBuffer(data))
//	if err != nil {
//		utils.Logger.Error("post solve(tools) error: ", zap.Error(err))
//		return "None", nil
//	}
//
//	if res.StatusCode != 200 {
//		utils.Logger.Error("post solve(tools) status code error: " + strconv.Itoa(res.StatusCode))
//		return "None", nil
//	}
//
//	var results map[string]any
//	responseData, err := io.ReadAll(res.Body)
//	if err != nil {
//		utils.Logger.Error("post solve(tools) response read error: ", zap.Error(err))
//		return "None", nil
//	}
//	err = json.Unmarshal(responseData, &results)
//	if err != nil {
//		utils.Logger.Error("post solve(tools) response unmarshal error: ", zap.Error(err))
//		return "None", nil
//	}
//	solveResult, exist := results["result"]
//	if !exist {
//		utils.Logger.Error("post solve(tools) response format error: ", zap.Error(keyNotExistError{Results: results}))
//		return "None", nil
//	}
//	resultsString, ok := solveResult.(string)
//	if !ok {
//		utils.Logger.Error("post solve(tools) response format error: ", zap.Error(resultNotStringError{Results: results}))
//		return "None", nil
//	}
//	if resultsString == `[ERROR]` {
//		utils.Logger.Warn("post solve(tools) request no solution")
//		return "None", nil
//	}
//	return resultsString, map[string]any{"type": "solve", "data": results, "request": request}
//}
