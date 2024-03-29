package config

import "github.com/gofiber/fiber/v2"

func RegisterRoutes(routes fiber.Router) {
	routes.Get("/config", GetConfig)
	// redis update & config update
	routes.Patch("/config", PatchConfig)
}
