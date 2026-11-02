// Package call keeps the room call roster in Redis.
package call

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/redis/go-redis/v9"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
)

// ErrNotInCall is returned when a toggle arrives for a user who did not join.
var ErrNotInCall = errors.New("call: not in call")

// Participant is one user entering a call.
type Participant struct {
	UserID string
	Name   string
}

// JoinResult is the state sent to the joiner and, when they were new, the broadcast.
type JoinResult struct {
	State  *realtimev1.CallState
	Joined *realtimev1.CallUserJoined
}

// LeaveResult is the broadcast after a leave. Ended is set when the roster is empty.
type LeaveResult struct {
	Left  *realtimev1.CallUserLeft
	Ended *realtimev1.CallEnded
}

type media struct {
	Name string `json:"name"`
	Mic  bool   `json:"mic"`
	Cam  bool   `json:"cam"`
}

// Roster stores call:{roomID} as a set of user ids and callmedia:{roomID} for mic/camera.
type Roster struct {
	rdb redis.Cmdable
}

// New returns a roster backed by rdb.
func New(rdb redis.Cmdable) *Roster {
	return &Roster{rdb: rdb}
}

func setKey(roomID string) string   { return "call:" + roomID }
func mediaKey(roomID string) string { return "callmedia:" + roomID }

// Join adds userID to the roster. Joined is nil when the user was already in the call.
func (r *Roster) Join(ctx context.Context, roomID string, p Participant) (JoinResult, error) {
	if err := ctx.Err(); err != nil {
		return JoinResult{}, err
	}
	n, err := r.rdb.SAdd(ctx, setKey(roomID), p.UserID).Result()
	if err != nil {
		return JoinResult{}, fmt.Errorf("call join: %w", err)
	}
	if n == 1 {
		raw, err := json.Marshal(media{Name: p.Name})
		if err != nil {
			return JoinResult{}, err
		}
		if err := r.rdb.HSet(ctx, mediaKey(roomID), p.UserID, raw).Err(); err != nil {
			return JoinResult{}, fmt.Errorf("call join media: %w", err)
		}
	}
	parts, err := r.participants(ctx, roomID)
	if err != nil {
		return JoinResult{}, err
	}
	res := JoinResult{State: &realtimev1.CallState{RoomId: roomID, Participants: parts}}
	if n == 1 {
		res.Joined = &realtimev1.CallUserJoined{
			RoomId:       roomID,
			UserId:       p.UserID,
			Username:     p.Name,
			Participants: parts,
		}
	}
	return res, nil
}

// Leave removes userID. A user who was not in the call yields an empty result.
func (r *Roster) Leave(ctx context.Context, roomID, userID string) (LeaveResult, error) {
	if err := ctx.Err(); err != nil {
		return LeaveResult{}, err
	}
	n, err := r.rdb.SRem(ctx, setKey(roomID), userID).Result()
	if err != nil {
		return LeaveResult{}, fmt.Errorf("call leave: %w", err)
	}
	if n == 0 {
		return LeaveResult{}, nil
	}
	if err := r.rdb.HDel(ctx, mediaKey(roomID), userID).Err(); err != nil {
		return LeaveResult{}, fmt.Errorf("call leave media: %w", err)
	}
	card, err := r.rdb.SCard(ctx, setKey(roomID)).Result()
	if err != nil {
		return LeaveResult{}, fmt.Errorf("call card: %w", err)
	}
	if card == 0 {
		if err := r.rdb.Del(ctx, setKey(roomID), mediaKey(roomID)).Err(); err != nil {
			return LeaveResult{}, fmt.Errorf("call drop: %w", err)
		}
		return LeaveResult{Ended: &realtimev1.CallEnded{RoomId: roomID}}, nil
	}
	parts, err := r.participants(ctx, roomID)
	if err != nil {
		return LeaveResult{}, err
	}
	return LeaveResult{Left: &realtimev1.CallUserLeft{RoomId: roomID, UserId: userID, Participants: parts}}, nil
}

// Toggle updates mic or camera for a user already in the call.
func (r *Roster) Toggle(ctx context.Context, roomID, userID string, kind realtimev1.MediaKind, enabled bool) (realtimev1.CallToggled, error) {
	if err := ctx.Err(); err != nil {
		return realtimev1.CallToggled{}, err
	}
	ok, err := r.rdb.SIsMember(ctx, setKey(roomID), userID).Result()
	if err != nil {
		return realtimev1.CallToggled{}, fmt.Errorf("call member: %w", err)
	}
	if !ok {
		return realtimev1.CallToggled{}, ErrNotInCall
	}
	cur, err := r.one(ctx, roomID, userID)
	if err != nil {
		return realtimev1.CallToggled{}, err
	}
	switch kind {
	case realtimev1.MediaKind_MEDIA_KIND_MIC:
		cur.Mic = enabled
	case realtimev1.MediaKind_MEDIA_KIND_CAMERA:
		cur.Cam = enabled
	default:
		return realtimev1.CallToggled{}, fmt.Errorf("call toggle: bad media")
	}
	raw, err := json.Marshal(cur)
	if err != nil {
		return realtimev1.CallToggled{}, err
	}
	if err := r.rdb.HSet(ctx, mediaKey(roomID), userID, raw).Err(); err != nil {
		return realtimev1.CallToggled{}, fmt.Errorf("call toggle: %w", err)
	}
	return realtimev1.CallToggled{RoomId: roomID, UserId: userID, Media: kind, Enabled: enabled}, nil
}

func (r *Roster) participants(ctx context.Context, roomID string) ([]*realtimev1.CallParticipant, error) {
	ids, err := r.rdb.SMembers(ctx, setKey(roomID)).Result()
	if err != nil {
		return nil, fmt.Errorf("call members: %w", err)
	}
	sort.Strings(ids)
	out := make([]*realtimev1.CallParticipant, 0, len(ids))
	for _, id := range ids {
		m, err := r.one(ctx, roomID, id)
		if err != nil {
			return nil, err
		}
		name := m.Name
		if name == "" {
			name = "user"
		}
		out = append(out, &realtimev1.CallParticipant{
			UserId:        id,
			Username:      name,
			MicEnabled:    m.Mic,
			CameraEnabled: m.Cam,
		})
	}
	return out, nil
}

func (r *Roster) one(ctx context.Context, roomID, userID string) (media, error) {
	raw, err := r.rdb.HGet(ctx, mediaKey(roomID), userID).Result()
	if errors.Is(err, redis.Nil) {
		return media{Name: "user"}, nil
	}
	if err != nil {
		return media{}, fmt.Errorf("call media: %w", err)
	}
	var m media
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return media{}, fmt.Errorf("call media json: %w", err)
	}
	return m, nil
}
