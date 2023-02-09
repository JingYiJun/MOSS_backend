package apis

import (
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
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}
	var user User
	err = DB.Take(&user, userID).Error
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
//	@Success		201		{object}	User
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

		if body.EmailModel != nil && body.Email != user.Email {
			ok, err := auth.CheckVerificationCode(body.Email, scope, body.Verification)
			if err != nil {
				return err
			}
			if !ok {
				return BadRequest("verification code error")
			}

			var found int64
			err = tx.Where("email = ?", body.Email).Count(&found).Error
			if err != nil {
				return err
			}
			if found > 0 {
				return BadRequest("this email has been registered")
			}

			user.Email = body.Email
		}

		if body.PhoneModel != nil && body.Phone != user.Phone {
			ok, err := auth.CheckVerificationCode(body.Phone, scope, body.Verification)
			if err != nil {
				return err
			}
			if !ok {
				return BadRequest("verification code error")
			}

			var found int64
			err = tx.Where("phone = ?", body.Phone).Count(&found).Error
			if err != nil {
				return err
			}
			if found > 0 {
				return BadRequest("this phone has been registered")
			}

			user.Phone = body.Phone
		}

		return tx.Save(&user).Error
	})
	if err != nil {
		return err
	}

	return c.Status(201).JSON(user)
}
