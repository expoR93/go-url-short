package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisStore(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	ttl := 10 * time.Minute
	store := NewRedisStore(mr.Addr(), "", 0, ttl)
	ctx := context.Background()

	t.Run("Set and Get", func(t *testing.T) {
		key := "test-key"
		val := "https://example.com"

		// Test Set
		err := store.Set(ctx, key, val)
		if err != nil {
			t.Errorf("failed to set value: %v", err)
		}

		// Test Get
		got, err := store.Get(ctx, key)
		if err != nil {
			t.Errorf("failed to get value: %v", err)
		}
		if got != val {
			t.Errorf("expected %s, got %s", val, got)
		}

	})

	t.Run("TTL Verification", func(t *testing.T) {
		key := "ttl-key"
		val := "ttl-val"

		_ = store.Set(ctx, key, val)

		// Fast-forward time in miniredis
		mr.FastForward(ttl + time.Second)

		_, err := store.Get(ctx, key)
		if err == nil {
			t.Error("expected key to be expired, but it still exists")
		}
	})
}
