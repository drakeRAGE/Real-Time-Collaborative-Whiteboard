package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

// mdUserID is the gRPC metadata key for the caller. Gateways stamp Wb-User-Id;
// gRPC lowercases metadata keys. The request body is never a principal.
const mdUserID = "wb-user-id"

var idPattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

// ValidID reports a Crockford ULID, the op id and Nats-Msg-Id.
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

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
