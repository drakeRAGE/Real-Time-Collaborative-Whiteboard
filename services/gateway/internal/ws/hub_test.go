package ws

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/authn"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/authz"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const (
	userA = "11111111-1111-4111-8111-111111111111"
	roomX = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

func TestDefaults(t *testing.T) {
	h := New(Options{})
	t.Cleanup(func() { h.Shutdown(context.Background()) })
	if h.authTimeout != DefaultAuthTimeout || h.pingEvery != DefaultPingInterval {
		t.Fatalf("timeouts %s %s", h.authTimeout, h.pingEvery)
	}
	if DefaultAuthTimeout != 5*time.Second || DefaultPingInterval != 30*time.Second {
		t.Fatalf("documented defaults %s %s", DefaultAuthTimeout, DefaultPingInterval)
	}
}

func TestUnauthenticatedClose(t *testing.T) {
	h, srv := newServer(t, staticAuth{token: "good", id: userA}, bindPipe{room: roomX})
	h.SetAuthTimeout(200 * time.Millisecond)
	conn := dial(t, srv, nil)
	if code := readClose(t, conn); code != 4401 {
		t.Fatalf("close %d", code)
	}
}

func TestProtocolAndFirstMessageAuth(t *testing.T) {
	h, srv := newServer(t, staticAuth{token: "good", id: userA}, bindPipe{room: roomX})
	h.SetAuthTimeout(time.Second)

	protoConn := dial(t, srv, []string{"bearer", "good"})
	id := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	writeEnv(t, protoConn, &realtimev1.ClientEnvelope{
		ClientMsgId: id,
		Payload:     &realtimev1.ClientEnvelope_JoinRoom{JoinRoom: &realtimev1.JoinRoom{RoomId: roomX}},
	})
	ack := readEnv(t, protoConn)
	if ack.GetAck().GetClientMsgId() != id || ack.GetAck().GetError() != nil {
		t.Fatalf("protocol ack %+v", ack.GetAck())
	}

	first := dial(t, srv, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := first.Write(ctx, websocket.MessageText, []byte("good")); err != nil {
		t.Fatal(err)
	}
	writeEnv(t, first, &realtimev1.ClientEnvelope{
		ClientMsgId: id,
		Payload:     &realtimev1.ClientEnvelope_JoinRoom{JoinRoom: &realtimev1.JoinRoom{RoomId: roomX}},
	})
	ack = readEnv(t, first)
	if ack.GetAck().GetClientMsgId() != id {
		t.Fatalf("first-message ack %+v", ack)
	}

	bad := dial(t, srv, []string{"bearer", "nope"})
	if code := readClose(t, bad); code != 4401 {
		t.Fatalf("bad protocol %d", code)
	}
}

func TestMaxThreeConnections(t *testing.T) {
	_, srv := newServer(t, staticAuth{token: "good", id: userA}, bindPipe{room: roomX})
	var conns []*websocket.Conn
	for i := 0; i < 3; i++ {
		c := dial(t, srv, []string{"bearer", "good"})
		writeEnv(t, c, pingEnv())
		if readEnv(t, c).GetAck() == nil {
			t.Fatal("expected ack")
		}
		conns = append(conns, c)
	}
	closed := make(chan websocket.StatusCode, 1)
	go func() { closed <- readClose(t, conns[0]) }()
	fourth := dial(t, srv, []string{"bearer", "good"})
	select {
	case code := <-closed:
		if code != websocket.StatusPolicyViolation {
			t.Fatalf("oldest close %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("oldest connection stayed open")
	}
	writeEnv(t, fourth, pingEnv())
	if readEnv(t, fourth).GetAck() == nil {
		t.Fatal("fourth connection was not accepted")
	}
}

func TestSlowConsumer(t *testing.T) {
	h, srv := newServer(t, staticAuth{token: "good", id: userA}, bindPipe{room: roomX})
	fast := dial(t, srv, []string{"bearer", "good"})
	slow := dial(t, srv, []string{"bearer", "good"})
	fast.SetReadLimit(1 << 20)
	var n atomic.Int32
	still := make(chan struct{}, 1)
	acked := make(chan struct{}, 1)
	go func() {
		for {
			env, err := readEnvErr(fast)
			if err != nil {
				return
			}
			if env.GetAck() != nil {
				select {
				case acked <- struct{}{}:
				default:
				}
			}
			if env.GetError().GetMessage() == "still-up" {
				still <- struct{}{}
				return
			}
			if env.GetError() != nil {
				n.Add(1)
			}
		}
	}()
	writeEnv(t, fast, pingEnv())
	writeEnv(t, slow, pingEnv())
	select {
	case <-acked:
	case <-time.After(3 * time.Second):
		t.Fatal("fast client did not ack")
	}
	_ = readEnv(t, slow)

	big := &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
		Message: strings.Repeat("x", 16*1024),
	}}}
	for i := 0; i < 400; i++ {
		h.Broadcast(roomX, big, "")
		if i%25 == 24 {
			waitCount(t, &n, int32(i/2))
		}
	}
	if code := readClose(t, slow); code != websocket.StatusTryAgainLater {
		t.Fatalf("slow close %d", code)
	}
	h.Broadcast(roomX, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
		Message: "still-up",
	}}}, "")
	select {
	case <-still:
	case <-time.After(3 * time.Second):
		t.Fatal("fast connection did not stay up")
	}
}

