//	@title			Moss Backend
//	@version		0.0.1
//	@description	Moss Backend

//	@contact.name	Maintainer Chen Ke
//	@contact.url	https://danxi.fduhole.com/about
//	@contact.email	dev@fduhole.com

//	@license.name	Apache 2.0
//	@license.url	https://www.apache.org/licenses/LICENSE-2.0.html

//	@host		localhost:8000
//	@BasePath	/api

package main

import (
	"MOSS_backend/apis"
	"MOSS_backend/apis/record"
	"MOSS_backend/config"
	_ "MOSS_backend/docs"
	"MOSS_backend/middlewares"
	"MOSS_backend/models"
	"MOSS_backend/utils"
	"MOSS_backend/utils/auth"
	"MOSS_backend/utils/kong"
	"github.com/gofiber/fiber/v2"
	"github.com/robfig/cron/v3"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	config.InitConfig()
	models.InitDB()
	auth.InitCache()

	// connect to kong
	err := kong.Ping()
	if err != nil {
		panic(err)
	}

	app := fiber.New(fiber.Config{
		AppName:      config.AppName,
		ErrorHandler: utils.MyErrorHandler,
	})
	middlewares.RegisterMiddlewares(app)
	apis.RegisterRoutes(app)

	startTasks()

	go func() {
		err = app.Listen("0.0.0.0:8000")
		if err != nil {
			log.Println(err)
		}
	}()

	interrupt := make(chan os.Signal, 1)

	// wait for CTRL-C interrupt
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interrupt

	// close app
	err = app.Shutdown()
	if err != nil {
		log.Println(err)
	}

	_ = utils.Logger.Sync()
}

func startTasks() {
	c := cron.New()
	_, err := c.AddFunc("CRON_TZ=Asia/Shanghai 0 0 * * *", models.ActiveStatusTask) // run every day 00:00 +8:00
	if err != nil {
		panic(err)
	}
	go c.Start()
	go record.UserLockCheck()
}
