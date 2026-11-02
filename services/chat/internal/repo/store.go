package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/db"
)

// ErrNotFound is returned when a row is absent.
var ErrNotFound = errors.New("not found")

// Message is one chat row with username joined from profiles.
type Message struct {
	ID        string
	RoomID    uuid.UUID
	UserID    uuid.UUID
	Body      string
	Username  string
	CreatedAt time.Time
}

// Store persists and reads chat messages.
type Store interface {
	InsertMessage(ctx context.Context, id string, roomID, userID uuid.UUID, body string) (inserted bool, err error)
	GetMessageByID(ctx context.Context, id string) (Message, error)
	ListHistory(ctx context.Context, roomID uuid.UUID, before *Message, limit int32) ([]Message, error)
	IsRoomMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error)
}

// PGStore runs sqlc queries on a pgx pool.
type PGStore struct {
	q *db.Queries
}

// New returns a store bound to pool.
func New(pool *pgxpool.Pool) *PGStore {
	return &PGStore{q: db.New(pool)}
}

func (s *PGStore) InsertMessage(ctx context.Context, id string, roomID, userID uuid.UUID, body string) (bool, error) {
	n, err := s.q.InsertChatMessage(ctx, db.InsertChatMessageParams{
		ID:     id,
		RoomID: roomID,
		UserID: userID,
		Body:   body,
	})
	if err != nil {
		return false, fmt.Errorf("insert chat message: %w", err)
	}
	return n > 0, nil
}

func (s *PGStore) GetMessageByID(ctx context.Context, id string) (Message, error) {
	row, err := s.q.GetChatMessageByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}
		return Message{}, fmt.Errorf("get chat message: %w", err)
	}
	return fromGetRow(row), nil
}

func (s *PGStore) ListHistory(ctx context.Context, roomID uuid.UUID, before *Message, limit int32) ([]Message, error) {
	fetch := limit + 1
	if before == nil {
		rows, err := s.q.ListChatHistoryLatest(ctx, db.ListChatHistoryLatestParams{
			RoomID: roomID,
			Limit:  fetch,
		})
		if err != nil {
			return nil, fmt.Errorf("list chat history: %w", err)
		}
		return fromLatestRows(rows), nil
	}
	rows, err := s.q.ListChatHistoryBefore(ctx, db.ListChatHistoryBeforeParams{
		RoomID:    roomID,
		CreatedAt: before.CreatedAt,
		ID:        before.ID,
		Limit:     fetch,
	})
	if err != nil {
		return nil, fmt.Errorf("list chat history: %w", err)
	}
	return fromBeforeRows(rows), nil
}

func (s *PGStore) IsRoomMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error) {
	ok, err := s.q.IsRoomMember(ctx, db.IsRoomMemberParams{RoomID: roomID, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("is room member: %w", err)
	}
	return ok, nil
}

func fromGetRow(row db.GetChatMessageByIDRow) Message {
	return Message{
		ID: row.ID, RoomID: row.RoomID, UserID: row.UserID,
		Body: row.Body, Username: row.Username, CreatedAt: row.CreatedAt,
	}
}

func fromLatestRows(rows []db.ListChatHistoryLatestRow) []Message {
	out := make([]Message, len(rows))
	for i, row := range rows {
		out[i] = Message{
			ID: row.ID, RoomID: row.RoomID, UserID: row.UserID,
			Body: row.Body, Username: row.Username, CreatedAt: row.CreatedAt,
		}
	}
	return out
}

func fromBeforeRows(rows []db.ListChatHistoryBeforeRow) []Message {
	out := make([]Message, len(rows))
	for i, row := range rows {
		out[i] = Message{
			ID: row.ID, RoomID: row.RoomID, UserID: row.UserID,
			Body: row.Body, Username: row.Username, CreatedAt: row.CreatedAt,
		}
	}
	return out
}
