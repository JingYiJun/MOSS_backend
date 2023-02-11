package auth

import (
	"MOSS_backend/config"
	"context"
	"crypto/rand"
	"fmt"
	"github.com/eko/gocache/lib/v4/cache"
	gocacheStore "github.com/eko/gocache/store/go_cache/v4"
	redisStore "github.com/eko/gocache/store/redis/v4"
	"github.com/go-redis/redis/v8"
	gocache "github.com/patrickmn/go-cache"
	"math/big"
	"time"
)

var verificationCodeCache *cache.Cache[string]

func init() {
	if config.Config.RedisUrl != "" {
		verificationCodeCache = cache.New[string](
			redisStore.NewRedis(
				redis.NewClient(
					&redis.Options{
						Addr: config.Config.RedisUrl,
					},
				),
			),
		)
	} else {
		verificationCodeCache = cache.New[string](
			gocacheStore.NewGoCache(
				gocache.New(
					time.Duration(config.Config.VerificationCodeExpires)*time.Minute,
					20*time.Minute),
			),
		)
	}
}

// SetVerificationCode 缓存中设置验证码，key = {scope}-{info}
func SetVerificationCode(info, scope string) (string, error) {
	codeInt, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", codeInt.Uint64())

	return code, verificationCodeCache.Set(
		context.Background(),
		fmt.Sprintf("%v-%v", scope, info),
		code,
	)
}

// CheckVerificationCode 检查验证码
func CheckVerificationCode(info, scope, code string) bool {
	storedCode, err := verificationCodeCache.Get(
		context.Background(),
		fmt.Sprintf("%v-%v", scope, info),
	)
	return err == nil && storedCode == code
}

func DeleteVerificationCode(info, scope string) error {
	return verificationCodeCache.Delete(
		context.Background(),
		fmt.Sprintf("%v-%v", scope, info),
	)
}
