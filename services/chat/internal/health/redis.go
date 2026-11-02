package health

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type redisPinger struct {
	rdb *redis.Client
}

func (r redisPinger) Ping(ctx context.Context) error {
	return r.rdb.Ping(ctx).Err()
}

// RedisPinger adapts a go-redis client to Pinger.
func RedisPinger(rdb *redis.Client) Pinger {
	return redisPinger{rdb: rdb}
}
