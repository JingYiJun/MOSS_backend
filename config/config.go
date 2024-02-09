package config

import (
	"fmt"

	"github.com/caarlos0/env/v8"
)

const AppName = "moss_backend"

var Config struct {
	Mode     string `env:"MODE" envDefault:"dev"`
	Debug    bool   `env:"DEBUG" envDefault:"false"`
	Hostname string `env:"HOSTNAME,required"`
	DbUrl    string `env:"DB_URL,required"`
	KongUrl  string `env:"KONG_URL,required"`
	RedisUrl string `env:"REDIS_URL"`
	// sending email config
	EmailUrl          string `env:"EMAIL_URL,required"`
	TencentSecretID   string `env:"SECRET_ID,required"`
	TencentSecretKey  string `env:"SECRET_KEY,required"`
	TencentTemplateID uint64 `env:"TEMPLATE_ID,required"`
	// sending message config
	UniAccessID   string `env:"UNI_ACCESS_ID,required"`
	UniSignature  string `env:"UNI_SIGNATURE" envDefault:"fastnlp"`
	UniTemplateID string `env:"UNI_TEMPLATE_ID,required"`

	// InferenceUrl string `env:"INFERENCE_URL,required"` // now save it in db

	// 敏感信息检测
	EnableSensitiveCheck   bool   `env:"ENABLE_SENSITIVE_CHECK" envDefault:"true"`
	SensitiveCheckPlatform string `env:"SENSITIVE_CHECK_PLATFORM" envDefault:"ShuMei"` // one of ShuMei or DiTing

	// 谛听平台
	DiTingToken string `env:"SENSITIVE_CHECK_TOKEN"`

	// 数美平台
	ShuMeiAccessKey string `env:"SHU_MEI_ACCESS_KEY"`
	ShuMeiAppID     string `env:"SHU_MEI_APP_ID"`
	ShuMeiEventID   string `env:"SHU_MEI_EVENT_ID"`
	ShuMeiType      string `env:"SHU_MEI_TYPE"`

	VerificationCodeExpires int `env:"VERIFICATION_CODE_EXPIRES" envDefault:"10"`
	ChatNameLength          int `env:"CHAT_NAME_LENGTH" envDefault:"30"`

	AccessExpireTime  int `env:"ACCESS_EXPIRE_TIME" envDefault:"30"`  // 30 minutes
	RefreshExpireTime int `env:"REFRESH_EXPIRE_TIME" envDefault:"30"` // 30 days

	CallbackUrl string `env:"CALLBACK_URL,required"` // async callback url

	OpenScreenshot bool `env:"OPEN_SCREENSHOT" envDefault:"true"`

	PassSensitiveCheckUsername []string `env:"PASS_SENSITIVE_CHECK_USERNAME"`

	// tools
	EnableTools       bool   `env:"ENABLE_TOOLS" envDefault:"true"`
	ToolsSearchUrl    string `env:"TOOLS_SEARCH_URL,required"`
	ToolsCalculateUrl string `env:"TOOLS_CALCULATE_URL,required"`
	ToolsSolveUrl     string `env:"TOOLS_SOLVE_URL,required"`
	ToolsDrawUrl      string `env:"TOOLS_DRAW_URL,required"`

	// DefaultPluginConfig map[string]bool `env:"DEFAULT_PLUGIN_CONFIG"`

	// InnerThoughtsPostprocess bool `env:"INNER_THOUGHTS_POSTPROCESS" envDefault:"false"`

	DefaultModelID int `env:"DEFAULT_MODEL_ID" envDefault:"1"`
}

func InitConfig() {
	var err error
	if err = env.Parse(&Config); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", &Config)

	initCache()
}
