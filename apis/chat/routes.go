package chat

import (
	"MOSS_backend/apis/record"
	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(routes fiber.Router) {
	// chat
	routes.Get("/chats", ListChats)
	routes.Post("/chats", AddChat)
	routes.Put("/chats/:id/regenerate", record.RetryRecord)
	routes.Put("/chats/:id", ModifyChat)
	routes.Delete("/chats/:id", DeleteChat)
	routes.Get("/chats/:id/screenshot.png", GenerateChatScreenshot)
}
