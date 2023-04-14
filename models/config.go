package models

import (
	"MOSS_backend/config"
	"log"
	"time"
)

type Config struct {
	ID             int           `json:"-"`
	InviteRequired bool          `json:"invite_required"`
	OffenseCheck   bool          `json:"offense_check"`
	Notice         string        `json:"notice"`
	ModelConfig    []ModelConfig `json:"model_config"`
}

var configCacheName = "moss_backend_config"

func LoadConfig() *Config {
	var configObject Config
	var modelConfig []ModelConfig
	if config.GetCache(configCacheName, &configObject) != nil {
		DB.First(&configObject)
		DB.First(&modelConfig)
		configObject.ModelConfig = modelConfig
		err := config.SetCache(configCacheName, configObject, 24*time.Hour)
		if err != nil {
			log.Println(err)
		}
	}
	return &configObject
}

type ModelConfig struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Url         string `json:"-"`
}
