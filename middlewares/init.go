package middlewares

import (
	"MOSS_backend/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func RegisterMiddlewares(app *fiber.App) {
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	if config.Config.Mode != "bench" {
		app.Use(logger.New())
	}
	if config.Config.Mode == "dev" {
		app.Use(cors.New(cors.Config{AllowOrigins: "*"})) // for swag docs
	}
}
