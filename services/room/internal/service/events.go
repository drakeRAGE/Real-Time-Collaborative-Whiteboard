package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
)

func (s *Service) publishMember(ctx context.Context, subject string, roomID string, m repo.Member) error {
	body, err := proto.Marshal(&roomv1.Member{
		UserId:   m.UserID.String(),
		Username: m.Username,
		Role:     protoRole(m.Role),
		JoinedAt: timestamppb.New(m.JoinedAt),
	})
	if err != nil {
		return fmt.Errorf("marshal member: %w", err)
	}
	return s.publish(ctx, subject, body, roomID)
}

func (s *Service) publishDeleted(ctx context.Context, roomID string) error {
	return s.publish(ctx, subjectDeleted, []byte(roomID), roomID)
}

func (s *Service) publish(ctx context.Context, subject string, data []byte, roomID string) error {
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		return fmt.Errorf("msg id: %w", err)
	}
	if err := s.pub.Publish(ctx, subject, data, id.String(), roomID); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}

func protoRole(role string) roomv1.Role {
	switch role {
	case roleAdmin:
		return roomv1.Role_ROLE_ADMIN
	case roleMember:
		return roomv1.Role_ROLE_MEMBER
	default:
		return roomv1.Role_ROLE_NONE
	}
}
