package principal

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestWithFrom(t *testing.T) {
	ctx := context.Background()
	if _, ok := From(ctx); ok {
		t.Fatal("empty ctx should miss principal")
	}
	u := User{ID: "uid-1", Email: "ada@example.com", Name: "ada"}
	ctx = With(ctx, u)
	got, ok := From(ctx)
	if !ok || got.ID != u.ID || got.Email != u.Email {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok := From(With(ctx, User{Email: "no-id"})); ok {
		t.Fatal("empty id should not be stored principal")
	}
}

func TestGRPCMetadata(t *testing.T) {
	u := User{ID: "uid-2", Name: "bob"}
	ctx := u.GRPC(context.Background())
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("missing metadata")
	}
	if md.Get("wb-user-id")[0] != u.ID || md.Get("wb-username")[0] != u.Name {
		t.Fatalf("md %v", md)
	}
}

func TestNameFromEmail(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "user"},
		{"   ", "user"},
		{"Ada.Lovelace@example.com", "Ada.Lovelace"},
		{"name@", "name"},
		{strings.Repeat("a", 120), strings.Repeat("a", 100)},
	}
	for _, tc := range cases {
		if got := NameFromEmail(tc.in); got != tc.want {
			t.Fatalf("NameFromEmail(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
