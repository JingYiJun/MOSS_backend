package utils

import (
	"MOSS_backend/config"
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
)

var sensitiveClient http.Client

const sensitiveCheckUrl = `https://gtf.ai.xingzheai.cn/v2.0/game_chat_ban/detect_text`

type SensitiveRequest struct {
	DataID      string `json:"data_id"`
	Context     string `json:"context"`
	ContextType string `json:"context_type"`
	Token       string `json:"token"`
}

type SensitiveResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	DataID string `json:"data_id"`
	Data   struct {
		Suggestion string `json:"suggestion"`
		Label      string `json:"label"`
	} `json:"data,omitempty"`
}

func IsSensitive(context string) bool {
	data, err := json.Marshal(SensitiveRequest{
		DataID:      uuid.NewString(),
		Context:     context,
		ContextType: "chat",
		Token:       config.Config.SensitiveCheckToken,
	})
	if err != nil {
		log.Println("marshal data err")
		return false
	}
	rsp, err := sensitiveClient.Post(
		sensitiveCheckUrl,
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		log.Println("sending detect request error")
		return false
	}
	defer func() {
		_ = rsp.Body.Close()
	}()

	if rsp.StatusCode != 200 {
		log.Printf("detect request status code: %d\n", rsp.StatusCode)
		return false
	}

	var response SensitiveResponse
	responseData, err := io.ReadAll(rsp.Body)
	if err != nil {
		log.Println("response read error")
		return false
	}
	err = json.Unmarshal(responseData, &response)
	if err != nil {
		log.Println("response decode error")
		return false
	}

	if response.Code == -1 {
		log.Println("detect error")
		if response.Msg == "recharge" {
			log.Println("recharge sensitive detect platform")
		}
	} else {
		if response.Data.Suggestion == "pass" {
			return false
		} else {
			return true
		}
	}
	return false
}
