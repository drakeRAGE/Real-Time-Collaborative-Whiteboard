package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestAllowChat(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis %s: %v", addr, err)
	}
	lim := New(rdb)
	user := uuid.New()
	for i := 0; i < chatLimit; i++ {
		ok, err := lim.AllowChat(ctx, user)
		if err != nil || !ok {
			t.Fatalf("allow %d: %v %v", i, ok, err)
		}
	}
	ok, err := lim.AllowChat(ctx, user)
	if err != nil || ok {
		t.Fatalf("over limit ok=%v err=%v", ok, err)
	}
}
