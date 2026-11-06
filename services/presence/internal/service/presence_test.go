package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/store"
)

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis %s: %v", addr, err)
	}
	return rdb
}

func testNATS(t *testing.T) *nats.Conn {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(url, nats.Timeout(2*time.Second))
	if err != nil {
		t.Fatalf("nats %s: %v", url, err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func TestPrincipalFromNATS(t *testing.T) {
	id := uuid.New()
	got, name, err := principalFromNATS(nats.Header{"Wb-User-Id": []string{id.String()}, "Wb-Username": []string{"ada"}})
	if err != nil || got != id || name != "ada" {
		t.Fatalf("%s %s %v", got, name, err)
	}
	_, name, err = principalFromNATS(nats.Header{"Wb-User-Id": []string{id.String()}})
	if err != nil || name != "user" {
		t.Fatalf("default %s %v", name, err)
	}
	if _, _, err := principalFromNATS(nil); err == nil {
		t.Fatal("expected missing user")
	}
	if _, _, err := principalFromNATS(nats.Header{"Wb-User-Id": []string{"nope"}}); err == nil {
		t.Fatal("expected bad user")
	}
}

func TestHandleCursorAndSweep(t *testing.T) {
	rdb := testRedis(t)
	nc := testNATS(t)
	st := store.New(rdb)
	svc := New(st, nc)
	room := uuid.NewString()
	user := uuid.New()
	body, err := proto.Marshal(&presencev1.CursorMoved{RoomId: room, X: 3, Y: 4})
	if err != nil {
		t.Fatal(err)
	}
	hdr := nats.Header{"Wb-User-Id": []string{user.String()}, "Wb-Username": []string{"ada"}}
	if err := svc.HandleCursor(context.Background(), body, hdr); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCursor(context.Background(), room, user.String())
	if err != nil {
		t.Fatal(err)
	}
	if got.GetUsername() != "ada" || got.GetColor() == "" || got.GetX() != 3 {
		t.Fatalf("cursor %+v", got)
	}
	if err := svc.HandleCursor(context.Background(), []byte{0xff}, hdr); err == nil {
		t.Fatal("expected unmarshal error")
	}
	if err := svc.HandleCursor(context.Background(), body, nil); err == nil {
		t.Fatal("expected missing principal")
	}

	staleUser := uuid.New()
	old := &presencev1.CursorMoved{RoomId: room, UserId: staleUser.String(), X: 1, Y: 1}
	if err := st.SaveCursor(context.Background(), old, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	sub, err := nc.SubscribeSync(subjectOffline)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SweepOffline(context.Background(), time.Hour); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var ev presencev1.UserOffline
	for time.Now().Before(deadline) {
		msg, err := sub.NextMsg(time.Until(deadline))
		if err != nil {
			t.Fatal(err)
		}
		ev.Reset()
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.GetUserId() == staleUser.String() {
			break
		}
	}
	if ev.GetRoomId() != room || ev.GetUserId() != staleUser.String() {
		t.Fatalf("offline %+v", ev.String())
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = rdb.Del(ctx, "presence:"+room+":"+user.String(), "presence:"+room+":"+staleUser.String(), "presence_hb:"+room).Err()
	})
}
