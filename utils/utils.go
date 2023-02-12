package utils

import "github.com/gofiber/fiber/v2"

type CanPreprocess interface {
	Preprocess(c *fiber.Ctx) error
}

func Serialize(c *fiber.Ctx, obj CanPreprocess) error {
	err := obj.Preprocess(c)
	if err != nil {
		return err
	}
	return c.JSON(obj)
}

func GetRealIP(c *fiber.Ctx) string {
	IPs := c.IPs()
	if len(IPs) > 0 {
		return IPs[0]
	} else {
		return c.Get("X-Real-Ip", c.IP())
	}
}
