package record

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

func RegisterRoutes(routes fiber.Router) {
	// record
	routes.Get("/chats/:id/records", ListRecords)
	routes.Post("/chats/:id/records", AddRecord)
	routes.Get("/ws/chats/:id/records", websocket.New(AddRecordAsync))
	routes.Get("/ws/chats/:id/regenerate", websocket.New(RegenerateAsync))
	routes.Put("/records/:id", ModifyRecord)

	// infer without login
	routes.Post("/inference", InferWithoutLogin)
	routes.Get("/ws/inference", websocket.New(InferWithoutLoginAsync))

	// OpenAI API protocol
	routes.Get("/v1/models", OpenAIListModels)
	routes.Get("/v1/models/:name", OpenAIRetrieveModel)
	routes.Post("/v1/chat/completions", OpenAICreateChatCompletion)

	// yocsef API
	routes.Get("/ws/yocsef/inference", websocket.New(InferYocsefAsyncAPI))
}
