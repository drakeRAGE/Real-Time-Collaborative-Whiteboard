package grpcx_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const validRoom = "550e8400-e29b-41d4-a716-446655440000"

type joinImpl struct {
	mu               sync.Mutex
	calls            int
	panic            bool
	unavailableUntil int
	cancelOnFirst    context.CancelFunc
	returnInvalid    bool
}

func (s *joinImpl) Join(_ context.Context, req *realtimev1.JoinRoom) (*realtimev1.JoinRoom, error) {
	s.mu.Lock()
	s.calls++
	n := s.calls
	s.mu.Unlock()
	if s.cancelOnFirst != nil && n == 1 {
		s.cancelOnFirst()
	}
	if s.panic {
		panic("boom")
	}
	if s.returnInvalid {
		return nil, status.Error(codes.InvalidArgument, "nope")
	}
	if n <= s.unavailableUntil {
		return nil, status.Error(codes.Unavailable, "down")
	}
	return req, nil
}

func (s *joinImpl) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestPanicAndValidation(t *testing.T) {
	impl := &joinImpl{}
	addr := serve(t, impl)
	conn := dial(t, addr)

	err := conn.Invoke(context.Background(), "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: validRoom}, &realtimev1.JoinRoom{})
	if err != nil {
		t.Fatal(err)
	}

	err = conn.Invoke(context.Background(), "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: "not-a-uuid"}, &realtimev1.JoinRoom{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument || !strings.Contains(st.Message(), "room_id") {
		t.Fatalf("validation err: %v", err)
	}
	if impl.callCount() != 1 {
		t.Fatalf("handler calls=%d", impl.callCount())
	}

	impl.panic = true
	err = conn.Invoke(context.Background(), "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: validRoom}, &realtimev1.JoinRoom{})
	st, ok = status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("panic err: %v", err)
	}
}

func TestRetryUnavailable(t *testing.T) {
	impl := &joinImpl{unavailableUntil: 2}
	conn := dial(t, serve(t, impl))
	err := conn.Invoke(context.Background(), "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: validRoom}, &realtimev1.JoinRoom{})
	if err != nil {
		t.Fatal(err)
	}
	if impl.callCount() != 3 {
		t.Fatalf("calls=%d", impl.callCount())
	}

	impl = &joinImpl{unavailableUntil: 10}
	conn = dial(t, serve(t, impl))
	err = conn.Invoke(context.Background(), "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: validRoom}, &realtimev1.JoinRoom{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("got %v", err)
	}
	if impl.callCount() != 3 {
		t.Fatalf("calls=%d", impl.callCount())
	}

	impl = &joinImpl{returnInvalid: true}
	conn = dial(t, serve(t, impl))
	err = conn.Invoke(context.Background(), "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: validRoom}, &realtimev1.JoinRoom{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
	if impl.callCount() != 1 {
		t.Fatalf("calls=%d", impl.callCount())
	}

	ctx, cancel := context.WithCancel(context.Background())
	impl = &joinImpl{unavailableUntil: 5, cancelOnFirst: cancel}
	conn = dial(t, serve(t, impl))
	err = conn.Invoke(ctx, "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: validRoom}, &realtimev1.JoinRoom{})
	if status.Code(err) != codes.Canceled && status.Code(err) != codes.Unavailable {
		// The retry loop returns ctx.Err() which is context.Canceled, not a status.
		if ctx.Err() == nil || err == nil {
			t.Fatalf("got %v", err)
		}
	}
}

func serve(t *testing.T, impl *joinImpl) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpcx.NewServer(zerolog.Nop())
	srv.RegisterService(&joinDesc, impl)
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func dial(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpcx.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	return conn
}

type joinServer interface {
	Join(context.Context, *realtimev1.JoinRoom) (*realtimev1.JoinRoom, error)
}

var joinDesc = grpc.ServiceDesc{
	ServiceName: "whiteboard.test.Join",
	HandlerType: (*joinServer)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "Join",
		Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
			in := new(realtimev1.JoinRoom)
			if err := dec(in); err != nil {
				return nil, err
			}
			if interceptor == nil {
				return srv.(joinServer).Join(ctx, in)
			}
			info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/whiteboard.test.Join/Join"}
			handler := func(ctx context.Context, req any) (any, error) {
				return srv.(joinServer).Join(ctx, req.(*realtimev1.JoinRoom))
			}
			return interceptor(ctx, in, info, handler)
		},
	}},
	Streams:  []grpc.StreamDesc{},
	Metadata: "test",
}

func TestRetryTimingBudget(t *testing.T) {
	// Three attempts with two 100ms waits should finish well under a few seconds.
	impl := &joinImpl{unavailableUntil: 2}
	conn := dial(t, serve(t, impl))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := conn.Invoke(ctx, "/whiteboard.test.Join/Join", &realtimev1.JoinRoom{RoomId: validRoom}, &realtimev1.JoinRoom{}); err != nil {
		t.Fatal(err)
	}
}
