package utils

import (
	"github.com/gofiber/fiber/v2"
	"golang.org/x/exp/constraints"
)

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

func StripContent(content string, length int) string {
	return string([]rune(content)[:Min(len([]rune(content)), length)])
}

func Min[T constraints.Ordered](x, y T) T {
	if x < y {
		return x
	} else {
		return y
	}
}
