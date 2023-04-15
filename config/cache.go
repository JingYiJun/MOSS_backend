package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	gocacheStore "github.com/eko/gocache/store/go_cache/v4"
	redisStore "github.com/eko/gocache/store/redis/v4"
	gocache "github.com/patrickmn/go-cache"
	"github.com/redis/go-redis/v9"
)

var Cache *cache.Cache[[]byte]

func initCache() {
	if Config.RedisUrl != "" {
		Cache = cache.New[[]byte](
			redisStore.NewRedis(
				redis.NewClient(
					&redis.Options{
						Addr: Config.RedisUrl,
					},
				),
			),
		)
		fmt.Println("using redis")
	} else {
		Cache = cache.New[[]byte](
			gocacheStore.NewGoCache(
				gocache.New(
					10*time.Minute,
					20*time.Minute),
			),
		)
		fmt.Println("using gocache")
	}
}

func GetCache(key string, model any) error {
	data, err := Cache.Get(context.Background(), key)
	if err != nil {
		return err
	}
	
	err = json.Unmarshal(data, &model)
	log.Printf("get cache %s| err %v", key, err)
	return err
}

func SetCache(key string, model any, duration time.Duration) error {
	data, err := json.Marshal(model)
	if err != nil {
		return err
	}
	duration = GenRandomDuration(duration)
	log.Printf("set cache %s| with duration %s", key, duration.String())
	return Cache.Set(context.Background(), key, data, store.WithExpiration(duration))
}

func DeleteCache(key string) error {
	return Cache.Delete(context.Background(), key)
}

func ClearCache() error {
	return Cache.Clear(context.Background())
}

func GenRandomDuration(delay time.Duration) time.Duration {
	return delay + time.Duration(rand.Int63n(int64(900 * time.Second)))
}
