package apis

import (
	"MOSS_backend/models"
	"github.com/gofiber/fiber/v2"
)

// Index
//
//	@Produce	application/json
//	@Success	200	{object}	Info
//	@Router		/ [get]
func Index(c *fiber.Ctx) error {
	return c.JSON(models.Map{
		"name":    "MOSS backend",
		"version": "0.0.1",
		"author":  "JingYiJun",
		"email":   "dev@fduhole.com",
		"license": "Apache-2.0",
	})
}
