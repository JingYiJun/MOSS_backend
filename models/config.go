package models

import (
	"MOSS_backend/config"
	"log"
	"time"
)

type ModelConfig struct {
	ID                       int             `json:"id"`
	InnerThoughtsPostprocess bool            `json:"inner_thoughts_postprocess" default:"false"`
	Description              string          `json:"description"`
	DefaultPluginConfig      map[string]bool `json:"default_plugin_config" gorm:"serializer:json"`
	Url                      string          `json:"url"`
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
		_ = config.SetCache(configCacheName, *configObjectPtr, configCacheExpire)
	}
	return nil
}

func UpdateConfig(configObjectPtr *Config) error {
	err := DB.Model(&Config{}).Updates(configObjectPtr).Error
	if err != nil {
		log.Println(err)
		return err
	}
	_ = config.SetCache(configCacheName, *configObjectPtr, configCacheExpire)
	return nil
}

func GetPluginConfig(modelID int) (map[string]bool, error) {
	var config Config
	if err := LoadConfig(&config); err != nil {
		return nil, err
	}
	for _, modelConfig := range config.ModelConfig {
		if modelConfig.ID == modelID {
			return modelConfig.DefaultPluginConfig, nil
		}
	}
	// if not found, return default config of first model
	return config.ModelConfig[0].DefaultPluginConfig, nil
}
