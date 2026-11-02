package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
)

const (
	heartbeatPrefix = "presence_hb:"
	cursorPrefix    = "presence:"
)

// Redis keeps ephemeral presence state.
type Redis struct {
	rdb redis.Cmdable
}

// New returns a presence store.
func New(rdb redis.Cmdable) *Redis {
	return &Redis{rdb: rdb}
}

func cursorKey(roomID, userID string) string {
	return cursorPrefix + roomID + ":" + userID
}

func heartbeatKey(roomID string) string {
	return heartbeatPrefix + roomID
}

// SaveCursor records cursor state and refreshes the user's heartbeat score.
func (s *Redis) SaveCursor(ctx context.Context, state *presencev1.CursorMoved, now time.Time) error {
	if state == nil {
		return fmt.Errorf("nil cursor")
	}
	b, err := proto.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal cursor: %w", err)
	}
	score := float64(now.UnixMilli())
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, cursorKey(state.GetRoomId(), state.GetUserId()), b, 0)
	pipe.ZAdd(ctx, heartbeatKey(state.GetRoomId()), redis.Z{Score: score, Member: state.GetUserId()})
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save cursor: %w", err)
	}
	return nil
}

// GetCursor returns stored cursor state for a user in a room.
func (s *Redis) GetCursor(ctx context.Context, roomID, userID string) (*presencev1.CursorMoved, error) {
	b, err := s.rdb.Get(ctx, cursorKey(roomID, userID)).Bytes()
	if err != nil {
		return nil, err
	}
	var out presencev1.CursorMoved
	if err := proto.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExpireStale returns user IDs whose heartbeat is older than cutoff across all rooms.
func (s *Redis) ExpireStale(ctx context.Context, cutoff time.Time) (map[string][]string, error) {
	out := make(map[string][]string)
	var cursor uint64
	max := strconv.FormatInt(cutoff.UnixMilli(), 10)
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, heartbeatPrefix+"*", 50).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			roomID := strings.TrimPrefix(key, heartbeatPrefix)
			members, err := s.rdb.ZRangeByScore(ctx, key, &redis.ZRangeBy{
				Min: "-inf",
				Max: max,
			}).Result()
			if err != nil {
				return nil, err
			}
			if len(members) == 0 {
				continue
			}
			out[roomID] = append(out[roomID], members...)
			pipe := s.rdb.Pipeline()
			for _, userID := range members {
				pipe.Del(ctx, cursorKey(roomID, userID))
				pipe.ZRem(ctx, key, userID)
			}
			if _, err := pipe.Exec(ctx); err != nil {
				return nil, err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return out, nil
}
