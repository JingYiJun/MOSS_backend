package record

import (
	"strings"

	. "MOSS_backend/models"
	"MOSS_backend/utils"
)

type ParamsModel struct {
	Param map[string]float64 `json:"param"`
}

type CreateModel struct {
	ParamsModel
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
	Context      string          `json:"context"`
	Request      string          `json:"request" validate:"min=1"`
	Records      RecordModels    `json:"records" validate:"omitempty,dive"`
	PluginConfig map[string]bool `json:"plugin_config"`
	ModelID      int             `json:"model_id"`
	ParamsModel
}

type InferenceResponse struct {
	Response  string `json:"response"`
	Context   string `json:"context,omitempty"`
	ExtraData any    `json:"extra_data,omitempty"`
}

// OpenAI

type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func OpenAIModelFromModelConfig(config *ModelConfig) *OpenAIModel {
	return &OpenAIModel{
		ID:      config.Description,
		Object:  "model",
		Created: 0,
		OwnedBy: "moss",
	}
}

type OpenAIModels struct {
	Object string         `json:"object"`
	Data   []*OpenAIModel `json:"data"`
}

func OpenAIModelsFromModelConfigs(modelConfig ModelConfigs) *OpenAIModels {
	data := make([]*OpenAIModel, len(modelConfig))
	for i := range modelConfig {
		data[i] = OpenAIModelFromModelConfig(modelConfig[i])
	}
	return &OpenAIModels{
		Object: "list",
		Data:   data,
	}
}

type OpenAIMessage struct {
	Role    string `json:"role" validate:"required,oneof=system user assistant"`
	Content string `json:"content" validate:"required"`
}

type OpenAIMessages []*OpenAIMessage

func (messages OpenAIMessages) ValidateSequence() error {
	if len(messages) == 0 {
		return utils.BadRequest("empty messages")
	}
	currentRole := messages[0].Role
	for i := 1; i < len(messages); i++ {
		if messages[i] == nil {
			return utils.BadRequest("nil message")
		}
		if messages[i].Role == currentRole {
			return utils.BadRequest("consecutive messages with the same role")
		}
		if messages[i].Role == "system" || messages[i].Role == "tool" {
			return utils.BadRequest("unsupported message role " + messages[i].Role)
		}
		currentRole = messages[i].Role
	}
	if currentRole != "user" {
		return utils.BadRequest("last message must be user")
	}
	return nil
}

func (messages OpenAIMessages) Build() (prefix string, request string, err error) {
	err = messages.ValidateSequence()
	if err != nil {
		return "", "", err
	}
	var builder strings.Builder
	for i, message := range messages {
		if message == nil {
			err = utils.BadRequest("nil message")
			return
		}
		if i == len(messages)-1 {
			request = message.Content
			return builder.String(), request, nil
		}
		if message.Role == "user" {
			builder.WriteString("<|Human|>: ")
			builder.WriteString(message.Content)
			builder.WriteString("<eoh>\n")
			builder.WriteString("<|Inner Thoughts|>: None<eoi>\n")
			builder.WriteString("<|Commands|>: None<eoc>\n")
			builder.WriteString("<|Results|>: None<eor>\n")
		} else if message.Role == "assistant" {
			builder.WriteString("<|MOSS|>: ")
			builder.WriteString(message.Content)
			builder.WriteString("<eom>\n")
		} else {
			err = utils.BadRequest("invalid message role")
			return
		}
	}
	return builder.String(), request, nil
}

func (messages OpenAIMessages) BuildRecordModels() (models RecordModels, request string, err error) {
	err = messages.ValidateSequence()
	if err != nil {
		return nil, "", err
	}
	models = make(RecordModels, len(messages)/2)
	for i := 0; i < len(messages); i += 2 {
		models[i/2] = RecordModel{
			Request:  messages[i].Content,
			Response: messages[i+1].Content,
		}
	}
	request = models[len(models)-1].Request
	return models, request, nil
}

type OpenAIChatCompletionRequest struct {
	Messages OpenAIMessages `json:"messages" validate:"required,min=1,dive"`
	Model    string         `json:"model" validate:"required"`
}

type OpenAIChatCompletionChoice struct {
	Index        int            `json:"index"`
	Message      OpenAIMessages `json:"message"`
	Logprobs     interface{}    `json:"logprobs"`
	FinishReason string         `json:"finish_reason"`
}

type OpenAIChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChatCompletionResponse struct {
	Id                string                        `json:"id"`
	Object            string                        `json:"object"`
	Created           int64                         `json:"created"`
	Model             string                        `json:"model"`
	SystemFingerprint string                        `json:"system_fingerprint"`
	Choices           []*OpenAIChatCompletionChoice `json:"choices"`
	Usage             OpenAIChatCompletionUsage     `json:"usage"`
}
