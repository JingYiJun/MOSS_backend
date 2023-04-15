package config

type Response struct {
	Region              string          `json:"region"`
	InviteRequired      bool            `json:"invite_required"`
	Notice              string          `json:"notice"`
	DefaultPluginConfig map[string]bool `json:"default_plugin_config"`
}

type ModelConfigRequset struct {
	ID                       *int    `json:"id" validate:"min=1"`
	InnerThoughtsPostprocess *bool   `json:"inner_thoughts_postprocess" validate:"omitempty,oneof=true false"`
	Description              *string `json:"description" validate:"omitempty"`
}

type ModifyModelConfigRequest struct {
	InviteRequired *bool                 `json:"invite_required" validate:"omitempty,oneof=true false"`
	OffenseCheck   *bool                 `json:"offense_check" validate:"omitempty,oneof=true false"`
	Notice         *string               `json:"notice" validate:"omitempty"`
	ModelConfig    []*ModelConfigRequset `json:"model_config" validate:"omitempty"`
}
