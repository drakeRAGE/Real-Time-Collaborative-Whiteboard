package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

// mdUserID is the gRPC metadata key for the caller. Gateways stamp Wb-User-Id;
// gRPC lowercases metadata keys. The request body is never a principal.
const mdUserID = "wb-user-id"

func principal(ctx context.Context) (uuid.UUID, error) {
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
