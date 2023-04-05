package tools

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
	"time"
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

var calculateHttpClient = http.Client{Timeout: 20 * time.Second}

func calculate(request string) (string, map[string]any) {
	data, _ := json.Marshal(map[string]any{"text": request})
	res, err := calculateHttpClient.Post(config.Config.ToolsCalculateUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		utils.Logger.Error("post calculate(tools) error: ", zap.Error(err))
		return "None", nil
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post calculate(tools) status code error: " + strconv.Itoa(res.StatusCode))
		return "None", nil
	}

	var results map[string]any
	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post calculate(tools) response read error: ", zap.Error(err))
		return "None", nil
	}
	err = json.Unmarshal(responseData, &results)
	if err != nil {
		utils.Logger.Error("post calculate(tools) response unmarshal error: ", zap.Error(err))
		return "None", nil
	}
	calculateResult, exist := results["result"]
	if !exist {
		utils.Logger.Error("post calculate(tools) response format error: ", zap.Error(keyNotExistError{Results: results}))
		return "None", nil
	}
	resultsString, ok := calculateResult.(string)
	if !ok {
		utils.Logger.Error("post calculate(tools) response format error: ", zap.Error(resultNotStringError{Results: results}))
		return "None", nil
	}
	if _, err := strconv.ParseFloat(resultsString, 32); err != nil {
		utils.Logger.Error("post calculate(tools) response not number error: ", zap.Error(err))
		return "None", nil
	}
	return resultsString, map[string]any{"type": "calculate", "data": results, "request": request}
}
