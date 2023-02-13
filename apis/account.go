package apis

import (
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/auth"
	"MOSS_backend/utils/kong"
	"errors"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Register godoc
//
//	@Summary		register
//	@Description	register with email or phone, password and verification code
//	@Tags			account
//	@Accept			json
//	@Produce		json
//	@Router			/register [post]
//	@Param			json	body		RegisterRequest	true	"json"
//	@Success		201		{object}	TokenResponse
//	@Failure		400		{object}	utils.MessageResponse	"验证码错误、用户已注册"
//	@Failure		500		{object}	utils.MessageResponse
func Register(c *fiber.Ctx) error {
	scope := "register"
	var (
		body RegisterRequest
		ok   bool
	)
	err := ValidateBody(c, &body)
	if err != nil {
		return err
	}

	var (
		user       User
		registered = false
		deleted    = false
		inviteCode InviteCode
	)

	errCollection, messageCollection := GetInfoByIP(GetRealIP(c))

	// check invite code config
	var configObject Config
	err = DB.First(&configObject).Error
	if err != nil {
		return err
	}

	// check verification code first
	if body.PhoneModel != nil {
		ok = auth.CheckVerificationCode(body.Phone, scope, body.Verification)
	} else if body.EmailModel != nil {
		ok = auth.CheckVerificationCode(body.Email, scope, body.Verification)
	}
	if !ok {
		return errCollection.ErrVerificationCodeInvalid
	}

	// check Invite code
	if configObject.InviteRequired {
		if body.InviteCode == nil {
			return errCollection.ErrNeedInviteCode
		}
		err = DB.Take(&inviteCode, "code = ?", body.InviteCode).Error
		if err != nil || !inviteCode.IsSend || inviteCode.IsActivated {
			return errCollection.ErrInviteCodeInvalid
		}
	}

	if body.PhoneModel != nil {
		err = DB.Unscoped().Take(&user, "phone = ?", body.Phone).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			registered = false
			user.Phone = body.Phone
		} else {
			registered = true
			deleted = user.DeletedAt.Valid
		}
	} else if body.EmailModel != nil {
		err = DB.Unscoped().Take(&user, "email = ?", body.Email).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			registered = false
			user.Email = body.Email
		} else {
			registered = true
			deleted = user.DeletedAt.Valid
		}
	} else {
		return BadRequest()
	}

	user.Password, err = auth.MakePassword(body.Password)
	if err != nil {
		return err
	}
	remoteIP := GetRealIP(c)

	if registered {
		if deleted {
			err = DB.Unscoped().Model(&user).Update("DeletedAt", gorm.Expr("NULL")).Error
			if err != nil {
				return err
			}

			user.RegisterIP = remoteIP
			user.UpdateIP(remoteIP)
			user.ShareConsent = true
			// set invite code
			if configObject.InviteRequired {
				user.InviteCode = inviteCode.Code
			}
			err = DB.Unscoped().Model(&user).Select("RegisterIP", "LoginIP", "LastLoginIP").Updates(&user).Error
			if err != nil {
				return err
			}
		} else {
			return errCollection.ErrRegistered
		}
	} else {
		user.RegisterIP = remoteIP
		user.UpdateIP(remoteIP)
		user.ShareConsent = true

		// set invite code
		if configObject.InviteRequired {
			user.InviteCode = inviteCode.Code
		}

		err = DB.Create(&user).Error
		if err != nil {
			return err
		}

		err = kong.CreateUser(user.ID)
		if err != nil {
			return err
		}
	}

	// create kong token
	accessToken, refreshToken, err := kong.CreateToken(&user)
	if err != nil {
		return err
	}

	// delete verification
	if body.EmailModel != nil {
		_ = auth.DeleteVerificationCode(body.Email, scope)
	} else {
		_ = auth.DeleteVerificationCode(body.Phone, scope)
	}

	// update inviteCode
	if configObject.InviteRequired {
		inviteCode.IsActivated = true
		DB.Save(&inviteCode)
	}

	return c.JSON(TokenResponse{
		Access:  accessToken,
		Refresh: refreshToken,
		Message: messageCollection.MessageRegisterSuccess,
	})
}

