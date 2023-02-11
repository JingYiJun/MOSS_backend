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

	if body.PhoneModel != nil {
		ok, err = auth.CheckVerificationCode(body.Phone, scope, body.Verification)
	} else if body.EmailModel != nil {
		ok, err = auth.CheckVerificationCode(body.Email, scope, body.Verification)
	}
	if err != nil {
		return err
	}
	if !ok {
		return BadRequest("verification code error")
	}

	var (
		user       User
		registered = false
		deleted    = false
	)

	if body.PhoneModel != nil {
		err = DB.Unscoped().Take(&user, "phone = ?", body.Phone).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			registered = false
			user.Phone = body.Phone
		} else {
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
			err = DB.Unscoped().Model(&user).Select("RegisterIP", "LoginIP", "LastLoginIP").Updates(&user).Error
			if err != nil {
				return err
			}
		} else {
			return BadRequest("该用户已注册，如果忘记密码，请使用忘记密码功能找回")
		}
	} else {
		user.RegisterIP = remoteIP
		user.UpdateIP(remoteIP)
		user.ShareConsent = true
		err = DB.Create(&user).Error
		if err != nil {
			return err
		}

		err = kong.CreateUser(user.ID)
		if err != nil {
			return err
		}
	}

	accessToken, refreshToken, err := kong.CreateToken(&user)
	if err != nil {
		return err
	}

	err = auth.DeleteVerificationCode(body.Email, scope)
	if err != nil {
		return err
	}

	return c.JSON(TokenResponse{
		Access:  accessToken,
		Refresh: refreshToken,
		Message: "register successful",
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

	if body.PhoneModel != nil {
		ok, err = auth.CheckVerificationCode(body.Phone, scope, body.Verification)
	} else if body.EmailModel != nil {
		ok, err = auth.CheckVerificationCode(body.Email, scope, body.Verification)
	}
	if err != nil {
		return err
	}
	if !ok {
		return BadRequest("验证码错误")
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

	err = auth.DeleteVerificationCode(body.Email, scope)
	if err != nil {
		return err
	}

	return c.JSON(TokenResponse{
		Access:  accessToken,
		Refresh: refreshToken,
		Message: "reset password successful",
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
		return BadRequest("invalid email")
	}

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
			return BadRequest("该邮箱已被注册")
		}
	}
	if query.Scope != "" {
		if scope != query.Scope {
			switch scope {
			case "register":
				return BadRequest("该邮箱未注册")
			case "reset":
				switch query.Scope {
				case "register":
					return BadRequest("该邮箱已被注册")
				case "modify":
					return BadRequest("未登录状态，禁止修改邮箱")
				default:
					return BadRequest()
				}
			case "modify":
				switch query.Scope {
				case "register":
					return BadRequest("该邮箱已被注册")
				case "reset":
					return BadRequest("登录状态无法重置密码，请退出登录然后重试")
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
		Message: "验证邮件已发送，请查收\n如未收到，请检查邮件地址是否正确，检查垃圾箱，或重试",
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
			return BadRequest("该手机号已被注册") // 已注册、已登录
		}
	}

	if query.Scope != "" {
		if scope != query.Scope {
			switch scope {
			case "register":
				return BadRequest("该手机号未注册")
			case "reset":
				switch query.Scope {
				case "register":
					return BadRequest("该手机号已被注册")
				case "modify":
					return BadRequest("未登录状态，禁止修改手机号")
				default:
					return BadRequest()
				}
			case "modify":
				switch query.Scope {
				case "register":
					return BadRequest("该手机号已被注册")
				case "reset":
					return BadRequest("登录状态无法重置密码，请退出登录然后重试")
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
		Message: "验证短信已发送，请查收\n如未收到，请检查手机号是否正确，检查垃圾箱，或重试",
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
			return Forbidden("password incorrect")
		}

		return tx.Delete(&user).Error
	})

	err = kong.DeleteJwtCredential(user.ID)
	if err != nil {
		return err
	}

	return c.SendStatus(204)
}
