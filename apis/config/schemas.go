package config

type Response struct {
	Region              string          `json:"region"`
	InviteRequired      bool            `json:"invite_required"`
	Notice              string          `json:"notice"`
	DefaultPluginConfig map[string]bool `json:"default_plugin_config"`
}
