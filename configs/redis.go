package configs

import (
	"go-boilerplate/internal/utils"
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var RedisClient *redis.Client

func InitRedis() {
	if !Cfg.RedisEnabled {
		utils.Logger.Info("redis disabled")
		return
	}

	opts := &redis.Options{Addr: "localhost:6379"}
	if Cfg.RedisURL != "" {
		parsed, err := redis.ParseURL(Cfg.RedisURL)
		if err != nil {
			utils.Logger.Fatal("invalid REDIS_URL", zap.Error(err))
		}
		opts = parsed
	}

	RedisClient = redis.NewClient(opts)
	if err := RedisClient.Ping(context.Background()).Err(); err != nil {
		utils.Logger.Fatal("failed to connect to redis", zap.Error(err))
	}
	utils.Logger.Info("connected to redis")
}

func CacheSet(ctx context.Context, key string, value any, ttl time.Duration) error {
	if RedisClient == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return RedisClient.Set(ctx, key, data, ttl).Err()
}

func CacheGet(ctx context.Context, key string) (string, error) {
	if RedisClient == nil {
		return "", redis.Nil
	}
	return RedisClient.Get(ctx, key).Result()
}

func CacheDel(ctx context.Context, keyPattern string) error {
	if RedisClient == nil {
		return nil
	}
	keys, err := RedisClient.Keys(ctx, keyPattern+"*").Result()
	if err != nil || len(keys) == 0 {
		return err
	}
	return RedisClient.Del(ctx, keys...).Err()
}
