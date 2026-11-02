// Package fanout turns applied NATS events into ServerEnvelopes for room sockets.
// Board and chat use ephemeral JetStream consumers (deliver-new, filtered).
// Presence and room lifecycle are core NATS. natsx.Consume cannot set filters.
package fanout

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/authz"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

var (
	boardFilters = []string{
		"wb.board.stroke_applied.v1",
		"wb.board.shape_applied.v1",
		"wb.board.cleared.v1",
		"wb.board.undo_applied.v1",
		"wb.board.redo_applied.v1",
	}
	chatFilters = []string{
		"wb.chat.message_sent.v1",
		"wb.chat.rejected.v1",
	}
)

// Router delivers envelopes to live sockets.
type Router interface {
	Broadcast(roomID string, env *realtimev1.ServerEnvelope, excludeUserID string)
	SendUser(userID string, env *realtimev1.ServerEnvelope)
	AnyUser(roomID string) (principal.User, bool)
	CloseRoom(roomID string)
}

// Cache invalidates membership entries from room lifecycle events.
type Cache interface {
	Forget(ctx context.Context, userID, roomID string) error
	ForgetRoom(ctx context.Context, roomID string) error
}

// Fanout owns the gateway's ephemeral subscriptions.
type Fanout struct {
	nc    *nats.Conn
	hub   Router
	rooms authz.Rooms
	cache Cache
	log   zerolog.Logger

	mu       sync.Mutex
	subs     []*nats.Subscription
	consumes []jetstream.ConsumeContext
	seen     *dedupe
}

// New returns a fanout. Start subscribes.
func New(nc *nats.Conn, hub Router, rooms authz.Rooms, cache Cache, log zerolog.Logger) *Fanout {
	return &Fanout{nc: nc, hub: hub, rooms: rooms, cache: cache, log: log, seen: newDedupe()}
}

// Start binds deliver-new consumers and core subscriptions.
// JetStream callbacks are owned by the server client and stopped by Stop.
func (f *Fanout) Start(ctx context.Context) error {
	js, err := jetstream.New(f.nc)
	if err != nil {
		return fmt.Errorf("fanout jetstream: %w", err)
	}
	if err := f.consume(ctx, js, "BOARD", boardFilters); err != nil {
		return err
	}
	if err := f.consume(ctx, js, "CHAT", chatFilters); err != nil {
		return err
	}
	for _, subject := range []string{"wb.presence.>", "wb.room.>"} {
		sub, err := f.nc.Subscribe(subject, func(msg *nats.Msg) {
			f.deliver(msg.Subject, msg.Data, msg.Header)
		})
		if err != nil {
			return fmt.Errorf("fanout subscribe %s: %w", subject, err)
		}
		f.mu.Lock()
		f.subs = append(f.subs, sub)
		f.mu.Unlock()
	}
	if err := f.nc.Flush(); err != nil {
		return fmt.Errorf("fanout flush: %w", err)
	}
	return nil
}

// Stop unsubscribes. It does not close the connection.
func (f *Fanout) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.consumes {
		c.Stop()
	}
	f.consumes = nil
	for _, sub := range f.subs {
		_ = sub.Unsubscribe()
	}
	f.subs = nil
}

func (f *Fanout) consume(ctx context.Context, js jetstream.JetStream, stream string, filters []string) error {
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	name := "gw-" + strings.ToLower(stream)
	if err == nil {
		name = "gw-" + strings.ToLower(id.String())
	}
	cons, err := js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Name:              name,
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		FilterSubjects:    filters,
		InactiveThreshold: 5 * time.Minute,
		MemoryStorage:     true,
	})
	if err != nil {
		return fmt.Errorf("fanout consumer %s: %w", stream, err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		f.deliver(msg.Subject(), msg.Data(), msg.Headers())
		if ackErr := msg.Ack(); ackErr != nil {
			f.log.Warn().Err(ackErr).Str("subject", msg.Subject()).Msg("fanout ack")
		}
	})
	if err != nil {
		return fmt.Errorf("fanout consume %s: %w", stream, err)
	}
	f.mu.Lock()
	f.consumes = append(f.consumes, cc)
	f.mu.Unlock()
	return nil
}

