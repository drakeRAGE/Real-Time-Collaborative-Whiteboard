package grpc

import (
	"context"
	"errors"

	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/service"
)

// Server is the gRPC transport for RoomService. Identity is not read here;
// the service reads it from metadata.
type Server struct {
	roomv1.UnimplementedRoomServiceServer
	svc *service.Service
	log zerolog.Logger
}

// New returns a server that maps service errors onto gRPC status codes.
func New(svc *service.Service, log zerolog.Logger) *Server {
	return &Server{svc: svc, log: log}
}

func (s *Server) CreateRoom(ctx context.Context, req *roomv1.CreateRoomRequest) (*roomv1.CreateRoomResponse, error) {
	room, err := s.svc.CreateRoom(ctx, req.GetTitle())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &roomv1.CreateRoomResponse{Room: toProtoRoom(room)}, nil
}

func (s *Server) GetRoom(ctx context.Context, req *roomv1.GetRoomRequest) (*roomv1.GetRoomResponse, error) {
	id, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	room, err := s.svc.GetRoom(ctx, id)
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &roomv1.GetRoomResponse{Room: toProtoRoom(room)}, nil
}

func (s *Server) JoinRoom(ctx context.Context, req *roomv1.JoinRoomRequest) (*roomv1.JoinRoomResponse, error) {
	id, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	room, role, err := s.svc.JoinRoom(ctx, id, req.GetInviteToken())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &roomv1.JoinRoomResponse{Room: toProtoRoom(room), Role: toProtoRole(role)}, nil
}

func (s *Server) LeaveRoom(ctx context.Context, req *roomv1.LeaveRoomRequest) (*roomv1.LeaveRoomResponse, error) {
	id, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	res, err := s.svc.LeaveRoom(ctx, id)
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	out := &roomv1.LeaveRoomResponse{RoomDeleted: res.Deleted}
	if res.NewAdminID != uuid.Nil {
		out.NewAdminId = res.NewAdminID.String()
	}
	return out, nil
}

func (s *Server) DeleteRoom(ctx context.Context, req *roomv1.DeleteRoomRequest) (*roomv1.DeleteRoomResponse, error) {
	id, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	if err := s.svc.DeleteRoom(ctx, id); err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &roomv1.DeleteRoomResponse{}, nil
}

func (s *Server) ListMembers(ctx context.Context, req *roomv1.ListMembersRequest) (*roomv1.ListMembersResponse, error) {
	id, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	members, err := s.svc.ListMembers(ctx, id)
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	out := make([]*roomv1.Member, 0, len(members))
	for _, m := range members {
		out = append(out, toProtoMember(m))
	}
	return &roomv1.ListMembersResponse{Members: out}, nil
}

func (s *Server) TransferAdmin(ctx context.Context, req *roomv1.TransferAdminRequest) (*roomv1.TransferAdminResponse, error) {
	roomID, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	next, err := parseID(req.GetNewAdminUserId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	if err := s.svc.TransferAdmin(ctx, roomID, next); err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &roomv1.TransferAdminResponse{}, nil
}

func (s *Server) CreateInvite(ctx context.Context, req *roomv1.CreateInviteRequest) (*roomv1.CreateInviteResponse, error) {
	id, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	inv, err := s.svc.CreateInvite(ctx, id, time.Duration(req.GetExpiresInSeconds())*time.Second, req.GetMaxUses())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &roomv1.CreateInviteResponse{
		InviteId:  inv.ID.String(),
		Token:     inv.Token,
		ExpiresAt: timestamppb.New(inv.ExpiresAt),
	}, nil
}

func (s *Server) ValidateInvite(ctx context.Context, req *roomv1.ValidateInviteRequest) (*roomv1.ValidateInviteResponse, error) {
	valid, roomID, err := s.svc.ValidateInvite(ctx, req.GetToken())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	out := &roomv1.ValidateInviteResponse{Valid: valid}
	if valid {
		out.RoomId = roomID.String()
	}
	return out, nil
}

func (s *Server) CheckMembership(ctx context.Context, req *roomv1.CheckMembershipRequest) (*roomv1.CheckMembershipResponse, error) {
	roomID, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	// user_id is the subject the gateway is asking about, not the caller.
	userID, err := parseID(req.GetUserId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	role, err := s.svc.CheckMembership(ctx, roomID, userID)
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &roomv1.CheckMembershipResponse{Role: toProtoRole(role)}, nil
}

func (s *Server) mapErr(ctx context.Context, err error) error {
	code := codes.Internal
	msg := "internal error"
	switch {
	case errors.Is(err, service.ErrNotFound), errors.Is(err, repo.ErrNotFound):
		code, msg = codes.NotFound, "not found"
	case errors.Is(err, service.ErrNotAdmin):
		code, msg = codes.PermissionDenied, "not admin"
	case errors.Is(err, service.ErrNotMember):
		code, msg = codes.PermissionDenied, "not a member"
	case errors.Is(err, service.ErrBadInvite):
		code, msg = codes.FailedPrecondition, "bad invite"
	case errors.Is(err, service.ErrUnauthenticated):
		code, msg = codes.Unauthenticated, "unauthenticated"
	case errors.Is(err, service.ErrInvalidArgument):
		code, msg = codes.InvalidArgument, "invalid argument"
	}
	if code == codes.Internal {
		entry := logger.WithTrace(ctx, s.log)
		entry.Error().Err(err).Msg("room rpc")
	}
	return status.Error(code, msg)
}

func parseID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, service.ErrInvalidArgument
	}
	return parsed, nil
}

func toProtoRoom(room service.RoomView) *roomv1.Room {
	out := &roomv1.Room{
		Id:        room.ID.String(),
		Title:     room.Title,
		CreatedAt: timestamppb.New(room.CreatedAt),
	}
	if room.AdminID != uuid.Nil {
		out.AdminId = room.AdminID.String()
	}
	return out
}

func toProtoMember(m repo.Member) *roomv1.Member {
	return &roomv1.Member{
		UserId:   m.UserID.String(),
		Username: m.Username,
		Role:     toProtoRole(m.Role),
		JoinedAt: timestamppb.New(m.JoinedAt),
	}
}

func toProtoRole(role string) roomv1.Role {
	switch role {
	case "admin":
		return roomv1.Role_ROLE_ADMIN
	case "member":
		return roomv1.Role_ROLE_MEMBER
	default:
		return roomv1.Role_ROLE_NONE
	}
}
