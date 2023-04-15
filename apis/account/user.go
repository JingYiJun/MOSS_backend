package account

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/auth"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// GetCurrentUser godoc
//
//	@Summary		get current user
//	@Tags			user
//	@Produce		json
//	@Router			/users/me [get]
//	@Success		200	{object}	User
//	@Failure		404	{object}	utils.MessageResponse	"User not found"
//	@Failure		500	{object}	utils.MessageResponse
func GetCurrentUser(c *fiber.Ctx) error {
	user, err := LoadUser(c)
	if err != nil {
		return err
	}
	return c.JSON(user)
}

// ModifyUser godoc
//
//	@Summary		modify user, need login
//	@Tags			user
//	@Produce		json
//	@Router			/users/me [put]
//	@Param			json	body		ModifyUserRequest	true	"json"
//	@Success		200		{object}	User
//	@Failure		500		{object}	utils.MessageResponse
func ModifyUser(c *fiber.Ctx) error {
	scope := "modify"
	var body ModifyUserRequest
	err := ValidateBody(c, &body)
	if err != nil {
		return err
	}

	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	var user User
	err = DB.Transaction(func(tx *gorm.DB) error {
		err = tx.Clauses(LockingClause).Take(&user, userID).Error
		if err != nil {
			return err
		}

		if body.Nickname != nil {
			user.Nickname = *body.Nickname
		}

		if body.ShareConsent != nil {
			user.ShareConsent = *body.ShareConsent
		}

		if body.EmailModel != nil && body.Email != user.Email {
			ok := auth.CheckVerificationCode(body.Email, scope, body.Verification)
			if !ok {
				return BadRequest("verification code error")
			}

			user.Email = body.Email
		}

		if body.PhoneModel != nil && body.Phone != user.Phone {
			ok := auth.CheckVerificationCode(body.Phone, scope, body.Verification)
			if !ok {
				return BadRequest("verification code error")
			}

			user.Phone = body.Phone
		}

		if body.DisableSensitiveCheck != nil {
			if !user.IsAdmin {
				return Forbidden()
			}
			user.DisableSensitiveCheck = *body.DisableSensitiveCheck
		}

		if body.PluginConfig != nil {
			if user.PluginConfig == nil {
				user.PluginConfig = make(map[string]bool)
			}
			for key, value := range body.PluginConfig {
				if _, ok := config.Config.DefaultPluginConfig[key]; ok {
					user.PluginConfig[key] = value
				}
			}
		}

		if body.ModelID != nil {
			if user.ModelID == 0 {
				user.ModelID = 1
			}
			user.ModelID = *body.ModelID
		}

		return tx.Save(&user).Error
	})

	if err != nil {
		return err
	}

	// redis update
	config.SetCache(GetUserCacheKey(user.ID), user, UserCacheExpire)

	return c.JSON(user)
}
