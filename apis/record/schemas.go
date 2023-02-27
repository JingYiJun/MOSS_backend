package record

import (
	"MOSS_backend/models"
	"strings"
)

type CreateModel struct {
	Request string `json:"request" validate:"required"`
}

type InterruptModel struct {
	Interrupt bool `json:"interrupt"`
}

type ModifyModel struct {
	Feedback *string `json:"feedback"`
	Like     *int    `json:"like" validate:"omitempty,oneof=1 0 -1"` // 1 like, -1 dislike, 0 reset
}

type InferenceRequest struct {
	Records []models.RecordModel `json:"records"`
	Request string               `json:"request" validate:"min=1"`
}

func (i InferenceRequest) String() string {
	var builder strings.Builder
	for _, record := range i.Records {
		builder.WriteString(record.Request)
		builder.WriteByte('\n')
		builder.WriteString(record.Response)
		builder.WriteByte('\n')
	}
	builder.WriteString(i.Request)
	return builder.String()
}

type InferenceResponse struct {
	Response string `json:"response"`
}
