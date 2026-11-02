// Package principal carries the verified caller. Payloads are never a source of identity.
package principal

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"
)

// User is the authenticated caller.
type User struct {
	ID    string
	Email string
	Name  string
}

type ctxKey struct{}

// With stores u on ctx.
func With(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// From returns the user stored by With.
func From(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok && u.ID != ""
}

// GRPC stamps wb-user-id and wb-username on an outgoing gRPC call.
// Room, board, and chat read those keys and ignore ids in the payload.
func (u User) GRPC(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "wb-user-id", u.ID, "wb-username", u.Name)
}

// NameFromEmail is the display name stamped on NATS and call events.
// The local-part of the email is used. An empty email becomes "user".
func NameFromEmail(email string) string {
	name := strings.TrimSpace(email)
	if i := strings.IndexByte(name, '@'); i > 0 {
		name = strings.TrimSpace(name[:i])
	}
	if name == "" {
		return "user"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}
