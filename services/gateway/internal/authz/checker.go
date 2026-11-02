package authz

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

const membershipTTL = 30 * time.Second

// MemberAPI is the room-service lookup used on a cache miss.
type MemberAPI interface {
	CheckMembership(ctx context.Context, in *roomv1.CheckMembershipRequest, opts ...grpc.CallOption) (*roomv1.CheckMembershipResponse, error)
}

// Checker caches CheckMembership at authz:{userID}:{roomID} for 30s.
type Checker struct {
	rdb   redis.Cmdable
	rooms MemberAPI
	ttl   time.Duration
}

// NewChecker returns a cache. ttl is 30s when zero.
func NewChecker(rdb redis.Cmdable, rooms MemberAPI, ttl time.Duration) *Checker {
	if ttl <= 0 {
		ttl = membershipTTL
	}
	return &Checker{rdb: rdb, rooms: rooms, ttl: ttl}
}

func cacheKey(userID, roomID string) string {
	return "authz:" + userID + ":" + roomID
}

// Role returns the cached role, loading it from room-service on a miss.
// A Redis error falls through to room-service. An RPC error is returned.
func (c *Checker) Role(ctx context.Context, user principal.User, roomID string) (roomv1.Role, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	key := cacheKey(user.ID, roomID)
	raw, err := c.rdb.Get(ctx, key).Result()
	if err == nil {
		if n, conv := strconv.Atoi(raw); conv == nil && n >= 0 && n <= int(roomv1.Role_ROLE_ADMIN) {
			return roomv1.Role(n), nil
		}
	}
	resp, err := c.rooms.CheckMembership(user.GRPC(ctx), &roomv1.CheckMembershipRequest{
		RoomId: roomID,
		UserId: user.ID,
	})
	if err != nil {
		return 0, fmt.Errorf("check membership: %w", err)
	}
	role := resp.GetRole()
	if role == roomv1.Role_ROLE_UNSPECIFIED {
		role = roomv1.Role_ROLE_NONE
	}
	if setErr := c.Remember(ctx, user.ID, roomID, role); setErr != nil {
		return role, nil
	}
	return role, nil
}

// Remember stores role until the TTL elapses.
func (c *Checker) Remember(ctx context.Context, userID, roomID string, role roomv1.Role) error {
	if err := c.rdb.Set(ctx, cacheKey(userID, roomID), strconv.Itoa(int(role)), c.ttl).Err(); err != nil {
		return fmt.Errorf("authz cache: %w", err)
	}
	return nil
}

// Forget drops one user's cached role. member_left and member_joined call this.
func (c *Checker) Forget(ctx context.Context, userID, roomID string) error {
	if err := c.rdb.Del(ctx, cacheKey(userID, roomID)).Err(); err != nil {
		return fmt.Errorf("authz forget: %w", err)
	}
	return nil
}

// ForgetRoom drops every cached role for roomID. Room deletion calls this.
func (c *Checker) ForgetRoom(ctx context.Context, roomID string) error {
	var cursor uint64
	pattern := "authz:*:" + roomID
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("authz scan: %w", err)
		}
		if len(keys) > 0 {
			if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("authz forget room: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
