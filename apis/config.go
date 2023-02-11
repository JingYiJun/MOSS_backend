package apis

import (
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"github.com/gofiber/fiber/v2"
)

// GetConfig
// @Summary get global config
// @Tags Config
// @Produce json
// @Router /config [get]
// @Success 200 {object} ConfigResponse
func GetConfig(c *fiber.Ctx) error {
	var configObject Config
	err := DB.First(&configObject).Error
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

	return c.JSON(ConfigResponse{
		Region:         region,
		InviteRequired: configObject.InviteRequired,
	})
}
