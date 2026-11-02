package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/store"
)

const (
	natsUserIDHeader   = "Wb-User-Id"
	natsUsernameHeader = "Wb-Username"
	subjectOffline     = "wb.presence.user_offline.v1"
)

// Service tracks cursor presence in Redis and emits offline events.
type Service struct {
	store *store.Redis
	nc    *nats.Conn
}

// New returns a presence service.
func New(st *store.Redis, nc *nats.Conn) *Service {
	return &Service{store: st, nc: nc}
}

// HandleCursor stores an enriched cursor update from core NATS.
func (s *Service) HandleCursor(ctx context.Context, data []byte, hdr nats.Header) error {
	var move presencev1.CursorMoved
	if err := proto.Unmarshal(data, &move); err != nil {
		return fmt.Errorf("unmarshal cursor: %w", err)
	}
	userID, username, err := principalFromNATS(hdr)
	if err != nil {
		return err
	}
	move.UserId = userID.String()
	move.Username = username
	move.Color = PickColorForUser(move.GetUserId())
	return s.store.SaveCursor(ctx, &move, time.Now())
}

func principalFromNATS(hdr nats.Header) (uuid.UUID, string, error) {
	raw := strings.TrimSpace(hdr.Get(natsUserIDHeader))
	if raw == "" {
		return uuid.Nil, "", fmt.Errorf("missing user id")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, "", err
	}
	name := strings.TrimSpace(hdr.Get(natsUsernameHeader))
	if name == "" {
		name = "user"
	}
	return id, name, nil
}

// SweepOffline publishes offline events for heartbeats older than maxAge.
func (s *Service) SweepOffline(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	stale, err := s.store.ExpireStale(ctx, cutoff)
	if err != nil {
		return err
	}
	for roomID, users := range stale {
		for _, userID := range users {
			ev := &presencev1.UserOffline{RoomId: roomID, UserId: userID}
			b, err := proto.Marshal(ev)
			if err != nil {
				return err
			}
			if err := s.nc.Publish(subjectOffline, b); err != nil {
				return err
			}
		}
	}
	return nil
}
