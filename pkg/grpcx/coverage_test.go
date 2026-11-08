package grpcx

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestMDCarrier(t *testing.T) {
	md := metadata.Pairs("wb-user-id", "u1")
	c := mdCarrier(md)
	if c.Get("wb-user-id") != "u1" || c.Get("missing") != "" {
		t.Fatal()
	}
	c.Set("wb-username", "ada")
	if len(c.Keys()) != 2 {
		t.Fatalf("keys %v", c.Keys())
	}
}

func TestValidationMessagePlainError(t *testing.T) {
	if validationMessage(errors.New("plain")) != "plain" {
		t.Fatal()
	}
}

func TestValidateUnaryInitFailure(t *testing.T) {
	ic := validateUnary(nil, errors.New("init"))
	_, err := ic(context.Background(), &realtimev1.JoinRoom{RoomId: "550e8400-e29b-41d4-a716-446655440000"}, &grpc.UnaryServerInfo{}, func(context.Context, any) (any, error) {
		return nil, nil
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestLoggingUnary(t *testing.T) {
	ic := loggingUnary(zerolog.Nop())
	_, err := ic(context.Background(), &realtimev1.JoinRoom{}, &grpc.UnaryServerInfo{FullMethod: "/test"}, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.NotFound, "missing")
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}