func (f *Fanout) deliver(subject string, data []byte, hdr nats.Header) {
	if id := hdr.Get("Nats-Msg-Id"); id != "" && !f.seen.once(id) {
		return
	}
	eff, err := translate(subject, data, hdr)
	if err != nil {
		f.log.Warn().Err(err).Str("subject", subject).Msg("fanout decode")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	f.apply(ctx, eff)
}

func (f *Fanout) apply(ctx context.Context, eff effect) {
	if eff.RoomID == "" && eff.DirectUser == "" {
		return
	}
	if f.cache != nil && eff.ForgetUser != "" && eff.RoomID != "" {
		if err := f.cache.Forget(ctx, eff.ForgetUser, eff.RoomID); err != nil {
			f.log.Warn().Err(err).Msg("authz forget")
		}
	}
	if f.cache != nil && eff.ForgetRoom && eff.RoomID != "" {
		if err := f.cache.ForgetRoom(ctx, eff.RoomID); err != nil {
			f.log.Warn().Err(err).Msg("authz forget room")
		}
	}
	if eff.RoomState && eff.RoomID != "" {
		f.pushState(ctx, eff)
	}
	if eff.DirectUser != "" && eff.Env != nil {
		f.hub.SendUser(eff.DirectUser, eff.Env)
	} else if eff.Env != nil && eff.RoomID != "" && !eff.RoomState {
		f.hub.Broadcast(eff.RoomID, eff.Env, eff.ExcludeUser)
	}
	if eff.CloseRoom && eff.RoomID != "" {
		f.hub.CloseRoom(eff.RoomID)
	}
}

func (f *Fanout) pushState(ctx context.Context, eff effect) {
	user := eff.StateUser
	if user.ID == "" {
		found, ok := f.hub.AnyUser(eff.RoomID)
		if !ok {
			return
		}
		user = found
	}
	if user.Name == "" {
		user.Name = "user"
	}
	state, err := authz.LoadRoomState(ctx, f.rooms, user, eff.RoomID)
	if err != nil {
		f.log.Debug().Err(err).Str("room_id", eff.RoomID).Msg("room state")
		return
	}
	f.hub.Broadcast(eff.RoomID, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_RoomState{RoomState: state}}, "")
}

type effect struct {
	RoomID      string
	ExcludeUser string
	DirectUser  string
	ForgetUser  string
	ForgetRoom  bool
	RoomState   bool
	CloseRoom   bool
	StateUser   principal.User
	Env         *realtimev1.ServerEnvelope
}

func translate(subject string, data []byte, hdr nats.Header) (effect, error) {
	switch subject {
	case "wb.board.stroke_applied.v1":
		ev := &boardv1.StrokeAdded{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetStroke().GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_StrokeApplied{StrokeApplied: ev}}}, nil
	case "wb.board.shape_applied.v1":
		ev := &boardv1.ShapeAdded{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetShape().GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_ShapeApplied{ShapeApplied: ev}}}, nil
	case "wb.board.cleared.v1":
		ev := &boardv1.BoardCleared{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_BoardCleared{BoardCleared: ev}}}, nil
	case "wb.board.undo_applied.v1":
		ev := &boardv1.UndoApplied{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_UndoApplied{UndoApplied: ev}}}, nil
	case "wb.board.redo_applied.v1":
		ev := &boardv1.RedoApplied{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_RedoApplied{RedoApplied: ev}}}, nil
	case "wb.chat.message_sent.v1":
		ev := &chatv1.Message{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_ChatMessage{ChatMessage: ev}}}, nil
	case "wb.chat.rejected.v1":
		ev := &chatv1.Message{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		code := realtimev1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION
		reason := hdr.Get("Wb-Reason")
		if reason == "" {
			reason = "rejected"
		}
		if reason == "rate_limited" {
			code = realtimev1.ErrorCode_ERROR_CODE_RATE_LIMITED
		}
		return effect{DirectUser: ev.GetUserId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
			Code: code, Message: reason,
		}}}}, nil
	case "wb.presence.cursor.v1":
		ev := &presencev1.CursorMoved{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetRoomId(), ExcludeUser: ev.GetUserId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_CursorMoved{CursorMoved: ev}}}, nil
	case "wb.presence.user_online.v1":
		ev := &presencev1.UserOnline{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_UserOnline{UserOnline: ev}}}, nil
	case "wb.presence.user_offline.v1":
		ev := &presencev1.UserOffline{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: ev.GetRoomId(), Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_UserOffline{UserOffline: ev}}}, nil
	case "wb.room.member_joined.v1":
		ev := &roomv1.Member{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		name := ev.GetUsername()
		if name == "" {
			name = "user"
		}
		return effect{
			RoomID: hdr.Get("Wb-Room-Id"), ForgetUser: ev.GetUserId(), RoomState: true,
			StateUser: principal.User{ID: ev.GetUserId(), Name: name},
		}, nil
	case "wb.room.member_left.v1":
		ev := &roomv1.Member{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return effect{}, err
		}
		return effect{RoomID: hdr.Get("Wb-Room-Id"), ForgetUser: ev.GetUserId(), RoomState: true}, nil
	case "wb.room.deleted.v1":
		roomID := hdr.Get("Wb-Room-Id")
		if roomID == "" {
			roomID = strings.TrimSpace(string(data))
		}
		return effect{
			RoomID: roomID, ForgetRoom: true, CloseRoom: true,
			Env: &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
				Code: realtimev1.ErrorCode_ERROR_CODE_NOT_FOUND, Message: "room deleted",
			}}},
		}, nil
	default:
		return effect{}, nil
	}
}

type dedupe struct {
	mu    sync.Mutex
	ids   map[string]struct{}
	order []string
}

func newDedupe() *dedupe {
	return &dedupe{ids: map[string]struct{}{}}
}

func (d *dedupe) once(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.ids[id]; ok {
		return false
	}
	d.ids[id] = struct{}{}
	d.order = append(d.order, id)
	if len(d.order) > 4096 {
		for _, old := range d.order[:2048] {
			delete(d.ids, old)
		}
		d.order = d.order[2048:]
	}
	return true
}
