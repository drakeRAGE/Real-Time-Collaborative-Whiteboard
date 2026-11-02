package ingest

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/service"
)

type fakeApply struct {
	order *[]string
	err   error
}

func (f *fakeApply) ApplyStroke(context.Context, uuid.UUID, *realtimev1.StrokeInput) error {
	*f.order = append(*f.order, "commit")
	return f.err
}
func (f *fakeApply) ApplyShape(context.Context, uuid.UUID, *realtimev1.ShapeInput) error {
	return f.err
}
func (f *fakeApply) ApplyClear(context.Context, uuid.UUID, uuid.UUID, string) error { return f.err }
func (f *fakeApply) ApplyUndo(context.Context, uuid.UUID, uuid.UUID, string) error  { return f.err }
func (f *fakeApply) ApplyRedo(context.Context, uuid.UUID, uuid.UUID, string) error  { return f.err }

type fakeMsg struct {
	subject string
	data    []byte
	hdr     nats.Header
	order   *[]string
	acked   bool
	naked   bool
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: 1}, nil
}
func (m *fakeMsg) Data() []byte                     { return m.data }
func (m *fakeMsg) Headers() nats.Header             { return m.hdr }
func (m *fakeMsg) Subject() string                  { return m.subject }
func (m *fakeMsg) Reply() string                    { return "" }
func (m *fakeMsg) Ack() error                       { m.acked = true; *m.order = append(*m.order, "ack"); return nil }
func (m *fakeMsg) DoubleAck(context.Context) error  { return nil }
func (m *fakeMsg) Nak() error                       { m.naked = true; return nil }
func (m *fakeMsg) NakWithDelay(time.Duration) error { m.naked = true; return nil }
func (m *fakeMsg) InProgress() error                { return nil }
func (m *fakeMsg) Term() error                      { return nil }
func (m *fakeMsg) TermWithReason(string) error      { return nil }

func TestAckAfterHandler(t *testing.T) {
	var order []string
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	c := &Consumer{apply: &fakeApply{order: &order}, log: zerolog.Nop(), v: v}
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	room := uuid.New()
	user := uuid.New()
	body, err := proto.Marshal(&realtimev1.StrokeInput{
		StrokeId: id.String(),
		RoomId:   room.String(),
		Points:   []*boardv1.Point{{X: 1, Y: 1}},
		Color:    "#abcdef",
		Width:    3,
		Tool:     boardv1.Tool_TOOL_PEN,
	})
	if err != nil {
		t.Fatal(err)
	}
	hdr := nats.Header{}
	hdr.Set("Nats-Msg-Id", id.String())
	hdr.Set("Wb-User-Id", user.String())
	msg := &fakeMsg{subject: "wb.board.stroke.v1", data: body, hdr: hdr, order: &order}
	c.onMsg(context.Background(), msg)
	if !msg.acked || msg.naked {
		t.Fatalf("acked %v naked %v", msg.acked, msg.naked)
	}
	if len(order) != 2 || order[0] != "commit" || order[1] != "ack" {
		t.Fatalf("order %v", order)
	}
}

func TestPermanentErrorAcksWithoutRetry(t *testing.T) {
	var order []string
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	c := &Consumer{apply: &fakeApply{order: &order, err: service.ErrNotAdmin}, log: zerolog.Nop(), v: v}
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, err := proto.Marshal(&realtimev1.UndoInput{RoomId: uuid.New().String()})
	if err != nil {
		t.Fatal(err)
	}
	hdr := nats.Header{}
	hdr.Set("Nats-Msg-Id", id.String())
	hdr.Set("Wb-User-Id", uuid.New().String())
	msg := &fakeMsg{subject: "wb.board.undo.v1", data: body, hdr: hdr, order: &order}
	c.onMsg(context.Background(), msg)
	if !msg.acked || msg.naked {
		t.Fatalf("acked %v naked %v", msg.acked, msg.naked)
	}
}

func TestAppliedSubjectIsNotIngested(t *testing.T) {
	var order []string
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	c := &Consumer{apply: &fakeApply{order: &order}, log: zerolog.Nop(), v: v}
	msg := &fakeMsg{subject: "wb.board.stroke_applied.v1", data: []byte{1}, hdr: nats.Header{}, order: &order}
	c.onMsg(context.Background(), msg)
	if !msg.acked || len(order) != 1 || order[0] != "ack" {
		t.Fatalf("order %v acked %v", order, msg.acked)
	}
}
