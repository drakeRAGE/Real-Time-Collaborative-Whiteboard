package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/service"
)

// Server is the gRPC transport for ChatService.
type Server struct {
	chatv1.UnimplementedChatServiceServer
	svc *service.Service
	log zerolog.Logger
}

// New returns a server that maps service errors onto gRPC status codes.
func New(svc *service.Service, log zerolog.Logger) *Server {
	return &Server{svc: svc, log: log}
}

func (s *Server) GetHistory(ctx context.Context, req *chatv1.GetHistoryRequest) (*chatv1.GetHistoryResponse, error) {
	roomID, err := uuid.Parse(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, service.ErrInvalidArgument)
	}
	msgs, next, err := s.svc.GetHistory(ctx, roomID, req.GetCursor(), req.GetLimit())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &chatv1.GetHistoryResponse{Messages: msgs, NextCursor: next}, nil
}

func (s *Server) mapErr(ctx context.Context, err error) error {
	code := codes.Internal
	msg := "internal error"
	switch {
	case errors.Is(err, service.ErrNotFound):
		code, msg = codes.NotFound, "not found"
	case errors.Is(err, service.ErrNotMember):
		code, msg = codes.PermissionDenied, "not a member"
	case errors.Is(err, service.ErrUnauthenticated):
		code, msg = codes.Unauthenticated, "unauthenticated"
	case errors.Is(err, service.ErrInvalidArgument):
		code, msg = codes.InvalidArgument, "invalid argument"
	}
	if code == codes.Internal {
		entry := logger.WithTrace(ctx, s.log)
		entry.Error().Err(err).Msg("chat rpc")
	}
	return status.Error(code, msg)
}
