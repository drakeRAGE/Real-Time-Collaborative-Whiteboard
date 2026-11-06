package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/repo"
)

type memStore struct {
	member bool
	msgs   []repo.Message
}

func (m *memStore) InsertMessage(_ context.Context, id string, roomID, userID uuid.UUID, body string) (bool, error) {
	m.msgs = append(m.msgs, repo.Message{
		ID: id, RoomID: roomID, UserID: userID, Body: body, CreatedAt: time.Unix(1, 0).UTC(),
	})
	return true, nil
}

func (m *memStore) GetMessageByID(context.Context, string) (repo.Message, error) {
	return repo.Message{}, repo.ErrNotFound
}

func (m *memStore) ListHistory(context.Context, uuid.UUID, *repo.Message, int32) ([]repo.Message, error) {
	out := make([]repo.Message, len(m.msgs))
	for i := range m.msgs {
		out[len(m.msgs)-1-i] = m.msgs[i]
	}
	return out, nil
}

func (m *memStore) IsRoomMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return m.member, nil
}
