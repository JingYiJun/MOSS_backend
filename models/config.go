package models

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"go.uber.org/zap"
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
	ID             int           `json:"id"`
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
		if err := DB.Find(&(configObjectPtr.ModelConfig)).Error; err != nil {
			return err
		}
		_ = config.SetCache(configCacheName, *configObjectPtr, configCacheExpire)
	}
	return nil
}

func UpdateConfig(configObjectPtr *Config) error {
	err := DB.Model(&Config{ID: 1}).Updates(configObjectPtr).Error
	if err != nil {
		utils.Logger.Error("failed to update config", zap.Error(err))
		return err
	}
	for i := range configObjectPtr.ModelConfig {
		err = DB.Model(&configObjectPtr.ModelConfig).Updates(&configObjectPtr.ModelConfig[i]).Error
		if err != nil {
			utils.Logger.Error("failed to update model config", zap.Error(err))
			return err
		}
	}
	_ = config.SetCache(configCacheName, *configObjectPtr, configCacheExpire)
	return nil
}

func GetPluginConfig(modelID int) (map[string]bool, error) {
	var configObject Config
	if err := LoadConfig(&configObject); err != nil {
		return nil, err
	}
	for _, modelConfig := range configObject.ModelConfig {
		if modelConfig.ID == modelID {
			return modelConfig.DefaultPluginConfig, nil
		}
	}
	// if not found, return default config of first model
	return configObject.ModelConfig[0].DefaultPluginConfig, nil
}
