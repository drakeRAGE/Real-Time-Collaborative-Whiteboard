package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
)

type errRole struct{ err error }

func (e errRole) Role(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return "", e.err
}

type coldCache struct{}

func (coldCache) Append(context.Context, string, []byte) error    { return nil }
func (coldCache) Replace(context.Context, string, [][]byte) error { return nil }
func (coldCache) Exists(context.Context, string) (bool, error)    { return false, nil }

type failingPub struct{}

func (failingPub) Publish(context.Context, string, string, proto.Message) error {
	return errors.New("nats")
}

func TestHistoryShapeAndGuards(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &mem{}
	svc := New(store, roles{"member"}, coldCache{}, nopPub{})
	ctx := userCtx(user)
	if _, _, err := svc.GetHistory(context.Background(), room, "", 1); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("anon %v", err)
	}
	if _, _, err := svc.GetHistory(ctx, uuid.Nil, "", 1); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil room %v", err)
	}
	if _, _, _, err := svc.GetSnapshot(ctx, uuid.Nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil snap %v", err)
	}
	stranger := New(store, roles{"nope"}, nopCache{}, nopPub{})
	if _, _, err := stranger.GetHistory(ctx, room, "", 1); !errors.Is(err, ErrNotMember) {
		t.Fatalf("stranger %v", err)
	}
	if err := stranger.ApplyStroke(context.Background(), user, strokeOf(room, testID(t))); !errors.Is(err, ErrNotMember) {
		t.Fatalf("stroke stranger %v", err)
	}
	down := New(store, errRole{err: errors.New("room")}, nopCache{}, nopPub{})
	if err := down.ApplyStroke(context.Background(), user, strokeOf(room, testID(t))); err == nil {
		t.Fatal("expected membership error")
	}
	if err := svc.ApplyStroke(context.Background(), uuid.Nil, strokeOf(room, testID(t))); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("nil principal %v", err)
	}
	if err := svc.ApplyStroke(context.Background(), user, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil stroke %v", err)
	}
	if err := svc.ApplyStroke(context.Background(), user, &realtimev1.StrokeInput{StrokeId: "bad", RoomId: room.String()}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bad id %v", err)
	}
	if err := svc.ApplyShape(context.Background(), user, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil shape %v", err)
	}
	shapeID := testID(t)
	shape := &realtimev1.ShapeInput{
		ShapeId: shapeID, RoomId: room.String(), Kind: boardv1.ShapeKind_SHAPE_KIND_RECT,
		Start: &boardv1.Point{X: 1, Y: 2}, End: &boardv1.Point{X: 3, Y: 4},
		Color: "#abcdef", Width: 2,
	}
	if err := svc.ApplyShape(context.Background(), user, shape); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := svc.ApplyStroke(context.Background(), user, strokeOf(room, testID(t))); err != nil {
			t.Fatal(err)
		}
	}
	page, next, err := svc.GetHistory(ctx, room, "", 2)
	if err != nil || len(page) != 2 || next == "" {
		t.Fatalf("page %d next %q err %v", len(page), next, err)
	}
	rest, next2, err := svc.GetHistory(ctx, room, next, 10)
	if err != nil || len(rest) == 0 || next2 != "" {
		t.Fatalf("rest %d next %q err %v", len(rest), next2, err)
	}
	if _, _, err := svc.GetHistory(ctx, room, "nope", 1); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("cursor %v", err)
	}
	empty, _, _, err := New(&mem{}, roles{"member"}, nopCache{}, nopPub{}).GetSnapshot(ctx, room)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty snap %+v %v", empty, err)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.ApplyStroke(cancelCtx, user, strokeOf(room, testID(t))); err == nil {
		t.Fatal("expected canceled context")
	}
	admin := New(&mem{}, roles{"admin"}, nopCache{}, nopPub{})
	if err := admin.ApplyUndo(context.Background(), user, uuid.New(), testID(t)); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("undo empty %v", err)
	}
	if !IsPermanent(ErrNothingToUndo) || IsPermanent(errors.New("other")) {
		t.Fatal("IsPermanent")
	}
	if err := New(store, roles{"member"}, nopCache{}, failingPub{}).ApplyStroke(context.Background(), user, strokeOf(room, testID(t))); err == nil {
		t.Fatal("expected publish error")
	}
}

