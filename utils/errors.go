package utils

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type MessageResponse struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type HttpError struct {
	Code    int          `json:"code,omitempty"`
	Message string       `json:"message,omitempty"`
	Detail  *ErrorDetail `json:"detail,omitempty"`
}

func (e *HttpError) Error() string {
	return e.Message
}

func BadRequest(messages ...string) *HttpError {
	message := "Bad Request"
	if len(messages) > 0 {
		message = messages[0]
	}
	return &HttpError{
		Code:    400,
		Message: message,
	}
}

func Unauthorized(messages ...string) *HttpError {
	message := "Invalid JWT Token"
	if len(messages) > 0 {
		message = messages[0]
	}
	return &HttpError{
		Code:    401,
		Message: message,
	}
}

func Forbidden(messages ...string) *HttpError {
	message := "您没有权限进行此操作"
	if len(messages) > 0 {
		message = messages[0]
	}
	return &HttpError{
		Code:    403,
		Message: message,
	}
}

func NotFound(messages ...string) *HttpError {
	message := "Not Found"
	if len(messages) > 0 {
		message = messages[0]
	}
	return &HttpError{
		Code:    404,
		Message: message,
	}
}

func InternalServerError(messages ...string) *HttpError {
	message := "Internal Server Error"
	if len(messages) > 0 {
		message = messages[0]
	}
	return &HttpError{
		Code:    500,
		Message: message,
	}
}

func MyErrorHandler(ctx *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}

	httpError := HttpError{
		Code:    500,
		Message: err.Error(),
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpError.Code = 404
	} else {
		switch e := err.(type) {
		case *HttpError:
			httpError = *e
		case *fiber.Error:
			httpError.Code = e.Code
		case *ErrorDetail:
			httpError.Code = 400
			httpError.Detail = e
		case fiber.MultiError:
			httpError.Code = 400
			httpError.Message = ""
			for _, err = range e {
				httpError.Message += err.Error() + "\n"
			}
		}
	}

	return ctx.Status(httpError.Code).JSON(&httpError)
}

type ErrCollection struct {
	ErrVerificationCodeInvalid error
	ErrNeedInviteCode          error
	ErrInviteCodeInvalid       error
	ErrRegistered              error
	ErrEmailRegistered         error
	ErrEmailNotRegistered      error
	ErrEmailCannotModify       error
	ErrEmailCannotReset        error
	ErrPhoneRegistered         error
	ErrPhoneNotRegistered      error
	ErrPhoneCannotModify       error
	ErrPhoneCannotReset        error
	ErrPasswordIncorrect       error
}

var ErrCollectionCN = ErrCollection{
	ErrVerificationCodeInvalid: BadRequest("验证码错误"),
	ErrNeedInviteCode:          BadRequest("需要邀请码"),
	ErrInviteCodeInvalid:       BadRequest("邀请码错误"),
	ErrRegistered:              BadRequest("您已注册，如果忘记密码，请使用忘记密码功能找回"),
	ErrEmailRegistered:         BadRequest("该邮箱已被注册"),
	ErrEmailNotRegistered:      BadRequest("该邮箱未注册"),
	ErrEmailCannotModify:       BadRequest("未登录状态，禁止修改邮箱"),
	ErrEmailCannotReset:        BadRequest("登录状态无法重置密码，请退出登录然后重试"),
	ErrPhoneRegistered:         BadRequest("该手机号已被注册"),
	ErrPhoneNotRegistered:      BadRequest("该手机号未注册"),
	ErrPhoneCannotModify:       BadRequest("未登录状态，禁止修改手机号"),
	ErrPhoneCannotReset:        BadRequest("登录状态无法重置密码，请退出登录然后重试"),
	ErrPasswordIncorrect:       Unauthorized("密码错误"),
}

var ErrCollectionGlobal = ErrCollection{
	ErrVerificationCodeInvalid: BadRequest("invalid verification code"),
	ErrNeedInviteCode:          BadRequest("invitation code needed"),
	ErrInviteCodeInvalid:       BadRequest("invalid invitation code"),
	ErrRegistered:              BadRequest("You have registered, if you forget your password, please use reset password function to retrieve"),
	ErrEmailRegistered:         BadRequest("email address registered"),
	ErrEmailNotRegistered:      BadRequest("email address not registered"),
	ErrEmailCannotModify:       BadRequest("cannot modify email address when not login"),
	ErrEmailCannotReset:        BadRequest("cannot reset password when login, please logout and retry"),
	ErrPhoneRegistered:         BadRequest("phone number registered"),
	ErrPhoneNotRegistered:      BadRequest("phone number not registered"),
	ErrPhoneCannotModify:       BadRequest("cannot modify phone number when not login"),
	ErrPhoneCannotReset:        BadRequest("cannot reset password when login, please logout and retry"),
	ErrPasswordIncorrect:       Unauthorized("password incorrect"),
}

type MessageCollection struct {
	MessageLoginSuccess          string
	MessageRegisterSuccess       string
	MessageLogoutSuccess         string
	MessageResetPasswordSuccess  string
	MessageVerificationEmailSend string
	MessageVerificationPhoneSend string
}

var MessageCollectionCN = MessageCollection{
	MessageLoginSuccess:          "登录成功",
	MessageRegisterSuccess:       "注册成功",
	MessageLogoutSuccess:         "登出成功",
	MessageResetPasswordSuccess:  "重置密码成功",
	MessageVerificationEmailSend: "验证邮件已发送，请查收\n如未收到，请检查邮件地址是否正确，检查垃圾箱，或重试",
	MessageVerificationPhoneSend: "验证短信已发送，请查收\n如未收到，请检查手机号是否正确，检查垃圾箱，或重试",
}

var MessageCollectionGlobal = MessageCollection{
	MessageLoginSuccess:          "Login successful",
	MessageRegisterSuccess:       "register successful",
	MessageLogoutSuccess:         "logout successful",
	MessageResetPasswordSuccess:  "reset password successful",
	MessageVerificationEmailSend: "The verification email has been sent, please check\nIf not, please check if the email address is correct, check the spam box, or try again",
	MessageVerificationPhoneSend: "The verification message has been sent, please check\nIf not, please check if the phone number is correct, check the spam box, or try again",
}

func GetInfoByIP(ip string) (*ErrCollection, *MessageCollection) {
	if ok, _ := IsInChina(ip); ok {
		return &ErrCollectionCN, &MessageCollectionCN
	} else {
		return &ErrCollectionGlobal, &MessageCollectionGlobal
	}
}
