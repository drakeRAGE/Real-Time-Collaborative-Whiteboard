// Package cache keeps the last 500 board ops in a Redis LIST.
package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const hotLimit = 500

// Key is the hot-list key for a room.
func Key(roomID string) string {
	return "board:" + roomID
}

// Cache is a Redis LIST of encoded ops, oldest at index 0.
type Cache struct {
	rdb redis.Cmdable
}

// New returns a cache bound to rdb.
func New(rdb redis.Cmdable) *Cache {
	return &Cache{rdb: rdb}
}

// Append pushes payload and trims the list to the last 500 entries.
func (c *Cache) Append(ctx context.Context, roomID string, payload []byte) error {
	key := Key(roomID)
	_, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.RPush(ctx, key, payload)
		p.LTrim(ctx, key, -hotLimit, -1)
		return nil
	})
	if err != nil {
		return fmt.Errorf("cache append: %w", err)
	}
	return nil
}

// Replace rewrites the list in the order of payloads.
func (c *Cache) Replace(ctx context.Context, roomID string, payloads [][]byte) error {
	key := Key(roomID)
	_, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, key)
		for _, payload := range payloads {
			p.RPush(ctx, key, payload)
		}
		if len(payloads) > hotLimit {
			p.LTrim(ctx, key, -hotLimit, -1)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cache replace: %w", err)
	}
	return nil
}

// Exists reports whether the hot list is present.
func (c *Cache) Exists(ctx context.Context, roomID string) (bool, error) {
	n, err := c.rdb.Exists(ctx, Key(roomID)).Result()
	if err != nil {
		return false, fmt.Errorf("cache exists: %w", err)
	}
	return n > 0, nil
}
