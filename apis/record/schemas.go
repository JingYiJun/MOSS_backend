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

type InferenceRequest struct {
	Context string `json:"context"`
	Request string `json:"request" validate:"min=1"`
}

type InferenceResponse struct {
	Response  string `json:"response"`
	Context   string `json:"context"`
	ExtraData any    `json:"extra_data"`
}
