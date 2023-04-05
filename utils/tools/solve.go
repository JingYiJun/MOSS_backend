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

var solveHttpClient = http.Client{Timeout: 20 * time.Second}

func solve(request string) (string, map[string]any) {
	data, _ := json.Marshal(map[string]any{"text": request})
	res, err := solveHttpClient.Post(config.Config.ToolsSolveUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		utils.Logger.Error("post solve(tools) error: ", zap.Error(err))
		return "None", nil
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post solve(tools) status code error: " + strconv.Itoa(res.StatusCode))
		return "None", nil
	}

	var results map[string]any
	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post solve(tools) response read error: ", zap.Error(err))
		return "None", nil
	}
	err = json.Unmarshal(responseData, &results)
	if err != nil {
		utils.Logger.Error("post solve(tools) response unmarshal error: ", zap.Error(err))
		return "None", nil
	}
	solveResult, exist := results["result"]
	if !exist {
		utils.Logger.Error("post solve(tools) response format error: ", zap.Error(keyNotExistError{Results: results}))
		return "None", nil
	}
	resultsString, ok := solveResult.(string)
	if !ok {
		utils.Logger.Error("post solve(tools) response format error: ", zap.Error(resultNotStringError{Results: results}))
		return "None", nil
	}
	if resultsString == `[ERROR]` {
		utils.Logger.Warn("post solve(tools) request no solution")
		return "None", nil
	}
	return resultsString, map[string]any{"type": "solve", "data": results, "request": request}
}
