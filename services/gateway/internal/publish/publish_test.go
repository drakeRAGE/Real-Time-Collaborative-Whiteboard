package publish

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

func TestPublishGuards(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("expected nil conn")
	}
	nc := startNATS(t, false)
	bus, err := New(nc)
	if err != nil {
		t.Fatal(err)
	}
	user := principal.User{ID: "11111111-1111-4111-8111-111111111111", Name: "ada"}
	msg := &presencev1.CursorMoved{RoomId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bus.Publish(ctx, "wb.presence.cursor.v1", "id", msg, user, true); err == nil {
		t.Fatal("expected canceled context")
	}
	if err := bus.Publish(context.Background(), "wb.presence.cursor.v1", "", msg, user, true); err == nil {
		t.Fatal("expected id error")
	}
	if err := bus.Publish(context.Background(), "wb.presence.cursor.v1", "id", msg, principal.User{}, true); err == nil {
		t.Fatal("expected principal error")
	}
}

func TestPublishHeaders(t *testing.T) {
	nc := startNATS(t, true)
	jsx, err := natsx.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsx.EnsureStreams(context.Background()); err != nil {
		t.Fatal(err)
	}
	sub, err := nc.SubscribeSync("wb.presence.cursor.v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	bus, err := New(nc)
	if err != nil {
		t.Fatal(err)
	}
	user := principal.User{ID: "11111111-1111-4111-8111-111111111111", Name: "ada"}
	msg := &presencev1.CursorMoved{RoomId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", UserId: user.ID, X: 1, Y: 2}
	if err := bus.Publish(context.Background(), "wb.presence.cursor.v1", "01ARZ3NDEKTSV4RRFFQ69G5FAV", msg, user, true); err != nil {
		t.Fatal(err)
	}
	got, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.Header.Get("Nats-Msg-Id") != "01ARZ3NDEKTSV4RRFFQ69G5FAV" || got.Header.Get("Wb-User-Id") != user.ID || got.Header.Get("Wb-Username") != "ada" {
		t.Fatalf("headers %+v", got.Header)
	}
	var moved presencev1.CursorMoved
	if err := proto.Unmarshal(got.Data, &moved); err != nil {
		t.Fatal(err)
	}
	if moved.GetX() != 1 {
		t.Fatalf("payload %+v", &moved)
	}
	boardSub, err := nc.SubscribeSync("wb.board.stroke.v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), "wb.board.stroke.v1", "01ARZ3NDEKTSV4RRFFQ69G5FAV", msg, user, false); err != nil {
		t.Fatal(err)
	}
	if _, err := boardSub.NextMsg(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), "", "id", msg, user, true); err == nil {
		t.Fatal("expected subject error")
	}
}

func startNATS(t *testing.T, jet bool) *nats.Conn {
	t.Helper()
	opts := &server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true}
	if jet {
		opts.JetStream = true
		opts.StoreDir = t.TempDir()
	}
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
