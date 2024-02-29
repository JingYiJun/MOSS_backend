package record

import (
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
)

// OpenAIListModels
// @Summary List models in OpenAI API protocol
// @Tags openai
// @Router /v1/models [get]
// @Success 200 {object} OpenAIModels
func OpenAIListModels(c *fiber.Ctx) (err error) {
	modelConfigs, err := LoadModelConfigs()
	if err != nil {
		return err
	}

	return c.JSON(OpenAIModelsFromModelConfigs(modelConfigs))
}

// OpenAIRetrieveModel
// @Summary Retrieve a model in OpenAI API protocol
// @Tags openai
// @Router /v1/models/{name} [get]
// @Success 200 {object} OpenAIModel
func OpenAIRetrieveModel(c *fiber.Ctx) (err error) {
	modelName := c.Params("name")
	modelConfig, err := LoadModelConfigByName(modelName)
	if err != nil {
		return err
	}

	return c.JSON(OpenAIModelFromModelConfig(modelConfig))
}

// OpenAICreateChatCompletion
// @Summary Create a chat completion in OpenAI API protocol
// @Tags openai
// @Router /v1/chat/completions [post]
// @Param json body OpenAIChatCompletionRequest true "json"
// @Success 200 {object} OpenAIChatCompletionResponse
func OpenAICreateChatCompletion(c *fiber.Ctx) (err error) {
	var request OpenAIChatCompletionRequest
	err = ValidateBody(c, &request)
	if err != nil {
		return err
	}

	modelConfig, err := LoadModelConfigByName(request.Model)
	if err != nil {
		return err
	}

	prefix, requestMessage, err := request.Messages.Build()
	if err != nil {
		return err
	}

	if requestMessage == "" {
		return BadRequest("request is empty")
	}
	//if len([]rune(requestMessage)) > 2048 {
	//	return maxInputExceededError
	//}

	// infer limiter
	if !inferLimiter.Allow() {
		return unknownError
	}

	consumerUsername := c.Get("X-Consumer-Username")
	passSensitiveCheck := slices.Contains(config.Config.PassSensitiveCheckUsername, consumerUsername)

	if !passSensitiveCheck && sensitive.IsSensitive(prefix+"\n"+requestMessage, &User{}) {
		return BadRequest(DefaultResponse).WithMessageType(Sensitive)
	}

	recordModels, _, err := request.Messages.BuildRecordModels()
	if err != nil {
		return err
	}

	record := Record{Request: requestMessage}
	err = Infer(&record, prefix, recordModels, &User{PluginConfig: nil, ModelID: modelConfig.ID}, nil)
	if err != nil {
		return err
	}

	if !passSensitiveCheck && sensitive.IsSensitive(record.Response, &User{}) {
		return BadRequest(DefaultResponse).WithMessageType(Sensitive)
	}

	return c.JSON(&OpenAIChatCompletionResponse{
		Id:                "chatcmpl-" + uuid.Must(uuid.NewUUID()).String(),
		Object:            "chat.completion",
		Created:           time.Now().Unix(),
		Model:             modelConfig.Description,
		SystemFingerprint: "",
		Choices: []*OpenAIChatCompletionChoice{
			{
				Index: 0,
				Message: OpenAIMessages{{
					Role:    "assistant",
					Content: record.Response,
				}},
				Logprobs:     nil,
				FinishReason: "stop",
			},
		},
		Usage: OpenAIChatCompletionUsage{},
	})
}
