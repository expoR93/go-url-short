package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func(r *RedisStore) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func(r *RedisStore) Set(ctx context.Context, key string, value string) error {
	return r.client.Set(ctx, key, value, 24*time.Hour).Err()
}
