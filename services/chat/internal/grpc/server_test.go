package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/service"
)

func TestMapErr(t *testing.T) {
	srv := New(nil, zerolog.Nop())
	tests := []struct {
		err  error
		code codes.Code
	}{
		{service.ErrNotFound, codes.NotFound},
		{service.ErrNotMember, codes.PermissionDenied},
		{service.ErrUnauthenticated, codes.Unauthenticated},
		{service.ErrInvalidArgument, codes.InvalidArgument},
		{errors.New("db"), codes.Internal},
	}
	for _, tt := range tests {
		if got := status.Code(srv.mapErr(context.Background(), tt.err)); got != tt.code {
			t.Fatalf("%v -> %s", tt.err, got)
		}
	}
}

func TestGetHistoryTransport(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	store := &memStore{member: true}
	if _, err := store.InsertMessage(context.Background(), "m1", room, user, "hello"); err != nil {
		t.Fatal(err)
	}
	srv := New(service.New(store, nopPub{}, allow{ok: true}), zerolog.Nop())
	if _, err := srv.GetHistory(context.Background(), &chatv1.GetHistoryRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad room %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("wb-user-id", user.String()))
	resp, err := srv.GetHistory(ctx, &chatv1.GetHistoryRequest{RoomId: room.String(), Limit: 10})
	if err != nil || len(resp.GetMessages()) != 1 || resp.GetMessages()[0].GetBody() != "hello" {
		t.Fatalf("resp %+v err %v", resp, err)
	}
	store.member = false
	if _, err := srv.GetHistory(ctx, &chatv1.GetHistoryRequest{RoomId: room.String()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("stranger %v", err)
	}
}

type nopPub struct{}

func (nopPub) MessageSent(context.Context, *chatv1.Message) error           { return nil }
func (nopPub) Rejected(context.Context, *chatv1.Message, string) error { return nil }

type allow struct{ ok bool }

func (a allow) AllowChat(context.Context, uuid.UUID) (bool, error) { return a.ok, nil }
