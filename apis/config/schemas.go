package config

import "MOSS_backend/models"

type Response struct {
	Region              string                `json:"region"`
	InviteRequired      bool                  `json:"invite_required"`
	Notice              string                `json:"notice"`
	DefaultPluginConfig map[string]bool       `json:"default_plugin_config"`
	ModelConfig         []ModelConfigResponse `json:"model_config"`
}

type ModelConfigResponse struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

func FromModelConfig(modelConfig []models.ModelConfig) []ModelConfigResponse {
	var response []ModelConfigResponse
	for _, config := range modelConfig {
		response = append(response, ModelConfigResponse{
			ID:          config.ID,
			Description: config.Description,
		})
	}
	return response
}

type ModelConfigRequest struct {
	ID                       *int    `json:"id" validate:"min=1"`
	InnerThoughtsPostprocess *bool   `json:"inner_thoughts_postprocess" validate:"omitempty,oneof=true false"`
	Description              *string `json:"description" validate:"omitempty"`
}

type ModifyModelConfigRequest struct {
	InviteRequired *bool                 `json:"invite_required" validate:"omitempty,oneof=true false"`
	OffenseCheck   *bool                 `json:"offense_check" validate:"omitempty,oneof=true false"`
	Notice         *string               `json:"notice" validate:"omitempty"`
	ModelConfig    []*ModelConfigRequest `json:"model_config" validate:"omitempty"`
}