// ChangePassword godoc
//
//	@Summary		reset password
//	@Description	reset password, reset jwt credential
//	@Tags			account
//	@Accept			json
//	@Produce		json
//	@Router			/register [put]
//	@Param			json	body		RegisterRequest	true	"json"
//	@Success		200		{object}	TokenResponse
//	@Failure		400		{object}	utils.MessageResponse	"验证码错误"
//	@Failure		500		{object}	utils.MessageResponse
func ChangePassword(c *fiber.Ctx) error {
	scope := "reset"
	var (
		body RegisterRequest
		ok   bool
	)
	err := ValidateBody(c, &body)
	if err != nil {
		return err
	}

	errCollection, messageCollection := GetInfoByIP(GetRealIP(c))

	if body.PhoneModel != nil {
		ok = auth.CheckVerificationCode(body.Phone, scope, body.Verification)
	} else if body.EmailModel != nil {
		ok = auth.CheckVerificationCode(body.Email, scope, body.Verification)
	}
	if !ok {
		return errCollection.ErrVerificationCodeInvalid
	}

	var user User
	err = DB.Transaction(func(tx *gorm.DB) error {
		querySet := tx.Clauses(clause.Locking{Strength: "UPDATE"})
		if body.PhoneModel != nil {
			querySet = querySet.Where("phone = ?", body.Phone)
		} else if body.EmailModel != nil {
			querySet = querySet.Where("email = ?", body.Email)
		} else {
			return BadRequest()
		}
		err = querySet.Take(&user).Error
		if err != nil {
			return err
		}

		user.Password, err = auth.MakePassword(body.Password)
		if err != nil {
			return err
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		return err
	}

	err = kong.DeleteJwtCredential(user.ID)
	if err != nil {
		return err
	}

	accessToken, refreshToken, err := kong.CreateToken(&user)
	if err != nil {
		return err
	}

	if body.EmailModel != nil {
		err = auth.DeleteVerificationCode(body.Email, scope)
	} else {
		err = auth.DeleteVerificationCode(body.Phone, scope)
	}
	if err != nil {
		return err
	}

	return c.JSON(TokenResponse{
		Access:  accessToken,
		Refresh: refreshToken,
		Message: messageCollection.MessageResetPasswordSuccess,
	})
}

// VerifyWithEmail godoc
//
//	@Summary		verify with email in query
//	@Description	verify with email in query, Send verification email
//	@Tags			account
//	@Produce		json
//	@Router			/verify/email [get]
//	@Param			email	query		VerifyEmailRequest	true	"email"
//	@Success		200		{object}	VerifyResponse
//	@Failure		400		{object}	utils.MessageResponse	"已注册“
func VerifyWithEmail(c *fiber.Ctx) error {
	var query VerifyEmailRequest
	err := ValidateQuery(c, &query)
	if err != nil {
		return err
	}

	errCollection, messageCollection := GetInfoByIP(GetRealIP(c))

	var (
		user  User
		scope string
		login bool
	)
	userID, _ := GetUserID(c)
	login = userID != 0

	err = DB.Take(&user, "email = ?", query.Email).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !login {
			scope = "register"
		} else {
			scope = "modify"
		}
	} else {
		if !login {
			scope = "reset"
		} else {
			return errCollection.ErrEmailRegistered
		}
	}
	if query.Scope != "" {
		if scope != query.Scope {
			switch scope {
			case "register":
				return errCollection.ErrEmailNotRegistered
			case "reset":
				switch query.Scope {
				case "register":
					return errCollection.ErrEmailRegistered
				case "modify":
					return errCollection.ErrEmailCannotModify
				default:
					return BadRequest()
				}
			case "modify":
				switch query.Scope {
				case "register":
					return errCollection.ErrEmailRegistered
				case "reset":
					return errCollection.ErrEmailCannotReset
				default:
					return BadRequest()
				}
			default:
				return BadRequest()
			}
		}
	}

	code, err := auth.SetVerificationCode(query.Email, scope)
	if err != nil {
		return err
	}

	err = SendCodeEmail(code, query.Email)
	if err != nil {
		return err
	}

	return c.JSON(VerifyResponse{
		Message: messageCollection.MessageVerificationEmailSend,
		Scope:   scope,
	})
}

// VerifyWithPhone godoc
//
//	@Summary		verify with phone in query
//	@Description	verify with phone in query, Send verification message
//	@Tags			account
//	@Produce		json
//	@Router			/verify/phone [get]
//	@Param			phone	query		VerifyPhoneRequest	true	"phone"
//	@Success		200		{object}	VerifyResponse
//	@Failure		400		{object}	utils.MessageResponse	"已注册“
func VerifyWithPhone(c *fiber.Ctx) error {
	var query VerifyPhoneRequest
	err := ValidateQuery(c, &query)
	if err != nil {
		return BadRequest("invalid phone number")
	}

	errCollection, messageCollection := GetInfoByIP(GetRealIP(c))

	var (
		user  User
		scope string
		login bool
	)
	userID, _ := GetUserID(c)
	login = userID != 0
	err = DB.Take(&user, "phone = ?", query.Phone).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !login {
			scope = "register" // 未注册、未登录
		} else {
			scope = "modify" // 未注册、已登录
		}
	} else {
		if !login {
			scope = "reset" // 已注册、未登录
		} else {
			return errCollection.ErrPhoneRegistered // 已注册、已登录
		}
	}

	if query.Scope != "" {
		if scope != query.Scope {
			switch scope {
			case "register":
				return errCollection.ErrPhoneNotRegistered
			case "reset":
				switch query.Scope {
				case "register":
					return errCollection.ErrPhoneRegistered
				case "modify":
					return errCollection.ErrPhoneCannotModify
				default:
					return BadRequest()
				}
			case "modify":
				switch query.Scope {
				case "register":
					return errCollection.ErrPhoneRegistered
				case "reset":
					return errCollection.ErrPhoneCannotReset
				default:
					return BadRequest()
				}
			default:
				return BadRequest()
			}
		}
	}
	code, err := auth.SetVerificationCode(query.Phone, scope)
	if err != nil {
		return err
	}

	err = SendCodeMessage(code, query.Phone)
	if err != nil {
		return err
	}

	return c.JSON(VerifyResponse{
		Message: messageCollection.MessageVerificationPhoneSend,
		Scope:   scope,
	})
}

