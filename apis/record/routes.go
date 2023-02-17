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
	routes.Put("/records/:id", ModifyRecord)

	// infer response
	routes.Get("/ws/response", websocket.New(ReceiveInferResponse))
}
