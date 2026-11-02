package fanout

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

const roomX = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestFanoutDeliversAppliedAndSkipsIngest(t *testing.T) {
	nc := startNATS(t)
	jsx, err := natsx.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsx.EnsureStreams(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	f := New(nc, rec, nil, memCache{}, zerolog.Nop())
	if err := f.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Stop)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ingest, err := proto.Marshal(&boardv1.Stroke{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAV", RoomId: roomX, UserId: "11111111-1111-4111-8111-111111111111"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.Publish(context.Background(), "wb.board.stroke.v1", ingest); err != nil {
		t.Fatal(err)
	}
	applied := &boardv1.StrokeAdded{Seq: 1, Stroke: &boardv1.Stroke{
		Id: "01ARZ3NDEKTSV4RRFFQ69G5FAV", RoomId: roomX, UserId: "11111111-1111-4111-8111-111111111111",
		Points: []*boardv1.Point{{X: 1, Y: 2}}, Color: "#112233", Width: 2, Tool: boardv1.Tool_TOOL_PEN,
	}}
	body, err := proto.Marshal(applied)
	if err != nil {
		t.Fatal(err)
	}
	msg := &nats.Msg{Subject: "wb.board.stroke_applied.v1", Data: body, Header: nats.Header{}}
	msg.Header.Set("Nats-Msg-Id", "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if _, err := js.PublishMsg(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if _, err := js.PublishMsg(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.envs) == 1 && rec.envs[0].GetStrokeApplied().GetSeq() == 1
	})
}

func TestCursorExcludesOriginator(t *testing.T) {
	nc := startNATS(t)
	rec := &recorder{}
	f := New(nc, rec, nil, nil, zerolog.Nop())
	if err := f.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Stop)
	moved := &presencev1.CursorMoved{RoomId: roomX, UserId: "11111111-1111-4111-8111-111111111111", X: 3, Y: 4}
	body, err := proto.Marshal(moved)
	if err != nil {
		t.Fatal(err)
	}
	msg := &nats.Msg{Subject: "wb.presence.cursor.v1", Data: body, Header: nats.Header{}}
	msg.Header.Set("Nats-Msg-Id", "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err := nc.PublishMsg(msg); err != nil {
		t.Fatal(err)
	}
	if err := nc.PublishMsg(msg); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.envs) == 1 && rec.exclude[0] == moved.GetUserId() && rec.envs[0].GetCursorMoved().GetX() == 3
	})
}

type recorder struct {
	mu      sync.Mutex
	envs    []*realtimev1.ServerEnvelope
	exclude []string
	closed  []string
}

func (r *recorder) Broadcast(roomID string, env *realtimev1.ServerEnvelope, excludeUserID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if roomID != roomX {
		return
	}
	r.envs = append(r.envs, env)
	r.exclude = append(r.exclude, excludeUserID)
}

func (r *recorder) SendUser(string, *realtimev1.ServerEnvelope) {}

func (r *recorder) AnyUser(string) (principal.User, bool) { return principal.User{}, false }

func (r *recorder) CloseRoom(roomID string) {
	r.mu.Lock()
	r.closed = append(r.closed, roomID)
	r.mu.Unlock()
}

type memCache struct{}

func (memCache) Forget(context.Context, string, string) error { return nil }
func (memCache) ForgetRoom(context.Context, string) error     { return nil }

func startNATS(t *testing.T) *nats.Conn {
	t.Helper()
	nc := dialNATS(t)
	jsx, err := natsx.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsx.EnsureStreams(context.Background()); err != nil {
		t.Fatal(err)
	}
	return nc
}

func dialNATS(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &server.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true}
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

func waitUntil(t *testing.T, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestTranslate(t *testing.T) {
	user := "11111111-1111-4111-8111-111111111111"
	must := func(msg proto.Message) []byte {
		t.Helper()
		b, err := proto.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	hdr := nats.Header{}
	hdr.Set("Wb-Room-Id", roomX)
	hdr.Set("Wb-Reason", "rate_limited")
	cases := []struct {
		subject string
		data    []byte
		header  nats.Header
		room    string
		direct  string
		exclude string
	}{
		{"wb.board.stroke_applied.v1", must(&boardv1.StrokeAdded{Seq: 1, Stroke: &boardv1.Stroke{RoomId: roomX}}), nil, roomX, "", ""},
		{"wb.board.shape_applied.v1", must(&boardv1.ShapeAdded{Seq: 1, Shape: &boardv1.Shape{RoomId: roomX}}), nil, roomX, "", ""},
		{"wb.board.cleared.v1", must(&boardv1.BoardCleared{RoomId: roomX, Seq: 1}), nil, roomX, "", ""},
		{"wb.board.undo_applied.v1", must(&boardv1.UndoApplied{RoomId: roomX, Seq: 1}), nil, roomX, "", ""},
		{"wb.board.redo_applied.v1", must(&boardv1.RedoApplied{RoomId: roomX, Seq: 1}), nil, roomX, "", ""},
		{"wb.chat.message_sent.v1", must(&chatv1.Message{RoomId: roomX, UserId: user}), nil, roomX, "", ""},
		{"wb.chat.rejected.v1", must(&chatv1.Message{RoomId: roomX, UserId: user}), hdr, "", user, ""},
		{"wb.presence.cursor.v1", must(&presencev1.CursorMoved{RoomId: roomX, UserId: user}), nil, roomX, "", user},
		{"wb.presence.user_online.v1", must(&presencev1.UserOnline{RoomId: roomX, UserId: user}), nil, roomX, "", ""},
		{"wb.presence.user_offline.v1", must(&presencev1.UserOffline{RoomId: roomX, UserId: user}), nil, roomX, "", ""},
		{"wb.room.member_joined.v1", must(&roomv1.Member{UserId: user, Username: "ada"}), hdr, roomX, "", ""},
		{"wb.room.member_left.v1", must(&roomv1.Member{UserId: user}), hdr, roomX, "", ""},
		{"wb.room.deleted.v1", []byte(roomX), nil, roomX, "", ""},
	}
	for _, tc := range cases {
		eff, err := translate(tc.subject, tc.data, tc.header)
		if err != nil || eff.RoomID != tc.room || eff.DirectUser != tc.direct || eff.ExcludeUser != tc.exclude {
			t.Fatalf("%s %+v %v", tc.subject, eff, err)
		}
	}
	if _, err := translate("wb.board.stroke_applied.v1", []byte{0xff}, nil); err == nil {
		t.Fatal("expected decode error")
	}
	eff, err := translate("wb.unknown.v1", nil, nil)
	if err != nil || eff.Env != nil {
		t.Fatalf("unknown %+v %v", eff, err)
	}
	d := newDedupe()
	if !d.once("a") || d.once("a") {
		t.Fatal("duplicate id")
	}
}