// DeleteUser godoc
//
//	@Summary		delete user
//	@Description	delete user and related jwt credentials
//	@Tags			account
//	@Router			/users/me [delete]
//	@Param			json	body	LoginRequest	true	"email, password"
//	@Success		204
//	@Failure		400	{object}	utils.MessageResponse	"密码错误“
//	@Failure		404	{object}	utils.MessageResponse	"用户不存在“
//	@Failure		500	{object}	utils.MessageResponse
func DeleteUser(c *fiber.Ctx) error {
	var body LoginRequest
	err := ValidateBody(c, &body)
	if err != nil {
		return err
	}

	errCollection, _ := GetInfoByIP(GetRealIP(c))

	var user User
	err = DB.Transaction(func(tx *gorm.DB) error {
		querySet := tx.Clauses(clause.Locking{Strength: "UPDATE"})
		if body.PhoneModel != nil {
			querySet = querySet.Where("phone = ?", body.Phone)
		} else if body.EmailModel != nil {
			querySet = querySet.Where("email = ?", body.Email)
		} else {
			return BadRequest()
		}
		err = querySet.Take(&user).Error
		if err != nil {
			return err
		}

		ok, err := auth.CheckPassword(body.Password, user.Password)
		if err != nil {
			return err
		}
		if !ok {
			return errCollection.ErrPasswordIncorrect
		}

		return tx.Delete(&user).Error
	})

	err = kong.DeleteJwtCredential(user.ID)
	if err != nil {
		return err
	}

	return c.SendStatus(204)
}
