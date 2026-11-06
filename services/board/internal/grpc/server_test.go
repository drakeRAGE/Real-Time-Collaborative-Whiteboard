package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/service"
)

func TestMapErr(t *testing.T) {
	srv := New(nil, zerolog.Nop())
	tests := []struct {
		err  error
		code codes.Code
	}{
		{service.ErrNotFound, codes.NotFound},
		{service.ErrNotAdmin, codes.PermissionDenied},
		{service.ErrNotMember, codes.PermissionDenied},
		{service.ErrUnauthenticated, codes.Unauthenticated},
		{service.ErrInvalidArgument, codes.InvalidArgument},
		{service.ErrNothingToUndo, codes.FailedPrecondition},
		{service.ErrRedoRejected, codes.FailedPrecondition},
		{errors.New("db"), codes.Internal},
	}
	for _, tt := range tests {
		if got := status.Code(srv.mapErr(context.Background(), tt.err)); got != tt.code {
			t.Fatalf("%v -> %s", tt.err, got)
		}
	}
}

func TestHistoryAndSnapshot(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"points": []map[string]float64{{"x": 1, "y": 2}},
		"color":  "#112233",
		"width":  2,
		"tool":   "pen",
	})
	if err != nil {
		t.Fatal(err)
	}
	op := repo.Op{
		ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", RoomID: room, Seq: 1, UserID: user,
		Kind: repo.KindStroke, Payload: payload, CreatedAt: time.Unix(5, 0).UTC(),
	}
	srv := New(service.New(histStore{op: op}, roles{"member"}, readyCache{}, nopPub{}), zerolog.Nop())
	if _, err := srv.GetHistory(context.Background(), &boardv1.GetHistoryRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad id %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("wb-user-id", user.String()))
	hist, err := srv.GetHistory(ctx, &boardv1.GetHistoryRequest{RoomId: room.String(), Limit: 10})
	if err != nil || len(hist.GetOps()) != 1 || hist.GetOps()[0].GetStrokeAdded().GetStroke().GetColor() != "#112233" {
		t.Fatalf("history %+v %v", hist, err)
	}
	snap, err := srv.GetSnapshot(ctx, &boardv1.GetSnapshotRequest{RoomId: room.String()})
	if err != nil || snap.GetUptoSeq() != 1 || len(snap.GetActiveOps()) != 1 {
		t.Fatalf("snap %+v %v", snap, err)
	}
	if _, err := srv.GetSnapshot(ctx, &boardv1.GetSnapshotRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad snap %v", err)
	}
}

type roles struct{ role string }

func (r roles) Role(context.Context, uuid.UUID, uuid.UUID) (string, error) { return r.role, nil }

type readyCache struct{}

func (readyCache) Append(context.Context, string, []byte) error    { return nil }
func (readyCache) Replace(context.Context, string, [][]byte) error { return nil }
func (readyCache) Exists(context.Context, string) (bool, error)    { return true, nil }

type nopPub struct{}

func (nopPub) Publish(context.Context, string, string, proto.Message) error { return nil }

type histStore struct{ op repo.Op }

func (s histStore) WithRoomLock(context.Context, uuid.UUID, func(repo.Tx) error) error { return nil }
func (s histStore) ListRoom(context.Context, uuid.UUID) ([]repo.Op, error) {
	return []repo.Op{s.op}, nil
}
func (s histStore) ListAfter(context.Context, uuid.UUID, int64) ([]repo.Op, error) {
	return []repo.Op{s.op}, nil
}
func (s histStore) ListRecent(context.Context, uuid.UUID, int32) ([]repo.Op, error) {
	return []repo.Op{s.op}, nil
}
func (s histStore) LatestSnapshot(context.Context, uuid.UUID) (repo.Snapshot, error) {
	return repo.Snapshot{}, repo.ErrNotFound
}
