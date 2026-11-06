package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc/metadata"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/repo"
)

type memStore struct {
	member    bool
	memberErr error
	insertErr error
	getErr    error
	msgs      []repo.Message
}

func (m *memStore) InsertMessage(_ context.Context, id string, roomID, userID uuid.UUID, body string) (bool, error) {
	if m.insertErr != nil {
		return false, m.insertErr
	}
	for _, row := range m.msgs {
		if row.ID == id {
			return false, nil
		}
	}
	m.msgs = append(m.msgs, repo.Message{
		ID: id, RoomID: roomID, UserID: userID, Body: body, CreatedAt: time.Unix(int64(len(m.msgs)+1), 0).UTC(),
	})
	return true, nil
}

func (m *memStore) GetMessageByID(_ context.Context, id string) (repo.Message, error) {
	if m.getErr != nil {
		return repo.Message{}, m.getErr
	}
	for _, row := range m.msgs {
		if row.ID == id {
			return row, nil
		}
	}
	return repo.Message{}, repo.ErrNotFound
}

func (m *memStore) ListHistory(_ context.Context, roomID uuid.UUID, before *repo.Message, _ int32) ([]repo.Message, error) {
	var out []repo.Message
	for i := len(m.msgs) - 1; i >= 0; i-- {
		row := m.msgs[i]
		if row.RoomID != roomID {
			continue
		}
		if before != nil && !row.CreatedAt.Before(before.CreatedAt) {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (m *memStore) IsRoomMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	if m.memberErr != nil {
		return false, m.memberErr
	}
	return m.member, nil
}

type tape struct {
	sent     []*chatv1.Message
	rejected []string
	err      error
}

func (t *tape) MessageSent(_ context.Context, msg *chatv1.Message) error {
	t.sent = append(t.sent, msg)
	return t.err
}

func (t *tape) Rejected(_ context.Context, _ *chatv1.Message, reason string) error {
	t.rejected = append(t.rejected, reason)
	return t.err
}

type allow struct {
	ok  bool
	err error
}

func (a allow) AllowChat(context.Context, uuid.UUID) (bool, error) { return a.ok, a.err }

func asUser(id uuid.UUID) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("wb-user-id", id.String()))
}

func TestGetHistoryPagesAndRejects(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &memStore{member: true}
	for _, body := range []string{"a", "b", "c"} {
		if _, err := store.InsertMessage(context.Background(), body, room, user, body); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(store, &tape{}, allow{ok: true})

	if _, _, err := svc.GetHistory(context.Background(), room, "", 2); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("anon %v", err)
	}
	bad := metadata.NewIncomingContext(context.Background(), metadata.Pairs("wb-user-id", "nope"))
	if _, _, err := svc.GetHistory(bad, room, "", 2); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bad sub %v", err)
	}
	store.member = false
	if _, _, err := svc.GetHistory(asUser(user), room, "", 2); !errors.Is(err, ErrNotMember) {
		t.Fatalf("stranger %v", err)
	}
	store.member = true
	store.memberErr = errors.New("db")
	if _, _, err := svc.GetHistory(asUser(user), room, "", 2); err == nil || errors.Is(err, ErrNotMember) {
		t.Fatalf("member err %v", err)
	}
	store.memberErr = nil

	page, next, err := svc.GetHistory(asUser(user), room, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Body != "b" || page[1].Body != "c" || next == "" {
		t.Fatalf("page %v next %q", page, next)
	}
	rest, next2, err := svc.GetHistory(asUser(user), room, next, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 1 || rest[0].Body != "a" || next2 != "" {
		t.Fatalf("rest %v next %q", rest, next2)
	}
	if _, _, err := svc.GetHistory(asUser(user), room, "%%%", 2); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("cursor %v", err)
	}
	zero := store.msgs[0]
	zero.CreatedAt = time.Time{}
	store.msgs = []repo.Message{zero}
	got, _, err := svc.GetHistory(asUser(user), room, "", 5)
	if err != nil || len(got) != 1 || got[0].CreatedAt != nil {
		t.Fatalf("zero time %+v err %v", got, err)
	}
}

