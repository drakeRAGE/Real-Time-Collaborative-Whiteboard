package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/repo"
)

// Publisher emits chat fanout events.
type Publisher interface {
	MessageSent(ctx context.Context, msg *chatv1.Message) error
	Rejected(ctx context.Context, msg *chatv1.Message, reason string) error
}

// Service implements chat business logic.
type Service struct {
	store repo.Store
	pub   Publisher
	rl    RateLimiter
}

// RateLimiter gates chat sends per user.
type RateLimiter interface {
	AllowChat(ctx context.Context, userID uuid.UUID) (bool, error)
}

// New returns a chat service.
func New(store repo.Store, pub Publisher, rl RateLimiter) *Service {
	return &Service{store: store, pub: pub, rl: rl}
}

// GetHistory returns one page of messages for a room member.
func (s *Service) GetHistory(ctx context.Context, roomID uuid.UUID, cursor string, limit int32) ([]*chatv1.Message, string, error) {
	caller, err := principalFromGRPC(ctx)
	if err != nil {
		return nil, "", err
	}
	member, err := s.store.IsRoomMember(ctx, roomID, caller)
	if err != nil {
		return nil, "", err
	}
	if !member {
		return nil, "", ErrNotMember
	}
	var before *repo.Message
	if cursor != "" {
		decoded, derr := decodeCursor(cursor)
		if derr != nil {
			return nil, "", derr
		}
		before = &decoded
	}
	rows, err := s.store.ListHistory(ctx, roomID, before, limit)
	if err != nil {
		return nil, "", err
	}
	var next string
	if len(rows) > int(limit) {
		rows = rows[:limit]
		oldest := rows[len(rows)-1]
		next = encodeCursor(oldest)
	}
	// Rows are newest-first from SQL; return ascending created_at within the page.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	out := make([]*chatv1.Message, len(rows))
	for i, row := range rows {
		out[i] = toProto(row)
	}
	return out, next, nil
}

// HandleIngest processes one JetStream chat ingest delivery.
func (s *Service) HandleIngest(ctx context.Context, send *realtimev1.ChatSend, userID uuid.UUID, username string) error {
	if send == nil {
		return ErrInvalidArgument
	}
	roomID, err := uuid.Parse(send.GetRoomId())
	if err != nil {
		return ErrInvalidArgument
	}
	allowed, err := s.rl.AllowChat(ctx, userID)
	if err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}
	if !allowed {
		reject := &chatv1.Message{
			Id:     send.GetMessageId(),
			RoomId: send.GetRoomId(),
			UserId: userID.String(),
		}
		if perr := s.pub.Rejected(ctx, reject, "rate_limited"); perr != nil {
			return perr
		}
		return nil
	}
	if _, err := s.store.InsertMessage(ctx, send.GetMessageId(), roomID, userID, send.GetBody()); err != nil {
		return err
	}
	stored, err := s.store.GetMessageByID(ctx, send.GetMessageId())
	if err != nil {
		return err
	}
	if stored.Username == "" {
		stored.Username = username
	}
	return s.pub.MessageSent(ctx, toProto(stored))
}

func toProto(row repo.Message) *chatv1.Message {
	out := &chatv1.Message{
		Id:       row.ID,
		RoomId:   row.RoomID.String(),
		UserId:   row.UserID.String(),
		Username: row.Username,
		Body:     row.Body,
	}
	if !row.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(row.CreatedAt)
	}
	return out
}
