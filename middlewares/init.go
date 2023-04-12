package middlewares

import (
	"MOSS_backend/config"
	"MOSS_backend/models"
	"MOSS_backend/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"
	"time"
)

func RegisterMiddlewares(app *fiber.App) {
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	if config.Config.Mode != "bench" {
		app.Use(MyLogger)
	}
	app.Use(cors.New(cors.Config{AllowOrigins: "*"}))
	app.Use("/ws", func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Use(GetUserID)
}

func GetUserID(c *fiber.Ctx) error {
	userID, err := models.GetUserID(c)
	if err == nil {
		c.Locals("user_id", userID)
	}

	return c.Next()
}

func MyLogger(c *fiber.Ctx) error {
	startTime := time.Now()
	chainErr := c.Next()

	if chainErr != nil {
		if err := c.App().ErrorHandler(c, chainErr); err != nil {
			_ = c.SendStatus(fiber.StatusInternalServerError)
		}
	}

	latency := time.Since(startTime).Milliseconds()
	userID, ok := c.Locals("user_id").(int)
	output := []zap.Field{
		zap.Int("status_code", c.Response().StatusCode()),
		zap.String("method", c.Method()),
		zap.String("origin_url", c.OriginalURL()),
		zap.String("remote_ip", utils.GetRealIP(c)),
		zap.Int64("latency", latency),
	}
	if ok {
		output = append(output, zap.Int("user_id", userID))
	}
	if chainErr != nil {
		output = append(output, zap.Error(chainErr))
	}
	utils.Logger.Info("http log", output...)
	return nil
}
