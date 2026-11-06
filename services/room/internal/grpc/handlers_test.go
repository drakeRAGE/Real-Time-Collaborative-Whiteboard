package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/service"
)

func TestHandlersRejectBadIDsAndMissingCaller(t *testing.T) {
	srv := New(service.New(rejectStore{}, nopPub{}), zerolog.Nop())
	room := uuid.NewString()
	if _, err := srv.GetRoom(context.Background(), &roomv1.GetRoomRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("get %v", err)
	}
	if _, err := srv.GetRoom(context.Background(), &roomv1.GetRoomRequest{RoomId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("get anon %v", err)
	}
	if _, err := srv.JoinRoom(context.Background(), &roomv1.JoinRoomRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("join %v", err)
	}
	if _, err := srv.JoinRoom(context.Background(), &roomv1.JoinRoomRequest{RoomId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("join anon %v", err)
	}
	if _, err := srv.LeaveRoom(context.Background(), &roomv1.LeaveRoomRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("leave %v", err)
	}
	if _, err := srv.LeaveRoom(context.Background(), &roomv1.LeaveRoomRequest{RoomId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("leave anon %v", err)
	}
	if _, err := srv.DeleteRoom(context.Background(), &roomv1.DeleteRoomRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("delete %v", err)
	}
	if _, err := srv.DeleteRoom(context.Background(), &roomv1.DeleteRoomRequest{RoomId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("delete anon %v", err)
	}
	if _, err := srv.ListMembers(context.Background(), &roomv1.ListMembersRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("list %v", err)
	}
	if _, err := srv.ListMembers(context.Background(), &roomv1.ListMembersRequest{RoomId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("list anon %v", err)
	}
	if _, err := srv.TransferAdmin(context.Background(), &roomv1.TransferAdminRequest{RoomId: "nope", NewAdminUserId: room}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("transfer room %v", err)
	}
	if _, err := srv.TransferAdmin(context.Background(), &roomv1.TransferAdminRequest{RoomId: room, NewAdminUserId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("transfer user %v", err)
	}
	if _, err := srv.TransferAdmin(context.Background(), &roomv1.TransferAdminRequest{RoomId: room, NewAdminUserId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("transfer anon %v", err)
	}
	if _, err := srv.CreateInvite(context.Background(), &roomv1.CreateInviteRequest{RoomId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invite %v", err)
	}
	if _, err := srv.CreateInvite(context.Background(), &roomv1.CreateInviteRequest{RoomId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("invite anon %v", err)
	}
	if _, err := srv.ValidateInvite(context.Background(), &roomv1.ValidateInviteRequest{Token: "x"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("validate %v", err)
	}
	if _, err := srv.CheckMembership(context.Background(), &roomv1.CheckMembershipRequest{RoomId: "nope", UserId: room}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("check room %v", err)
	}
	if _, err := srv.CheckMembership(context.Background(), &roomv1.CheckMembershipRequest{RoomId: room, UserId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("check user %v", err)
	}
	if _, err := srv.CheckMembership(context.Background(), &roomv1.CheckMembershipRequest{RoomId: room, UserId: room}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("check anon %v", err)
	}
}
