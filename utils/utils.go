package utils

import "github.com/gofiber/fiber/v2"

func GetRealIP(c *fiber.Ctx) string {
	IPs := c.IPs()
	if len(IPs) > 0 {
		return IPs[0]
	} else {
		return c.Get("X-Real-Ip", c.IP())
	}
}
