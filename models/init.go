package models

import (
	"MOSS_backend/config"
	"errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	"log"
	"os"
	"time"
)

var DB *gorm.DB

var gormConfig = &gorm.Config{
	NamingStrategy: schema.NamingStrategy{
		SingularTable: true, // use singular table name, table for `User` would be `user` with this option enabled
	},
	Logger: logger.New(
		log.Default(),
		logger.Config{
			SlowThreshold:             time.Second,  // 慢 SQL 阈值
			LogLevel:                  logger.Error, // 日志级别
			IgnoreRecordNotFoundError: true,         // 忽略ErrRecordNotFound（记录未找到）错误
			Colorful:                  false,        // 禁用彩色打印
		},
	),
}

func InitDB() {
	mysqlDB := func() (*gorm.DB, error) {
		return gorm.Open(mysql.Open(config.Config.DbUrl), gormConfig)
	}
	sqliteDB := func() (*gorm.DB, error) {
		err := os.MkdirAll("data", 0755)
		if err != nil && !os.IsExist(err) {
			panic(err)
		}
		return gorm.Open(sqlite.Open("data/sqlite.db"), gormConfig)
	}
	memoryDB := func() (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file::memory:?cache=shared"), gormConfig)
	}

	var err error

	// connect to database with different mode
	switch config.Config.Mode {
	case "production":
		DB, err = mysqlDB()
	case "dev":
		if config.Config.DbUrl == "" {
			DB, err = sqliteDB()
		} else {
			DB, err = mysqlDB()
		}
	case "test":
		DB, err = memoryDB()
	case "bench":
		if config.Config.DbUrl == "" {
			DB, err = memoryDB()
		} else {
			DB, err = mysqlDB()
		}
	default:
		panic("unsupported mode")
	}

	if err != nil {
		panic(err)
	}

	if config.Config.Mode == "dev" || config.Config.Mode == "test" {
		DB = DB.Debug()
	}

	// migrate database
	err = DB.AutoMigrate(
		User{},
		Chat{},
		Record{},
		ActiveStatus{},
		Config{},
		InviteCode{},
	)
	if err != nil {
		panic(err)
	}

	var configObject Config
	err = DB.First(&configObject).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		DB.Create(&configObject)
	}
}
