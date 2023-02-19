package shumei

import (
	"MOSS_backend/config"
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"time"
)

const url = `http://api-text-bj.fengkongcloud.com/text/v4`

var client = http.Client{Timeout: 1 * time.Second}

type Request struct {
	AccessKey string      `json:"accessKey"`
	AppId     string      `json:"appId"`
	EventId   string      `json:"eventId"`
	Type      string      `json:"type"`
	Data      RequestData `json:"data"`
}

type RequestData struct {
	Text    string `json:"text"`
	TokenId string `json:"tokenId"`
}

type Response struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestId string `json:"requestId"`
	RiskLevel string `json:"riskLevel"`
}

func IsSensitive(content string) bool {
	data, _ := json.Marshal(Request{
		AccessKey: config.Config.ShuMeiAccessKey,
		AppId:     config.Config.ShuMeiAppID,
		EventId:   config.Config.ShuMeiEventID,
		Type:      config.Config.ShuMeiType,
		Data: RequestData{
			Text:    content,
			TokenId: uuid.NewString(),
		},
	})

	rsp, err := client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Println(err)
		return false
	}

	defer func() {
		_ = rsp.Body.Close()
	}()

	data, err = io.ReadAll(rsp.Body)
	if err != nil {
		log.Println("shu mei: read body error")
		return false
	}

	if rsp.StatusCode != 200 {
		log.Println("shu mei: platform error, status code ", rsp.StatusCode)
		return false
	}

	var response Response
	err = json.Unmarshal(data, &response)
	if err != nil {
		log.Println("shu mei: response decode error: ", string(data))
		return false
	}

	if response.Code != 1100 {
		log.Println("shu mei: check error", response.Message)
		return false
	} else {
		if response.RiskLevel == "PASS" {
			return false
		} else {
			return true
		}
	}
}
