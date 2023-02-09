package apis

/* account */

type EmailModel struct {
	Email string `json:"email" query:"email" validate:"omitempty,email"`
}

type PhoneModel struct {
	Phone string `json:"phone" query:"phone" validate:"omitempty"` // phone number in e164 mode
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
	Verification string `json:"verification" minLength:"6" maxLength:"6" validate:"len=6"`
}

type VerifyResponse struct {
	Message string `json:"message"`
	Scope   string `json:"scope" enums:"register,reset"`
}

/* user account */

type ModifyUserRequest struct {
	Nickname     *string `json:"nickname" validate:"omitempty,min=1"`
	*EmailModel  `validate:"omitempty"`
	*PhoneModel  `validate:"omitempty"`
	Verification string `json:"verification" minLength:"6" maxLength:"6" validate:"len=6"`
}

type RecordCreateModel struct {
	Request string `json:"request" validate:"required"`
}

type RecordModifyModel struct {
	Feedback *string `json:"feedback"`
	Like     *int    `json:"like" validate:"omitempty,oneof=1 0 -1"` // 1 like, -1 dislike, 0 reset
}
