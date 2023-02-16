package record

type CreateModel struct {
	Request string `json:"request" validate:"required"`
}

type InterruptModel struct {
	Interrupt bool `json:"interrupt"`
}

type ModifyModel struct {
	Feedback *string `json:"feedback"`
	Like     *int    `json:"like" validate:"omitempty,oneof=1 0 -1"` // 1 like, -1 dislike, 0 reset
}