func TestHandleIngest(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &memStore{}
	pub := &tape{}
	svc := New(store, pub, allow{ok: true})
	send := &realtimev1.ChatSend{MessageId: "m1", RoomId: room.String(), Body: "hi"}

	if err := svc.HandleIngest(context.Background(), nil, user, "ada"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil %v", err)
	}
	if err := svc.HandleIngest(context.Background(), &realtimev1.ChatSend{RoomId: "nope"}, user, "ada"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("room %v", err)
	}

	limited := New(store, pub, allow{})
	if err := limited.HandleIngest(context.Background(), send, user, "ada"); err != nil {
		t.Fatal(err)
	}
	if len(pub.rejected) != 1 || pub.rejected[0] != "rate_limited" || len(pub.sent) != 0 {
		t.Fatalf("rejected %+v sent %d", pub.rejected, len(pub.sent))
	}
	pub.err = errors.New("nats")
	if err := limited.HandleIngest(context.Background(), send, user, "ada"); err == nil {
		t.Fatal("expected reject publish error")
	}
	pub.err = nil

	down := New(store, pub, allow{err: errors.New("redis")})
	if err := down.HandleIngest(context.Background(), send, user, "ada"); err == nil {
		t.Fatal("expected rate limit error")
	}

	if err := svc.HandleIngest(context.Background(), send, user, "ada"); err != nil {
		t.Fatal(err)
	}
	if len(pub.sent) != 1 || pub.sent[0].GetUsername() != "ada" || pub.sent[0].GetBody() != "hi" {
		t.Fatalf("sent %+v", pub.sent)
	}
	if err := svc.HandleIngest(context.Background(), send, user, "ada"); err != nil {
		t.Fatal(err)
	}
	if len(pub.sent) != 2 {
		t.Fatalf("duplicate should still publish, sent %d", len(pub.sent))
	}

	store.insertErr = errors.New("insert")
	if err := svc.HandleIngest(context.Background(), &realtimev1.ChatSend{MessageId: "m2", RoomId: room.String(), Body: "x"}, user, "ada"); err == nil {
		t.Fatal("expected insert error")
	}
	store.insertErr = nil
	store.getErr = errors.New("read")
	if err := svc.HandleIngest(context.Background(), &realtimev1.ChatSend{MessageId: "m3", RoomId: room.String(), Body: "x"}, user, "ada"); err == nil {
		t.Fatal("expected get error")
	}
}

func TestCursorAndPrincipal(t *testing.T) {
	msg := repo.Message{ID: "m", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)}
	back, err := decodeCursor(encodeCursor(msg))
	if err != nil || back.ID != msg.ID || !back.CreatedAt.Equal(msg.CreatedAt) {
		t.Fatalf("cursor %+v err %v", back, err)
	}
	for _, bad := range []string{"", "%%%", encodeCursor(repo.Message{CreatedAt: msg.CreatedAt})} {
		if _, err := decodeCursor(bad); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("%q %v", bad, err)
		}
	}
	raw := encodeCursor(msg)
	broken := raw[:len(raw)-4] + "@@@@"
	if _, err := decodeCursor(broken); err == nil {
		t.Fatal("expected bad encoding")
	}

	id := uuid.New()
	got, name, err := PrincipalFromNATS(nats.Header{"Wb-User-Id": []string{id.String()}, "Wb-Username": []string{"ada"}})
	if err != nil || got != id || name != "ada" {
		t.Fatalf("%s %s %v", got, name, err)
	}
	got, name, err = PrincipalFromNATS(nats.Header{"Wb-User-Id": []string{id.String()}})
	if err != nil || got != id || name != "user" {
		t.Fatalf("default name got=%s name=%s err=%v", got, name, err)
	}
	if _, _, err := PrincipalFromNATS(nil); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("missing %v", err)
	}
	if _, _, err := PrincipalFromNATS(nats.Header{"Wb-User-Id": []string{"nope"}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bad id %v", err)
	}
}