func TestBoardProtoKinds(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	now := time.Unix(10, 0).UTC()
	strokePayload, err := json.Marshal(strokeBody{
		Points: []point{{X: 1, Y: 2}, {}}, Color: "#112233", Width: 2, Tool: "eraser",
	})
	if err != nil {
		t.Fatal(err)
	}
	op := repo.Op{ID: testID(t), RoomID: room, UserID: user, Seq: 4, Kind: repo.KindStroke, Payload: strokePayload, CreatedAt: now}
	msg, err := BoardProto(op)
	if err != nil || msg.GetStrokeAdded().GetStroke().GetTool() != boardv1.Tool_TOOL_ERASER {
		t.Fatalf("stroke %+v %v", msg, err)
	}
	shapePayload, err := json.Marshal(shapeBody{
		Shape: "circle", Start: point{X: 1, Y: 1}, End: point{X: 2, Y: 2}, Color: "#abcdef", Width: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	op.Kind = repo.KindShape
	op.Payload = shapePayload
	msg, err = BoardProto(op)
	if err != nil || msg.GetShapeAdded().GetShape().GetKind() != boardv1.ShapeKind_SHAPE_KIND_CIRCLE {
		t.Fatalf("shape %+v %v", msg, err)
	}
	op.Payload, _ = json.Marshal(shapeBody{Shape: "rect"})
	if _, err := BoardProto(op); err != nil {
		t.Fatal(err)
	}
	op.Payload, _ = json.Marshal(shapeBody{Shape: "line"})
	if _, err := BoardProto(op); err != nil {
		t.Fatal(err)
	}
	op.Kind = repo.KindClear
	op.Payload = []byte(`{}`)
	if _, err := BoardProto(op); err != nil {
		t.Fatal(err)
	}
	op.Kind = repo.KindUndo
	op.Payload = []byte(`{"op_id":"abc"}`)
	if msg, err = BoardProto(op); err != nil || msg.GetUndoApplied().GetOpId() != "abc" {
		t.Fatalf("undo %+v %v", msg, err)
	}
	op.Kind = repo.KindRedo
	if _, err := BoardProto(op); err != nil {
		t.Fatal(err)
	}
	op.Kind = repo.KindStroke
	op.Payload = []byte(`{`)
	if _, err := BoardProto(op); err == nil {
		t.Fatal("expected bad stroke json")
	}
	op.Kind = "nope"
	if _, err := BoardProto(op); err == nil {
		t.Fatal("expected unknown kind")
	}
	raw, err := Encode(repo.Op{ID: testID(t), RoomID: room, UserID: user, Seq: 1, Kind: repo.KindClear, Payload: []byte(`{}`), CreatedAt: now})
	if err != nil || len(raw) == 0 {
		t.Fatalf("encode %d %v", len(raw), err)
	}
	if _, err := Encode(repo.Op{Kind: "nope"}); err == nil {
		t.Fatal("expected encode error")
	}
}

func TestSnapshotEveryThousand(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &mem{}
	svc := New(store, roles{"admin"}, nopCache{}, nopPub{})
	for i := 0; i < snapshotEvery; i++ {
		if err := svc.ApplyStroke(context.Background(), user, strokeOf(room, testID(t))); err != nil {
			t.Fatalf("stroke %d: %v", i, err)
		}
	}
	if len(store.snaps) != 1 || store.snaps[0].UptoSeq != snapshotEvery {
		t.Fatalf("snaps %+v", store.snaps)
	}
	page, _, err := svc.GetHistory(userCtx(user), room, "", 1)
	if err != nil || len(page) != 1 {
		t.Fatalf("history from snapshot %+v %v", page, err)
	}
}

func TestMetadataPrincipal(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("wb-user-id", "   "))
	if _, err := principal(ctx); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("blank %v", err)
	}
	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("wb-user-id", "nope"))
	if _, err := principal(ctx); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bad %v", err)
	}
	if ValidID("") || !ValidID(testID(t)) {
		t.Fatal("ValidID")
	}
}
