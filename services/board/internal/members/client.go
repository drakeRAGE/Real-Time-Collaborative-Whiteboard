// Package members calls room-service CheckMembership.
package members

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
)

// Client is a RoomService stub. The principal is sent as gRPC metadata.
type Client struct {
	api roomv1.RoomServiceClient
}

// New returns a client for api.
func New(api roomv1.RoomServiceClient) *Client {
	return &Client{api: api}
}

// Role returns "admin", "member", or "" when the user is not a member.
func (c *Client) Role(ctx context.Context, roomID, userID uuid.UUID) (string, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "wb-user-id", userID.String(), "wb-username", "board")
	resp, err := c.api.CheckMembership(ctx, &roomv1.CheckMembershipRequest{
		RoomId: roomID.String(),
		UserId: userID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("check membership: %w", err)
	}
	switch resp.GetRole() {
	case roomv1.Role_ROLE_ADMIN:
		return "admin", nil
	case roomv1.Role_ROLE_MEMBER:
		return "member", nil
	default:
		return "", nil
	}
}
