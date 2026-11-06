package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/service"
)

type fakeMsg struct {
	data      []byte
	hdr       nats.Header
	delivered uint64
	acked     bool
	naked     bool
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.delivered}, nil
}
func (m *fakeMsg) Data() []byte                     { return m.data }
func (m *fakeMsg) Headers() nats.Header             { return m.hdr }
func (m *fakeMsg) Subject() string                  { return "wb.chat.message.v1" }
func (m *fakeMsg) Reply() string                    { return "" }
func (m *fakeMsg) Ack() error                       { m.acked = true; return nil }
func (m *fakeMsg) DoubleAck(context.Context) error  { return nil }
func (m *fakeMsg) Nak() error                       { m.naked = true; return nil }
func (m *fakeMsg) NakWithDelay(time.Duration) error { return m.Nak() }
func (m *fakeMsg) InProgress() error                { return nil }
func (m *fakeMsg) Term() error                      { return nil }
func (m *fakeMsg) TermWithReason(string) error      { return nil }

type memStore struct {
	msgs []repo.Message
}

func (m *memStore) InsertMessage(_ context.Context, id string, roomID, userID uuid.UUID, body string) (bool, error) {
	m.msgs = append(m.msgs, repo.Message{ID: id, RoomID: roomID, UserID: userID, Body: body, CreatedAt: time.Unix(1, 0).UTC()})
	return true, nil
}
func (m *memStore) GetMessageByID(_ context.Context, id string) (repo.Message, error) {
	for _, row := range m.msgs {
		if row.ID == id {
			return row, nil
		}
	}
	return repo.Message{}, repo.ErrNotFound
}
func (m *memStore) ListHistory(context.Context, uuid.UUID, *repo.Message, int32) ([]repo.Message, error) {
	return nil, nil
}
func (m *memStore) IsRoomMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type tape struct{ n int }

func (t *tape) MessageSent(context.Context, *chatv1.Message) error {
	t.n++
	return nil
}
func (t *tape) Rejected(context.Context, *chatv1.Message, string) error { return nil }

type allow struct{}

func (allow) AllowChat(context.Context, uuid.UUID) (bool, error) { return true, nil }

func TestOnMsg(t *testing.T) {
	c, err := New(service.New(&memStore{}, &tape{}, allow{}))
	if err != nil {
		t.Fatal(err)
	}
	bad := &fakeMsg{data: []byte{0xff}}
	c.onMsg(context.Background(), bad)
	if !bad.acked {
		t.Fatal("bad payload should ack")
	}
	empty, err := proto.Marshal(&realtimev1.ChatSend{})
	if err != nil {
		t.Fatal(err)
	}
	invalid := &fakeMsg{data: empty}
	c.onMsg(context.Background(), invalid)
	if !invalid.acked {
		t.Fatal("invalid chat send should ack")
	}
	id, err := ulid.New(ulid.Timestamp(time.Now()), ulid.DefaultEntropy())
	if err != nil {
		t.Fatal(err)
	}
	body, err := proto.Marshal(&realtimev1.ChatSend{
		MessageId: id.String(), RoomId: uuid.NewString(), Body: "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	noUser := &fakeMsg{data: body}
	c.onMsg(context.Background(), noUser)
	if !noUser.acked {
		t.Fatal("missing principal should ack")
	}
	user := uuid.New()
	ok := &fakeMsg{data: body, hdr: nats.Header{
		"Wb-User-Id":   []string{user.String()},
		"Wb-Username":  []string{"ada"},
	}}
	c.onMsg(context.Background(), ok)
	if !ok.acked || ok.naked {
		t.Fatalf("success ack=%v nak=%v", ok.acked, ok.naked)
	}

	down := &failStore{}
	c.svc = service.New(down, &tape{}, allow{})
	retry := &fakeMsg{data: body, delivered: 1, hdr: nats.Header{"Wb-User-Id": []string{user.String()}}}
	c.onMsg(context.Background(), retry)
	if !retry.naked || retry.acked {
		t.Fatalf("retry nak=%v ack=%v", retry.naked, retry.acked)
	}
	poison := &fakeMsg{data: body, delivered: maxDeliver, hdr: nats.Header{"Wb-User-Id": []string{user.String()}}}
	c.onMsg(context.Background(), poison)
	if !poison.acked {
		t.Fatal("poison should ack")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped := &fakeMsg{data: body}
	c.onMsg(ctx, stopped)
	if stopped.acked || stopped.naked {
		t.Fatal("canceled context should ignore the delivery")
	}
}

type failStore struct{}

func (failStore) InsertMessage(context.Context, string, uuid.UUID, uuid.UUID, string) (bool, error) {
	return false, repo.ErrNotFound
}
func (failStore) GetMessageByID(context.Context, string) (repo.Message, error) {
	return repo.Message{}, repo.ErrNotFound
}
func (failStore) ListHistory(context.Context, uuid.UUID, *repo.Message, int32) ([]repo.Message, error) {
	return nil, nil
}
func (failStore) IsRoomMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}