func waitCount(t *testing.T, n *atomic.Int32, min int32) {
	t.Helper()
	if min < 0 {
		min = 0
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if n.Load() >= min {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fast reader count %d want >= %d", n.Load(), min)
}

func TestShutdownClose(t *testing.T) {
	h, srv := newServer(t, staticAuth{token: "good", id: userA}, bindPipe{room: roomX})
	conn := dial(t, srv, []string{"bearer", "good"})
	writeEnv(t, conn, pingEnv())
	_ = readEnv(t, conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.Shutdown(ctx)
		close(done)
	}()
	if code := readClose(t, conn); code != websocket.StatusGoingAway {
		t.Fatalf("shutdown close %d", code)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not return")
	}
}

type staticAuth struct {
	token string
	id    string
}

func (a staticAuth) Verify(_ context.Context, raw string) (authn.Principal, error) {
	if raw != a.token {
		return authn.Principal{}, authn.ErrInvalid
	}
	return authn.Principal{UserID: a.id, Email: "ada@example.com"}, nil
}

func (staticAuth) Refresh(context.Context) error { return nil }

type bindPipe struct{ room string }

func (p bindPipe) Handle(_ context.Context, _ string, _ principal.User, env *realtimev1.ClientEnvelope, socks authz.Sockets) *realtimev1.ServerEnvelope {
	if socks != nil && p.room != "" {
		socks.Bind(p.room)
	}
	return &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Ack{Ack: &realtimev1.Ack{ClientMsgId: env.GetClientMsgId()}}}
}

func (bindPipe) Disconnect(context.Context, principal.User, string, authz.Sockets) error { return nil }

func newServer(t *testing.T, auth authn.Verifier, pipe Pipe) (*Hub, *httptest.Server) {
	t.Helper()
	h := New(Options{Auth: auth, Pipe: pipe, AuthTimeout: 200 * time.Millisecond, PingEvery: time.Hour})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		h.Shutdown(ctx)
	})
	return h, srv
}

func dial(t *testing.T, srv *httptest.Server, protocols []string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{
		Subprotocols: protocols,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	return conn
}

func pingEnv() *realtimev1.ClientEnvelope {
	return &realtimev1.ClientEnvelope{
		ClientMsgId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Payload:     &realtimev1.ClientEnvelope_JoinRoom{JoinRoom: &realtimev1.JoinRoom{RoomId: roomX}},
	}
}

func writeEnv(t *testing.T, conn *websocket.Conn, env *realtimev1.ClientEnvelope) {
	t.Helper()
	body, err := proto.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageBinary, body); err != nil {
		t.Fatal(err)
	}
}

func readEnv(t *testing.T, conn *websocket.Conn) *realtimev1.ServerEnvelope {
	t.Helper()
	env, err := readEnvErr(conn)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func readEnvErr(conn *websocket.Conn) (*realtimev1.ServerEnvelope, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, body, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	env := &realtimev1.ServerEnvelope{}
	if err := proto.Unmarshal(body, env); err != nil {
		return nil, err
	}
	return env, nil
}

func readClose(t *testing.T, conn *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, _, err := conn.Read(ctx)
		if err == nil {
			continue
		}
		var ce websocket.CloseError
		if errors.As(err, &ce) {
			return ce.Code
		}
		t.Fatal(err)
	}
}
