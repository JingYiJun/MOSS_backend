package models

import (
	"MOSS_backend/config"
	"log"
	"time"
)

type ModelConfig struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Url         string `json:"-"`
}

type Config struct {
	ID             int           `json:"-"`
	InviteRequired bool          `json:"invite_required"`
	OffenseCheck   bool          `json:"offense_check"`
	Notice         string        `json:"notice"`
	ModelConfig    []ModelConfig `json:"model_config"`
}

const configCacheName = "moss_backend_config"
const configCacheExpire = 24 * time.Hour

func LoadConfig(configObjectPtr *Config) error {
	if config.GetCache(configCacheName, configObjectPtr) != nil {
		if err := DB.First(configObjectPtr).Error; err != nil {
			return err
		}
		if err := DB.First(&(configObjectPtr.ModelConfig)).Error; err != nil {
			return err
		}
		err := config.SetCache(configCacheName, configObjectPtr, configCacheExpire)
		if err != nil {
			log.Println(err)
		}
	}
	return nil
}

func UpdateConfig(configObject *Config) error {
	err := DB.Model(&Config{}).Updates(configObject).Error
	if err != nil {
		return err
	}
	err = config.SetCache(configCacheName, configObject, configCacheExpire)
	if err != nil {
		log.Println(err)
	}
	return nil
}
