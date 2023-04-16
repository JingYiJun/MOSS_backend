package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

func initCache() {
	RedisClient = redis.NewClient(&redis.Options{
		Addr: Config.RedisUrl,
	})
	pong, err := RedisClient.Ping(context.Background()).Result()
	fmt.Println(pong, err)
}

func GetCache(key string, modelPtr any) error {
	data, err := RedisClient.Get(context.Background(), key).Bytes()
	if err != nil {
		if err == redis.Nil {
			log.Printf("cache not found %s", key)
		} else if err != nil {
			log.Printf("error get cache %s err %v", key, err)
		}
		return err
	}
	if len(data) == 0 {
		log.Printf("empty value of key %v", key)
		return errors.New("empty value")
	}

	err = json.Unmarshal(data, modelPtr)
	if err != nil {
		log.Printf("error get cache %s|%v err %v", key, string(data), err)
	}
	return err
}

func SetCache(key string, model any, duration time.Duration) error {
	data, err := json.Marshal(model)
	if err != nil {
		return err
	}
	duration = GenRandomDuration(duration)
	err = RedisClient.Set(context.Background(), key, data, duration).Err()
	if err != nil {
		log.Printf("error set cache %s|%v data(string): %v with duration %s, err %v",
			key, model, string(data), duration.String(), err)
	}
	return err
}

func DeleteCache(key string) error {
	return RedisClient.Del(context.Background(), key).Err()
}

func ClearCache() error {
	return RedisClient.FlushAll(context.Background()).Err()
}

func GenRandomDuration(delay time.Duration) time.Duration {
	return delay + time.Duration(rand.Int63n(int64(900*time.Second)))
}
