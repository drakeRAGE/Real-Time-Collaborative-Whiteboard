package service

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/repo"
)

func encodeCursor(msg repo.Message) string {
	raw := fmt.Sprintf("%s|%s", msg.CreatedAt.UTC().Format(time.RFC3339Nano), msg.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (repo.Message, error) {
	if strings.TrimSpace(cursor) == "" {
		return repo.Message{}, ErrInvalidArgument
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return repo.Message{}, ErrInvalidArgument
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return repo.Message{}, ErrInvalidArgument
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return repo.Message{}, ErrInvalidArgument
	}
	return repo.Message{ID: parts[1], CreatedAt: ts.UTC()}, nil
}
