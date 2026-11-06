package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestAppendReplaceExists(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis %s: %v", addr, err)
	}
	room := uuid.NewString()
	t.Cleanup(func() { _ = rdb.Del(context.Background(), Key(room)).Err() })
	c := New(rdb)
	ok, err := c.Exists(ctx, room)
	if err != nil || ok {
		t.Fatalf("missing exists %v %v", ok, err)
	}
	if err := c.Append(ctx, room, []byte("a")); err != nil {
		t.Fatal(err)
	}
	ok, err = c.Exists(ctx, room)
	if err != nil || !ok {
		t.Fatalf("after append %v %v", ok, err)
	}
	payloads := make([][]byte, hotLimit+1)
	for i := range payloads {
		payloads[i] = []byte{byte(i)}
	}
	if err := c.Replace(ctx, room, payloads); err != nil {
		t.Fatal(err)
	}
	n, err := rdb.LLen(ctx, Key(room)).Result()
	if err != nil || n != hotLimit {
		t.Fatalf("len %d %v", n, err)
	}
}
