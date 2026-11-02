// Package fakeroom is an in-memory RoomService for gateway tests.
package fakeroom

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
)

// Server implements RoomService with caller identity taken from metadata.
type Server struct {
	roomv1.UnimplementedRoomServiceServer
	mu    sync.Mutex
	rooms map[string]*rec
}

type rec struct {
	title   string
	admin   string
	members map[string]roomv1.Role
	names   map[string]string
}

// New returns an empty server.
func New() *Server {
	return &Server{rooms: map[string]*rec{}}
}

func (s *Server) CreateRoom(ctx context.Context, req *roomv1.CreateRoomRequest) (*roomv1.CreateRoomResponse, error) {
	id, name, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetTitle() == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid argument")
	}
	roomID := uuid.NewString()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rooms[roomID] = &rec{
		title:   req.GetTitle(),
		admin:   id,
		members: map[string]roomv1.Role{id: roomv1.Role_ROLE_ADMIN},
		names:   map[string]string{id: name},
	}
	return &roomv1.CreateRoomResponse{Room: s.roomLocked(roomID)}, nil
}

func (s *Server) GetRoom(ctx context.Context, req *roomv1.GetRoomRequest) (*roomv1.GetRoomResponse, error) {
	id, _, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if _, ok := room.members[id]; !ok {
		return nil, status.Error(codes.PermissionDenied, "not a member")
	}
	return &roomv1.GetRoomResponse{Room: s.roomLocked(req.GetRoomId())}, nil
}

func (s *Server) DeleteRoom(ctx context.Context, req *roomv1.DeleteRoomRequest) (*roomv1.DeleteRoomResponse, error) {
	id, _, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if room.members[id] != roomv1.Role_ROLE_ADMIN {
		return nil, status.Error(codes.PermissionDenied, "not admin")
	}
	delete(s.rooms, req.GetRoomId())
	return &roomv1.DeleteRoomResponse{}, nil
}

func (s *Server) JoinRoom(ctx context.Context, req *roomv1.JoinRoomRequest) (*roomv1.JoinRoomResponse, error) {
	id, name, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if req.GetInviteToken() == "bad" {
		return nil, status.Error(codes.FailedPrecondition, "bad invite")
	}
	role, ok := room.members[id]
	if !ok {
		role = roomv1.Role_ROLE_MEMBER
		room.members[id] = role
		room.names[id] = name
	}
	return &roomv1.JoinRoomResponse{Room: s.roomLocked(req.GetRoomId()), Role: role}, nil
}

func (s *Server) LeaveRoom(ctx context.Context, req *roomv1.LeaveRoomRequest) (*roomv1.LeaveRoomResponse, error) {
	id, _, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if _, ok := room.members[id]; !ok {
		return nil, status.Error(codes.NotFound, "not found")
	}
	delete(room.members, id)
	return &roomv1.LeaveRoomResponse{}, nil
}

func (s *Server) ListMembers(ctx context.Context, req *roomv1.ListMembersRequest) (*roomv1.ListMembersResponse, error) {
	id, _, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if _, ok := room.members[id]; !ok {
		return nil, status.Error(codes.PermissionDenied, "not a member")
	}
	out := &roomv1.ListMembersResponse{}
	for userID, role := range room.members {
		name := room.names[userID]
		if name == "" {
			name = "user"
		}
		out.Members = append(out.Members, &roomv1.Member{
			UserId:   userID,
			Username: name,
			Role:     role,
			JoinedAt: timestamppb.New(time.Unix(0, 0).UTC()),
		})
	}
	return out, nil
}

func (s *Server) CheckMembership(ctx context.Context, req *roomv1.CheckMembershipRequest) (*roomv1.CheckMembershipResponse, error) {
	if _, _, err := caller(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return &roomv1.CheckMembershipResponse{Role: roomv1.Role_ROLE_NONE}, nil
	}
	role := room.members[req.GetUserId()]
	if role == 0 {
		role = roomv1.Role_ROLE_NONE
	}
	return &roomv1.CheckMembershipResponse{Role: role}, nil
}

func (s *Server) CreateInvite(ctx context.Context, req *roomv1.CreateInviteRequest) (*roomv1.CreateInviteResponse, error) {
	id, _, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if room.members[id] != roomv1.Role_ROLE_ADMIN {
		return nil, status.Error(codes.PermissionDenied, "not admin")
	}
	return &roomv1.CreateInviteResponse{InviteId: uuid.NewString(), Token: "invite-token-0123456789", ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))}, nil
}

func (s *Server) TransferAdmin(ctx context.Context, req *roomv1.TransferAdminRequest) (*roomv1.TransferAdminResponse, error) {
	id, _, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[req.GetRoomId()]
	if room == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if room.members[id] != roomv1.Role_ROLE_ADMIN {
		return nil, status.Error(codes.PermissionDenied, "not admin")
	}
	if _, ok := room.members[req.GetNewAdminUserId()]; !ok {
		return nil, status.Error(codes.PermissionDenied, "not a member")
	}
	room.members[id] = roomv1.Role_ROLE_MEMBER
	room.members[req.GetNewAdminUserId()] = roomv1.Role_ROLE_ADMIN
	room.admin = req.GetNewAdminUserId()
	return &roomv1.TransferAdminResponse{}, nil
}

func (s *Server) ValidateInvite(ctx context.Context, req *roomv1.ValidateInviteRequest) (*roomv1.ValidateInviteResponse, error) {
	if _, _, err := caller(ctx); err != nil {
		return nil, err
	}
	if req.GetToken() != "invite-token-0123456789" {
		return &roomv1.ValidateInviteResponse{Valid: false}, nil
	}
	return &roomv1.ValidateInviteResponse{Valid: true}, nil
}

func (s *Server) roomLocked(id string) *roomv1.Room {
	room := s.rooms[id]
	return &roomv1.Room{Id: id, AdminId: room.admin, Title: room.title, CreatedAt: timestamppb.New(time.Unix(0, 0).UTC())}
}

func caller(ctx context.Context) (string, string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "unauthenticated")
	}
	ids := md.Get("wb-user-id")
	if len(ids) == 0 || ids[0] == "" {
		return "", "", status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if _, err := uuid.Parse(ids[0]); err != nil {
		return "", "", status.Error(codes.InvalidArgument, "invalid argument")
	}
	name := "user"
	if names := md.Get("wb-username"); len(names) > 0 && names[0] != "" {
		name = names[0]
	}
	return ids[0], name, nil
}
