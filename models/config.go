package models

import (
	"MOSS_backend/config"
	"log"
	"time"
)

type ModelConfig struct {
	ID                       int    `json:"id"`
	InnerThoughtsPostprocess bool   `json:"inner_thoughts_postprocess" default:"false"`
	Description              string `json:"description"`
	Url                      string `json:"-"`
}

func (cfg *ModelConfig) TableName() string {
	return "language_model_config"
}

type Config struct {
	ID             int           `json:"-"`
	InviteRequired bool          `json:"invite_required"`
	OffenseCheck   bool          `json:"offense_check"`
	Notice         string        `json:"notice"`
	ModelConfig    []ModelConfig `json:"model_config" gorm:"-:all"`
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
		err := config.SetCache(configCacheName, *configObjectPtr, configCacheExpire)
		if err != nil {
			log.Println(err)
		}
		log.Printf("Config loaded from database configObjectPtr.InviteRequired:%v", configObjectPtr.InviteRequired)
	} else {
		log.Printf("Config loaded from cache configObjectPtr.InviteRequired:%v", configObjectPtr.InviteRequired)
	}
	return nil
}

func UpdateConfig(configObjectPtr *Config) error {
	err := DB.Model(&Config{}).Updates(configObjectPtr).Error
	if err != nil {
		return err
	}
	err = config.SetCache(configCacheName, *configObjectPtr, configCacheExpire)
	if err != nil {
		log.Println(err)
	}
	return nil
}
