package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc/metadata"
)

const (
	natsUserIDHeader   = "Wb-User-Id"
	natsUsernameHeader = "Wb-Username"
	mdUserID           = "wb-user-id"
)

func principalFromGRPC(ctx context.Context) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.Nil, ErrUnauthenticated
	}
	vals := md.Get(mdUserID)
	if len(vals) == 0 || strings.TrimSpace(vals[0]) == "" {
		return uuid.Nil, ErrUnauthenticated
	}
	id, err := uuid.Parse(vals[0])
	if err != nil {
		return uuid.Nil, ErrInvalidArgument
	}
	return id, nil
}

// PrincipalFromNATS reads the gateway-stamped principal headers.
func PrincipalFromNATS(hdr nats.Header) (userID uuid.UUID, username string, err error) {
	raw := strings.TrimSpace(hdr.Get(natsUserIDHeader))
	if raw == "" {
		return uuid.Nil, "", ErrUnauthenticated
	}
	userID, err = uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, "", ErrInvalidArgument
	}
	username = strings.TrimSpace(hdr.Get(natsUsernameHeader))
	if username == "" {
		username = "user"
	}
	return userID, username, nil
}
