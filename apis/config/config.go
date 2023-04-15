package config

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"github.com/gofiber/fiber/v2"
)

// GetConfig
// @Summary get global config
// @Tags Config
// @Produce json
// @Router /config [get]
// @Success 200 {object} Response
func GetConfig(c *fiber.Ctx) error {
	var configObject Config
	err := LoadConfig(&configObject)
	if err != nil {
		return err
	}
	
	var region string
	ok, err := IsInChina(GetRealIP(c))
	if err != nil {
		return err
	}
	if ok {
		region = "cn"
	} else {
		region = "global"
	}

	return c.JSON(Response{
		Region:              region,
		InviteRequired:      configObject.InviteRequired,
		Notice:              configObject.Notice,
		DefaultPluginConfig: config.Config.DefaultPluginConfig,
	})
}

// UpdateConfig
// @Summary update global config
// @Tags Config
// @Produce json
// @Router /config [put]
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Failure 500 {object} Response
func PatchConfig(c *fiber.Ctx) error {
	var configObject Config
	err := LoadConfig(&configObject)
	if err != nil {
		return InternalServerError("Failed to load config")
	}

	var patchData map[string]any
	if err := c.BodyParser(&patchData); err != nil {
		return BadRequest("Invalid JSON data for PatchConfig")
	}

	// 根据解析得到的数据，更新 configObject 中的相应字段
	for key, value := range patchData {
		switch key {
		case "invite_required":
			if v, ok := value.(bool); ok {
				configObject.InviteRequired = v
			} else {
				return BadRequest("Invalid JSON data: invite_required")
			}
		case "offense_check":
			if v, ok := value.(bool); ok {
				configObject.OffenseCheck = v
			} else {
				return BadRequest("Invalid JSON data: offense_check")
			}
		case "notice":
			if v, ok := value.(string); ok {
				configObject.Notice = v
			} else {
				return BadRequest("Invalid JSON data: notice")
			}
		case "model_config":
			if modelConfigs, ok := value.([]any); ok {
				for _, modelConfigData := range modelConfigs {
					if modelConfigMap, ok := modelConfigData.(map[string]interface{}); ok {
						modelID := int(modelConfigMap["id"].(float64))
						for i, modelConfig := range configObject.ModelConfig {
							if modelConfig.ID == modelID {
								if newURL, ok := modelConfigMap["url"].(string); ok {
									configObject.ModelConfig[i].Url = newURL
								}
								if newDescription, ok := modelConfigMap["description"].(string); ok {
									configObject.ModelConfig[i].Description = newDescription
								}
							}
						}
					}
				}
			} else {
				return BadRequest("Invalid JSON data: model_config")
			}
		}
	}

	// 将更新后的 configObject 保存到数据库中
	err = UpdateConfig(&configObject)
	if err != nil {
		return InternalServerError("Failed to update config")
	}

	return c.Status(200).JSON(fiber.Map{
		"success": "Config updated successfully",
	})
}