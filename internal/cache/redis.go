package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client     *redis.Client
	defaultTTL time.Duration
}

func NewRedisStore(addr, password string, db int, defaultTTL time.Duration) *RedisStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &RedisStore{
		client:     rdb,
		defaultTTL: defaultTTL,
	}
}

func (r *RedisStore) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisStore) Set(ctx context.Context, key string, value string) error {
	return r.client.Set(ctx, key, value, r.defaultTTL).Err()
}

func (s *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	// Atomic increment
	val, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// If it's a new key, set the TTL
	if val == 1 {
		s.client.Expire(ctx, key, ttl)
	}

	return val, nil
}