package tools

import (
	"errors"
	"fmt"
)

type ResultModel struct {
	Result             string
	ExtraData          *ExtraDataModel
	ProcessedExtraData *ExtraDataModel
}

type ResultTotalModel struct {
	Result             string            `json:"result"`
	ExtraData          []*ExtraDataModel `json:"extra_data"`
	ProcessedExtraData []*ExtraDataModel `json:"processed_extra_data"`
}

var NoneResultModel = &ResultModel{Result: "None"}

var NoneResultTotalModel = &ResultTotalModel{Result: "None"}

type ExtraDataModel struct {
	Type    string `json:"type"`
	Request string `json:"request"`
	Data    any    `json:"data"`
}

type task interface {
	name() string
	request()
	postprocess() *ResultModel
}

type taskModel struct {
	s      *scheduler
	action string
	args   string
	err    error
}

func (t *taskModel) name() string {
	return fmt.Sprintf("%s(\"%s\")", t.action, t.args)
}

type scheduler struct {
	tasks              []task
	searchResultsIndex int
}

var defaultError = errors.New("default error")
