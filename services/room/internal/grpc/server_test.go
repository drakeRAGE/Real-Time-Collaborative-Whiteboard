package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/service"
)

func TestMapErr(t *testing.T) {
	t.Parallel()
	srv := New(nil, zerolog.Nop())
	tests := []struct {
		err  error
		code codes.Code
	}{
		{service.ErrNotFound, codes.NotFound},
		{repo.ErrNotFound, codes.NotFound},
		{service.ErrNotAdmin, codes.PermissionDenied},
		{service.ErrNotMember, codes.PermissionDenied},
		{service.ErrBadInvite, codes.FailedPrecondition},
		{service.ErrUnauthenticated, codes.Unauthenticated},
		{service.ErrInvalidArgument, codes.InvalidArgument},
		{errors.New("db down"), codes.Internal},
	}
	for _, tt := range tests {
		if got := status.Code(srv.mapErr(context.Background(), tt.err)); got != tt.code {
			t.Fatalf("%v -> %s", tt.err, got)
		}
	}
}

func TestCreateRoomRequiresMetadata(t *testing.T) {
	t.Parallel()
	svc := service.New(rejectStore{}, nopPub{})
	_, err := New(svc, zerolog.Nop()).CreateRoom(context.Background(), &roomv1.CreateRoomRequest{Title: "Studio"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code %s err %v", status.Code(err), err)
	}
}

type nopPub struct{}

func (nopPub) Publish(context.Context, string, []byte, string, string) error { return nil }

// rejectStore fails if a handler reaches the database without a principal.
type rejectStore struct{}

var _ repo.Store = rejectStore{}

func (rejectStore) WithTx(context.Context, func(repo.Tx) error) error {
	return errors.New("store should not be called")
}

func (rejectStore) CreateRoom(context.Context, uuid.UUID, string) (repo.Room, error) {
	return repo.Room{}, errUnused
}
func (rejectStore) GetRoom(context.Context, uuid.UUID) (repo.Room, error) {
	return repo.Room{}, errUnused
}
func (rejectStore) SoftDeleteRoom(context.Context, uuid.UUID) error { return errUnused }
func (rejectStore) UpsertMember(context.Context, uuid.UUID, uuid.UUID, string) (repo.Member, error) {
	return repo.Member{}, errUnused
}
func (rejectStore) DeleteMember(context.Context, uuid.UUID, uuid.UUID) error { return errUnused }
func (rejectStore) ListMembers(context.Context, uuid.UUID) ([]repo.Member, error) {
	return nil, errUnused
}
func (rejectStore) GetMember(context.Context, uuid.UUID, uuid.UUID) (repo.Member, error) {
	return repo.Member{}, errUnused
}
func (rejectStore) GetMemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return "", errUnused
}
func (rejectStore) TransferAdmin(context.Context, uuid.UUID, uuid.UUID) error { return errUnused }
func (rejectStore) CreateInvite(context.Context, uuid.UUID, []byte, time.Time, int32) (repo.Invite, error) {
	return repo.Invite{}, errUnused
}
func (rejectStore) GetInviteByHash(context.Context, []byte) (repo.Invite, error) {
	return repo.Invite{}, errUnused
}
func (rejectStore) IncrementInviteUse(context.Context, uuid.UUID) error { return errUnused }
func (rejectStore) InsertAudit(context.Context, uuid.UUID, string, string, uuid.UUID, []byte) error {
	return errUnused
}

var errUnused = errors.New("unused")
