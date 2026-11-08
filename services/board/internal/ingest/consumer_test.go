package ingest

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
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

func (f *fakeApply) ApplyStroke(_ context.Context, _ uuid.UUID, _ *realtimev1.StrokeInput) error {
	*f.order = append(*f.order, "stroke")
	return f.err
}
func (f *fakeApply) ApplyShape(_ context.Context, _ uuid.UUID, _ *realtimev1.ShapeInput) error {
	*f.order = append(*f.order, "shape")
	return f.err
}
func (f *fakeApply) ApplyClear(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string) error {
	*f.order = append(*f.order, "clear")
	return f.err
}
func (f *fakeApply) ApplyUndo(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string) error {
	*f.order = append(*f.order, "undo")
	return f.err
}
func (f *fakeApply) ApplyRedo(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string) error {
	*f.order = append(*f.order, "redo")
	return f.err
}

type fakeMsg struct {
	subject   string
	data      []byte
	hdr       nats.Header
	order     *[]string
	delivered uint64
	metaErr   error
	acked     bool
	naked     bool
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	if m.metaErr != nil {
		return nil, m.metaErr
	}
	d := m.delivered
	if d == 0 {
		d = 1
	}
	return &jetstream.MsgMetadata{NumDelivered: d}, nil
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
	if len(order) != 2 || order[0] != "stroke" || order[1] != "ack" {
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

func mustULID(t *testing.T) string {
	t.Helper()
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func testConsumer(t *testing.T) *Consumer {
	t.Helper()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	return &Consumer{apply: &fakeApply{order: new([]string)}, log: zerolog.Nop(), v: v}
}

func headers(user uuid.UUID, msgID string) nats.Header {
	h := nats.Header{}
	h.Set("Nats-Msg-Id", msgID)
	h.Set("Wb-User-Id", user.String())
	return h
}

func TestDispatchTable(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	id := mustULID(t)
	strokeBody, err := proto.Marshal(&realtimev1.StrokeInput{
		StrokeId: id,
		RoomId:   room.String(),
		Points:   []*boardv1.Point{{X: 1, Y: 1}},
		Color:    "#abcdef",
		Width:    3,
		Tool:     boardv1.Tool_TOOL_PEN,
	})
	if err != nil {
		t.Fatal(err)
	}
	shapeBody, err := proto.Marshal(&realtimev1.ShapeInput{
		ShapeId: id,
		RoomId:  room.String(),
		Kind:    boardv1.ShapeKind_SHAPE_KIND_LINE,
		Start:   &boardv1.Point{X: 0, Y: 0},
		End:     &boardv1.Point{X: 5, Y: 5},
		Color:   "#abcdef",
		Width:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomOnly, err := proto.Marshal(&realtimev1.ClearInput{RoomId: room.String()})
	if err != nil {
		t.Fatal(err)
	}
	undoBody, err := proto.Marshal(&realtimev1.UndoInput{RoomId: room.String()})
	if err != nil {
		t.Fatal(err)
	}
	redoBody, err := proto.Marshal(&realtimev1.RedoInput{RoomId: room.String()})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("stroke shape clear undo redo", func(t *testing.T) {
		cases := []struct {
			subject string
			data    []byte
			want    string
		}{
			{"wb.board.stroke.v1", strokeBody, "stroke"},
			{"wb.board.shape.v1", shapeBody, "shape"},
			{"wb.board.clear.v1", roomOnly, "clear"},
			{"wb.board.undo.v1", undoBody, "undo"},
			{"wb.board.redo.v1", redoBody, "redo"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.subject, func(t *testing.T) {
				order := []string{}
				v, err := protovalidate.New()
				if err != nil {
					t.Fatal(err)
				}
				c := &Consumer{apply: &fakeApply{order: &order}, log: zerolog.Nop(), v: v}
				err = c.dispatch(context.Background(), tc.subject, tc.data, headers(user, id))
				if err != nil {
					t.Fatal(err)
				}
				if len(order) != 1 || order[0] != tc.want {
					t.Fatalf("order %v want %s", order, tc.want)
				}
			})
		}
	})

	t.Run("header and payload validation", func(t *testing.T) {
		c := testConsumer(t)
		h := headers(user, id)
		tests := []struct {
			name    string
			subject string
			data    []byte
			hdr     nats.Header
			want    error
		}{
			{"bad user", "wb.board.stroke.v1", strokeBody, func() nats.Header {
				bad := headers(user, id)
				bad.Set("Wb-User-Id", "not-uuid")
				return bad
			}(), service.ErrUnauthenticated},
			{"missing msg id", "wb.board.stroke.v1", strokeBody, func() nats.Header {
				bad := headers(user, id)
				bad.Del("Nats-Msg-Id")
				return bad
			}(), service.ErrInvalidArgument},
			{"stroke id mismatch", "wb.board.stroke.v1", strokeBody, headers(user, mustULID(t)), service.ErrInvalidArgument},
			{"garbage payload", "wb.board.stroke.v1", []byte{0xff}, h, service.ErrInvalidArgument},
			{"invalid room", "wb.board.undo.v1", mustMarshal(t, &realtimev1.UndoInput{RoomId: "nope"}), h, service.ErrInvalidArgument},
			{"empty undo fails validate", "wb.board.undo.v1", mustMarshal(t, &realtimev1.UndoInput{}), h, service.ErrInvalidArgument},
		}
		for _, tc := range tests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				err := c.dispatch(context.Background(), tc.subject, tc.data, tc.hdr)
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v want %v", err, tc.want)
				}
			})
		}
	})
}

func mustMarshal(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOnMsgBranches(t *testing.T) {
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	room := uuid.New()
	user := uuid.New()
	id := mustULID(t)
	body := mustMarshal(t, &realtimev1.StrokeInput{
		StrokeId: id,
		RoomId:   room.String(),
		Points:   []*boardv1.Point{{X: 1, Y: 1}},
		Color:    "#abcdef",
		Width:    3,
		Tool:     boardv1.Tool_TOOL_PEN,
	})
	hdr := headers(user, id)

	t.Run("transient error nak", func(t *testing.T) {
		var order []string
		c := &Consumer{apply: &fakeApply{order: &order, err: errors.New("db down")}, log: zerolog.Nop(), v: v}
		msg := &fakeMsg{subject: "wb.board.stroke.v1", data: body, hdr: hdr, order: &order, delivered: 2}
		c.onMsg(context.Background(), msg)
		if msg.acked || !msg.naked {
			t.Fatalf("ack=%v nak=%v", msg.acked, msg.naked)
		}
	})

	t.Run("metadata error nak", func(t *testing.T) {
		var order []string
		c := &Consumer{apply: &fakeApply{order: &order, err: errors.New("db down")}, log: zerolog.Nop(), v: v}
		msg := &fakeMsg{subject: "wb.board.stroke.v1", data: body, hdr: hdr, order: &order, metaErr: errors.New("no meta")}
		c.onMsg(context.Background(), msg)
		if msg.acked || !msg.naked {
			t.Fatalf("ack=%v nak=%v", msg.acked, msg.naked)
		}
	})

	t.Run("poison dlq", func(t *testing.T) {
		nc := startNATS(t)
		sub, err := nc.SubscribeSync("wb.dlq.wb.board.stroke.v1")
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		c := &Consumer{apply: &fakeApply{order: &order, err: errors.New("db down")}, nc: nc, log: zerolog.Nop(), v: v}
		msg := &fakeMsg{subject: "wb.board.stroke.v1", data: body, hdr: hdr, order: &order, delivered: maxDeliver}
		c.onMsg(context.Background(), msg)
		if !msg.acked || msg.naked {
			t.Fatalf("ack=%v nak=%v", msg.acked, msg.naked)
		}
		dlq, err := sub.NextMsg(3 * time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if got := dlq.Header.Get(errorHeader); !strings.Contains(got, "db down") {
			t.Fatalf("header %q", got)
		}
	})

	t.Run("dlq publish failure nak", func(t *testing.T) {
		var order []string
		c := &Consumer{apply: &fakeApply{order: &order, err: errors.New("db down")}, log: zerolog.Nop(), v: v}
		msg := &fakeMsg{subject: "wb.board.stroke.v1", data: body, hdr: hdr, order: &order, delivered: maxDeliver}
		c.onMsg(context.Background(), msg)
		if msg.acked || !msg.naked {
			t.Fatalf("ack=%v nak=%v", msg.acked, msg.naked)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		var order []string
		c := &Consumer{apply: &fakeApply{order: &order}, log: zerolog.Nop(), v: v}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		msg := &fakeMsg{subject: "wb.board.stroke.v1", data: body, hdr: hdr, order: &order}
		c.onMsg(ctx, msg)
		if msg.acked || msg.naked {
			t.Fatal("expected no ack/nak")
		}
	})
}

func TestBackoffDelay(t *testing.T) {
	if backoffDelay(0) != 100*time.Millisecond || backoffDelay(3) != 400*time.Millisecond || backoffDelay(9) != 800*time.Millisecond {
		t.Fatal(backoffDelay(0), backoffDelay(3), backoffDelay(9))
	}
}

func startNATS(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	t.Cleanup(ns.Shutdown)
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc
}
