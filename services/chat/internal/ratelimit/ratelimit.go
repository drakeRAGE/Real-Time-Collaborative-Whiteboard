package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
)

const chatLimit = 5

var chatWindow = 10 * time.Second

// Chat wraps redisx for chat ingest rate limits.
type Chat struct {
	rl *redisx.RateLimiter
}

// New returns a chat rate limiter.
func New(rdb redis.Cmdable) *Chat {
	return &Chat{rl: redisx.NewRateLimiter(rdb)}
}

// AllowChat implements service.RateLimiter.
func (c *Chat) AllowChat(ctx context.Context, userID uuid.UUID) (bool, error) {
	key := fmt.Sprintf("rl:chat:%s", userID)
	ok, err := c.rl.Allow(ctx, key, chatLimit, chatWindow)
	if err != nil {
		return false, err
	}
	return ok, nil
}
