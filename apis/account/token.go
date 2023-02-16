package account

import (
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/auth"
	"MOSS_backend/utils/kong"
	"errors"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Login godoc
//
//	@Summary		Login
//	@Description	Login with email and password, return jwt token, not need jwt
//	@Tags			token
//	@Accept			json
//	@Produce		json
//	@Router			/login [post]
//	@Param			json	body		LoginRequest	true	"json"
//	@Success		200		{object}	TokenResponse
//	@Failure		400		{object}	utils.MessageResponse
//	@Failure		404		{object}	utils.MessageResponse	"User Not Found"
//	@Failure		500		{object}	utils.MessageResponse
func Login(c *fiber.Ctx) error {
	var body LoginRequest
	err := ValidateBody(c, &body)
	if err != nil {
		return err
	}

	errCollection, messageCollection := GetInfoByIP(GetRealIP(c))

	var user User
	if body.EmailModel != nil {
		err = DB.Where("email = ?", body.Email).Take(&user).Error
	} else if body.PhoneModel != nil {
		err = DB.Where("phone = ?", body.Phone).Take(&user).Error
	} else {
		return BadRequest()
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return NotFound("User Not Found")
		} else {
			return err
		}
	}

	ok, err := auth.CheckPassword(body.Password, user.Password)
	if err != nil {
		return err
	}
	if !ok {
		return errCollection.ErrPasswordIncorrect
	}

	// update login time and ip
	user.UpdateIP(GetRealIP(c))
	err = DB.Save(&user).Error
	if err != nil {
		return err
	}

	access, refresh, err := kong.CreateToken(&user)
	if err != nil {
		return err
	}

	return c.JSON(TokenResponse{
		Access:  access,
		Refresh: refresh,
		Message: messageCollection.MessageLoginSuccess,
	})
}

// Logout
//
//	@Summary		Logout
//	@Description	Logout, clear jwt credential and return successful message, logout, login required
//	@Tags			token
//	@Produce		json
//	@Router			/logout [get]
//	@Success		200	{object}	utils.MessageResponse
func Logout(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	_, messageCollection := GetInfoByIP(GetRealIP(c))

	var user User
	err = DB.Take(&user, userID).Error
	if err != nil {
		return err
	}

	err = kong.DeleteJwtCredential(userID)
	if err != nil {
		return err
	}

	return c.JSON(MessageResponse{Message: messageCollection.MessageLogoutSuccess})
}

// Refresh
//
//	@Summary		Refresh jwt token
//	@Description	Refresh jwt token with refresh token in header, login required
//	@Tags			token
//	@Produce		json
//	@Router			/refresh [post]
//	@Success		200	{object}	TokenResponse
func Refresh(c *fiber.Ctx) error {
	user, err := GetUserByRefreshToken(c)
	if err != nil {
		return err
	}

	// update login time and ip
	user.UpdateIP(GetRealIP(c))
	err = DB.Save(&user).Error
	if err != nil {
		return err
	}

	access, refresh, err := kong.CreateToken(user)
	if err != nil {
		return err
	}
	return c.JSON(TokenResponse{
		Access:  access,
		Refresh: refresh,
		Message: "refresh successful",
	})
}
