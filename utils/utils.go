package utils

import (
	"github.com/gofiber/fiber/v2"
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
	return string([]rune(content)[:min(len([]rune(content)), length)])
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
			maxIndex = max(maxIndex, index)
		}
	}
	if maxIndex == -1 {
		return s, "", false
	} else {
		return string(sourceRunes[:maxIndex+1]), string(sourceRunes[maxIndex+1:]), true
	}
}

type JSONReader interface {
	ReadJSON(any) error
}

type JSONWriter interface {
	WriteJSON(any) error
}

type JsonReaderWriter interface {
	JSONReader
	JSONWriter
}
