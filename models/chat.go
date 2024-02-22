package models

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sashabaranov/go-openai"
	"gorm.io/gorm"
)

type Chat struct {
	ID                int            `json:"id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index:idx_chat_user_deleted,priority:2"`
	UserID            int            `json:"user_id" gorm:"index:idx_chat_user_deleted,priority:1"`
	Name              string         `json:"name"`
	Count             int            `json:"count"` // Record 条数
	Records           Records        `json:"records,omitempty"`
	MaxLengthExceeded bool           `json:"max_length_exceeded"`
}

type Chats []Chat

type Record struct {
	ID                 int            `json:"id"`
	CreatedAt          time.Time      `json:"created_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index:idx_record_chat_deleted,priority:2"`
	Duration           float64        `json:"duration"` // 处理时间，单位 s
	ChatID             int            `json:"chat_id" gorm:"index:idx_record_chat_deleted,priority:1"`
	Request            string         `json:"request"`
	Response           string         `json:"response"`
	Prefix             string         `json:"-"`
	RawContent         string         `json:"raw_content"`
	ExtraData          any            `json:"-" gorm:"serializer:json"` //`json:"extra_data" gorm:"serializer:json"`
	ProcessedExtraData any            `json:"processed_extra_data" gorm:"serializer:json"`
	LikeData           int            `json:"like_data"` // 1 like, -1 dislike
	Feedback           string         `json:"feedback"`
	RequestSensitive   bool           `json:"request_sensitive"`
	ResponseSensitive  bool           `json:"response_sensitive"`
	InnerThoughts      string         `json:"inner_thoughts"`
}

type Records []Record

func (record *Record) Preprocess(_ *fiber.Ctx) error {
	if record.ResponseSensitive {
		record.Response = DefaultResponse
	}
	return nil
}

func (records Records) Preprocess(c *fiber.Ctx) error {
	for i := range records {
		_ = records[i].Preprocess(c)
	}
	return nil
}

const DefaultResponse = `Sorry, I have nothing to say. Try another topic. I will block your account if we continue this topic :)`

func (records Records) ToRecordModel() (recordModel []RecordModel) {
	for _, record := range records {
		recordModel = append(recordModel, RecordModel{
			Request:  record.Request,
			Response: record.Response,
		})
	}
	return
}

func (records Records) ToOpenAIMessages() (messages []openai.ChatCompletionMessage) {
	for _, record := range records {
		messages = append(messages,
			openai.ChatCompletionMessage{
				Role:    "user",
				Content: record.Request,
			},
			openai.ChatCompletionMessage{
				Role:    "assistant",
				Content: record.Response,
			})
	}
	return
}

func (records Records) GetPrefix() string {
	if len(records) == 0 {
		return ""
	}
	return records[len(records)-1].Prefix
}

type RecordModel struct {
	Request  string `json:"request" validate:"required"`
	Response string `json:"response" validate:"required"`
}

type RecordModels []RecordModel

func (recordModels RecordModels) ToOpenAIMessages() (messages []openai.ChatCompletionMessage) {
	for _, record := range recordModels {
		messages = append(messages,
			openai.ChatCompletionMessage{
				Role:    "user",
				Content: record.Request,
			},
			openai.ChatCompletionMessage{
				Role:    "assistant",
				Content: record.Response,
			})
	}
	return
}

type Param struct {
	ID    int
	Name  string
	Value float64
}

func LoadParamToMap(m map[string]any) error {
	if DB == nil {
		return nil
	}
	var params []Param
	err := DB.Find(&params).Error
	if err != nil {
		return err
	}
	for _, param := range params {
		m[param.Name] = param.Value
	}
	return nil
}

type DirectRecord struct {
	ID               int
	CreatedAt        time.Time
	Duration         float64
	ConsumerUsername string
	Context          string
	Request          string
	Response         string
	ExtraData        any `json:"extra_data" gorm:"serializer:json"`
}
