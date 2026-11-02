package call

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
)

func TestRoster(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	r := New(rdb)
	roomID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	ada := Participant{UserID: "11111111-1111-4111-8111-111111111111", Name: "ada"}
	bea := Participant{UserID: "22222222-2222-4222-8222-222222222222", Name: "bea"}

	first, err := r.Join(ctx, roomID, ada)
	if err != nil || first.Joined == nil || len(first.State.GetParticipants()) != 1 {
		t.Fatalf("join %+v %v", first, err)
	}
	again, err := r.Join(ctx, roomID, ada)
	if err != nil || again.Joined != nil {
		t.Fatalf("rejoin %+v %v", again, err)
	}
	if _, err := r.Toggle(ctx, roomID, bea.UserID, realtimev1.MediaKind_MEDIA_KIND_MIC, true); !errors.Is(err, ErrNotInCall) {
		t.Fatalf("toggle outsider %v", err)
	}
	second, err := r.Join(ctx, roomID, bea)
	if err != nil || second.Joined.GetUserId() != bea.UserID {
		t.Fatal(err)
	}
	tog, err := r.Toggle(ctx, roomID, ada.UserID, realtimev1.MediaKind_MEDIA_KIND_MIC, true)
	if err != nil || !tog.GetEnabled() {
		t.Fatal(err)
	}
	left, err := r.Leave(ctx, roomID, ada.UserID)
	if err != nil || left.Left == nil || left.Ended != nil || len(left.Left.GetParticipants()) != 1 {
		t.Fatalf("leave %+v %v", left, err)
	}
	end, err := r.Leave(ctx, roomID, bea.UserID)
	if err != nil || end.Ended == nil {
		t.Fatalf("end %+v %v", end, err)
	}
	if n, err := rdb.Exists(ctx, "call:"+roomID).Result(); err != nil || n != 0 {
		t.Fatalf("roster key %d %v", n, err)
	}
	empty, err := r.Leave(ctx, roomID, ada.UserID)
	if err != nil || empty.Left != nil || empty.Ended != nil {
		t.Fatalf("idempotent %+v %v", empty, err)
	}
}
