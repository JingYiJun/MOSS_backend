package config

import (
	"fmt"
	"github.com/caarlos0/env/v6"
)

var Config struct {
	Mode     string `env:"MODE" envDefault:"dev"`
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

	VerificationCodeExpires int `env:"VERIFICATION_CODE_EXPIRES" envDefault:"10"`
}

func InitConfig() {
	var err error
	if err = env.Parse(&Config); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", &Config)
}
