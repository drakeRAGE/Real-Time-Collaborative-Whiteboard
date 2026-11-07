// Package redisx provides a go-redis client, a Lua token bucket, and a protobuf cache.
package redisx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

// ErrMiss is returned when a cache key is absent or expired.
var ErrMiss = errors.New("redisx: cache miss")

// New returns a go-redis v9 client. addr is either host:port or a
// redis:// / rediss:// URL (password, DB, and TLS for rediss).
func New(addr string) (*redis.Client, error) {
	if !strings.HasPrefix(addr, "redis://") && !strings.HasPrefix(addr, "rediss://") {
		return redis.NewClient(&redis.Options{Addr: addr}), nil
	}
	opts, err := redis.ParseURL(addr)
	if err != nil {
		return nil, fmt.Errorf("redisx: parse url: %w", err)
	}
	return redis.NewClient(opts), nil
}

// RateLimiter is a Redis-backed token bucket.
type RateLimiter struct {
	rdb redis.Cmdable
}

// NewRateLimiter returns a limiter that stores bucket state in rdb.
func NewRateLimiter(rdb redis.Cmdable) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// tokenBucketLua refills `capacity` tokens across window_ms and consumes one.
// KEYS[1] bucket key
// ARGV[1] capacity, ARGV[2] window milliseconds, ARGV[3] now unix milliseconds
const tokenBucketLua = `
local capacity = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local data = redis.call("HMGET", KEYS[1], "tokens", "ts")
local tokens = tonumber(data[1])
local ts = tonumber(data[2])

if tokens == nil or ts == nil then
  tokens = capacity
  ts = now
else
  local elapsed = now - ts
  if elapsed < 0 then
    elapsed = 0
  end
  local add = math.floor((elapsed * capacity) / window_ms)
  if add > 0 then
    if tokens + add > capacity then
      tokens = capacity
    else
      tokens = tokens + add
    end
    ts = now
  end
end

local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

redis.call("HSET", KEYS[1], "tokens", tokens, "ts", ts)
redis.call("PEXPIRE", KEYS[1], window_ms)
return allowed
`

// Allow consumes one token for key. limit is the bucket capacity refilled over window.
// The sixth call in a window of five is denied. After the key TTL elapses, calls are allowed again.
func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 || window < time.Millisecond {
		return false, fmt.Errorf("redisx: limit and window must be positive")
	}
	n, err := l.rdb.Eval(
		ctx,
		tokenBucketLua,
		[]string{key},
		limit,
		window.Milliseconds(),
		time.Now().UnixMilli(),
	).Int()
	if err != nil {
		return false, fmt.Errorf("redisx: allow: %w", err)
	}
	return n == 1, nil
}

// Cache stores protobuf messages under a TTL.
type Cache[T proto.Message] struct {
	rdb    redis.Cmdable
	newMsg func() T
}

// NewCache returns a cache. newMsg must return an empty message to unmarshal into.
func NewCache[T proto.Message](rdb redis.Cmdable, newMsg func() T) *Cache[T] {
	return &Cache[T]{rdb: rdb, newMsg: newMsg}
}

// Set marshals msg and stores it at key until ttl elapses.
func (c *Cache[T]) Set(ctx context.Context, key string, msg T, ttl time.Duration) error {
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("redisx: marshal: %w", err)
	}
	if err := c.rdb.Set(ctx, key, b, ttl).Err(); err != nil {
		return fmt.Errorf("redisx: set: %w", err)
	}
	return nil
}

// Get returns the message stored at key. A missing or expired key returns ErrMiss.
func (c *Cache[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	if c.newMsg == nil {
		return zero, errors.New("redisx: nil message constructor")
	}
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return zero, ErrMiss
	}
	if err != nil {
		return zero, fmt.Errorf("redisx: get: %w", err)
	}
	msg := c.newMsg()
	if err := proto.Unmarshal(b, msg); err != nil {
		return zero, fmt.Errorf("redisx: unmarshal: %w", err)
	}
	return msg, nil
}
