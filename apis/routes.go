package apis

import (
	"MOSS_backend/apis/account"
	"MOSS_backend/apis/chat"
	"MOSS_backend/apis/config"
	"MOSS_backend/apis/record"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

func RegisterRoutes(app *fiber.App) {
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/api")
	})
	// docs
	app.Get("/docs", func(c *fiber.Ctx) error {
		return c.Redirect("/docs/index.html")
	})
	app.Get("/docs/*", swagger.HandlerDefault)

	// meta
	routes := app.Group("/api")
	routes.Get("/", Index)

	account.RegisterRoutes(routes)
	chat.RegisterRoutes(routes)
	record.RegisterRoutes(routes)
	config.RegisterRoutes(routes)
}
