package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
)

func TestSaveGetExpire(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis %s: %v", addr, err)
	}
	st := New(rdb)
	if err := st.SaveCursor(ctx, nil, time.Now()); err == nil {
		t.Fatal("expected nil cursor")
	}
	room := uuid.NewString()
	user := uuid.NewString()
	in := &presencev1.CursorMoved{RoomId: room, UserId: user, X: 8, Y: 9, Username: "ada"}
	if err := st.SaveCursor(ctx, in, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCursor(ctx, room, user)
	if err != nil || got.GetX() != 8 || got.GetUsername() != "ada" {
		t.Fatalf("got %+v err %v", got, err)
	}
	stale, err := st.ExpireStale(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range stale[room] {
		if id == user {
			found = true
		}
	}
	if !found {
		t.Fatalf("room %s missing from %+v", room, stale[room])
	}
	if _, err := st.GetCursor(ctx, room, user); err == nil {
		t.Fatal("cursor should be removed")
	}
}
