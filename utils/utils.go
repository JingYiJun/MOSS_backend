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

func Max[T constraints.Ordered](x, y T) T {
	if x > y {
		return x
	} else {
		return y
	}
}

func CutLastAny(s string, chars string) (before, after string, found bool) {
	sourceRunes := []rune(s)
	charRunes := []rune(chars)
	maxIndex := -1
	for _, char := range charRunes {
		index := -1
		for i, sourceRune := range sourceRunes {
			if char == sourceRune {
				index = i
			}
		}
		if index > 0 {
			maxIndex = Max(maxIndex, index)
		}
	}
	if maxIndex == -1 {
		return s, "", false
	} else {
		return string(sourceRunes[:maxIndex+1]), string(sourceRunes[maxIndex+1:]), true
	}
}
