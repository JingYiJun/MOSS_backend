package apis

import (
	"MOSS_backend/data"
	"github.com/gofiber/fiber/v2"
)

// Index
//
//	@Produce	application/json
//	@Router		/ [get]
//	@Success	200	{object}	models.Map
func Index(c *fiber.Ctx) error {
	return c.Send(data.MetaData)
}
