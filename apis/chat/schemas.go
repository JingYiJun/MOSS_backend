package chat

type ModifyModel struct {
	Name *string `json:"name" validate:"omitempty,min=1"`
}
