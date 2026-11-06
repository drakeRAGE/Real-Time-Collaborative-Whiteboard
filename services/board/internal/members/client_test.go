package members

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
)

type stub struct {
	roomv1.RoomServiceClient
	role roomv1.Role
	err  error
}

func (s stub) CheckMembership(context.Context, *roomv1.CheckMembershipRequest, ...grpc.CallOption) (*roomv1.CheckMembershipResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &roomv1.CheckMembershipResponse{Role: s.role}, nil
}

func TestRole(t *testing.T) {
	room := uuid.New()
	user := uuid.New()
	client := New(stub{role: roomv1.Role_ROLE_ADMIN})
	role, err := client.Role(context.Background(), room, user)
	if err != nil || role != "admin" {
		t.Fatalf("%s %v", role, err)
	}
	role, err = New(stub{role: roomv1.Role_ROLE_MEMBER}).Role(context.Background(), room, user)
	if err != nil || role != "member" {
		t.Fatalf("member %s %v", role, err)
	}
	role, err = New(stub{role: roomv1.Role_ROLE_NONE}).Role(context.Background(), room, user)
	if err != nil || role != "" {
		t.Fatalf("none %q %v", role, err)
	}
	if _, err := New(stub{err: errors.New("down")}).Role(context.Background(), room, user); err == nil {
		t.Fatal("expected membership error")
	}
}
