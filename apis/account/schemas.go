package account

/* account */

type EmailModel struct {
	Email string `json:"email" query:"email" validate:"omitempty,email"`
}

type ScopeModel struct {
	Scope string `json:"scope" query:"scope" validate:"omitempty,oneof=register reset modify"`
}

type PhoneModel struct {
	Phone string `json:"phone" query:"phone" validate:"omitempty"` // phone number in e164 mode
}

type VerifyEmailRequest struct {
	EmailModel
	ScopeModel
	InviteCode *string `json:"invite_code" query:"invite_code" validate:"omitempty,min=1"`
}

type VerifyPhoneRequest struct {
	PhoneModel
	ScopeModel
	InviteCode *string `json:"invite_code" query:"invite_code" validate:"omitempty,min=1"`
}

type LoginRequest struct {
	*EmailModel `validate:"omitempty"`
	*PhoneModel `validate:"omitempty"`
	Password    string `json:"password" minLength:"8"`
}

type TokenResponse struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	Message string `json:"message"`
}

type RegisterRequest struct {
	LoginRequest
	Verification string  `json:"verification" minLength:"6" maxLength:"6" validate:"len=6"`
	InviteCode   *string `json:"invite_code" validate:"omitempty,min=1"`
}

type VerifyResponse struct {
	Message string `json:"message"`
	Scope   string `json:"scope" enums:"register,reset"`
}

type ModifyUserRequest struct {
	Nickname              *string `json:"nickname" validate:"omitempty,min=1"`
	ShareConsent          *bool   `json:"share_consent"`
	*EmailModel           `validate:"omitempty"`
	*PhoneModel           `validate:"omitempty"`
	Verification          string `json:"verification" minLength:"6" maxLength:"6" validate:"omitempty,len=6"`
	DisableSensitiveCheck *bool  `json:"disable_sensitive_check"`
}
