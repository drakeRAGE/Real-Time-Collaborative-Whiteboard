package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/service"
)

// Server is the gRPC transport for BoardService.
type Server struct {
	boardv1.UnimplementedBoardServiceServer
	svc *service.Service
	log zerolog.Logger
}

// New returns a server that maps service errors onto gRPC status codes.
func New(svc *service.Service, log zerolog.Logger) *Server {
	return &Server{svc: svc, log: log}
}

func (s *Server) GetHistory(ctx context.Context, req *boardv1.GetHistoryRequest) (*boardv1.GetHistoryResponse, error) {
	roomID, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	ops, next, err := s.svc.GetHistory(ctx, roomID, req.GetCursor(), req.GetLimit())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	out, err := toProtos(ops)
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &boardv1.GetHistoryResponse{Ops: out, NextCursor: next}, nil
}

func (s *Server) GetSnapshot(ctx context.Context, req *boardv1.GetSnapshotRequest) (*boardv1.GetSnapshotResponse, error) {
	roomID, err := parseID(req.GetRoomId())
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	ops, upto, at, err := s.svc.GetSnapshot(ctx, roomID)
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	out, err := toProtos(ops)
	if err != nil {
		return nil, s.mapErr(ctx, err)
	}
	return &boardv1.GetSnapshotResponse{
		RoomId:    roomID.String(),
		UptoSeq:   upto,
		ActiveOps: out,
		CreatedAt: timestamppb.New(at),
	}, nil
}

func toProtos(ops []repo.Op) ([]*boardv1.BoardOp, error) {
	out := make([]*boardv1.BoardOp, 0, len(ops))
	for _, op := range ops {
		msg, err := service.BoardProto(op)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}

func (s *Server) mapErr(ctx context.Context, err error) error {
	code := codes.Internal
	msg := "internal error"
	switch {
	case errors.Is(err, service.ErrNotFound):
		code, msg = codes.NotFound, "not found"
	case errors.Is(err, service.ErrNotAdmin):
		code, msg = codes.PermissionDenied, "not admin"
	case errors.Is(err, service.ErrNotMember):
		code, msg = codes.PermissionDenied, "not a member"
	case errors.Is(err, service.ErrUnauthenticated):
		code, msg = codes.Unauthenticated, "unauthenticated"
	case errors.Is(err, service.ErrInvalidArgument):
		code, msg = codes.InvalidArgument, "invalid argument"
	case errors.Is(err, service.ErrNothingToUndo), errors.Is(err, service.ErrRedoRejected):
		code, msg = codes.FailedPrecondition, "precondition failed"
	}
	if code == codes.Internal {
		entry := logger.WithTrace(ctx, s.log)
		entry.Error().Err(err).Msg("board rpc")
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
