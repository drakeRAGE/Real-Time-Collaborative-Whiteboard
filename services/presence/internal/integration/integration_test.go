//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	natsc "github.com/testcontainers/testcontainers-go/modules/nats"
	redisc "github.com/testcontainers/testcontainers-go/modules/redis"
	"google.golang.org/protobuf/proto"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	"github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/service"
	"github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/store"
)

func TestPresenceService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	natsURL := startNATS(t, ctx)
	redisAddr := startRedis(t, ctx)

	nc, err := nats.Connect(natsURL, nats.Name("presence-integration"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)

	rdb := redisx.New(redisAddr)
	st := store.New(rdb)
	svc := service.New(st, nc)

	offlineSub, err := nc.SubscribeSync("wb.presence.user_offline.v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nc.Subscribe("wb.presence.cursor.v1", func(msg *nats.Msg) {
		_ = svc.HandleCursor(ctx, msg.Data, msg.Header)
	}); err != nil {
		t.Fatal(err)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = svc.SweepOffline(ctx, 15*time.Second)
			}
		}
	}()
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	roomID := uuid.New().String()
	userID := uuid.New().String()
	move := &presencev1.CursorMoved{RoomId: roomID, UserId: userID, X: 10, Y: 20}
	data, err := proto.Marshal(move)
	if err != nil {
		t.Fatal(err)
	}
	msg := &nats.Msg{Subject: "wb.presence.cursor.v1", Data: data, Header: nats.Header{}}
	msg.Header.Set("Wb-User-Id", userID)
	msg.Header.Set("Wb-Username", "pat")
	if err := nc.PublishMsg(msg); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := st.GetCursor(ctx, roomID, userID)
		if err == nil && got.GetX() == 10 && got.GetColor() != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	got, err := st.GetCursor(ctx, roomID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetUsername() != "pat" || got.GetColor() == "" {
		t.Fatalf("cursor %+v", got)
	}

	offlineMsg, err := offlineSub.NextMsg(20 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var offline presencev1.UserOffline
	if err := proto.Unmarshal(offlineMsg.Data, &offline); err != nil {
		t.Fatal(err)
	}
	if offline.GetRoomId() != roomID || offline.GetUserId() != userID {
		t.Fatalf("offline %+v", &offline)
	}
}

func startNATS(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := natsc.Run(ctx, "nats:2.10-alpine")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ctr.Terminate(cctx)
	})
	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return url
}

func startRedis(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := redisc.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ctr.Terminate(cctx)
	})
	endpoint, err := ctr.Endpoint(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}
