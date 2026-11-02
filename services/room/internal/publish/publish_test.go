package publish

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestPublishGuards(t *testing.T) {
	t.Parallel()
	c := &Core{}
	if err := c.Publish(context.Background(), "wb.room.deleted.v1", []byte("x"), "id", "room"); err == nil {
		t.Fatal("expected nil conn error")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Publish(canceled, "wb.room.deleted.v1", nil, "id", ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCorePublish(t *testing.T) {
	opts := &server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	t.Cleanup(ns.Shutdown)
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	sub, err := nc.SubscribeSync("wb.room.deleted.v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := New(nc).Publish(context.Background(), "", nil, "id", "room"); err == nil {
		t.Fatal("expected subject error")
	}
	roomID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	if err := New(nc).Publish(context.Background(), "wb.room.deleted.v1", []byte(roomID), "01ARZ3NDEKTSV4RRFFQ69G5FAV", roomID); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(msg.Data) != roomID || msg.Header.Get("Wb-Room-Id") != roomID || msg.Header.Get("Nats-Msg-Id") == "" {
		t.Fatalf("msg %+v header %v", string(msg.Data), msg.Header)
	}
}
