package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	gwcfg "github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/config"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/fakeroom"
)

func TestServeShutdown(t *testing.T) {
	nc := startNATS(t)
	mr := miniredis.RunT(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	roomv1.RegisterRoomServiceServer(gs, fakeroom.New())
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	httpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + httpLn.Addr().String()
	const secret = "test-secret-at-least-32-bytes-long"
	gateway, err := New(context.Background(), gwcfg.Config{
		HTTPAddr:     httpLn.Addr().String(),
		NATSURL:      nc.ConnectedUrl(),
		RedisAddr:    mr.Addr(),
		RoomGRPCAddr: lis.Addr().String(),
		FrontendURL:  origin,
		Issuer:       "http://issuer.test",
		Audience:     "authenticated",
		AllowHS256:   true,
		HMACSecret:   secret,
	}, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveErr := make(chan error, 1)
	go func() { serveErr <- gateway.Serve(ctx, httpLn) }()

	ready := get(t, origin+"/readyz")
	if ready != http.StatusOK {
		t.Fatalf("readyz %d", ready)
	}
	health := get(t, origin+"/healthz")
	if health != http.StatusOK {
		t.Fatalf("healthz %d", health)
	}

	raw := sign(t, secret, "11111111-1111-4111-8111-111111111111")
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	conn, _, err := websocket.Dial(dialCtx, "ws"+strings.TrimPrefix(origin, "http")+"/ws", &websocket.DialOptions{
		Subprotocols: []string{"bearer", raw},
	})
	dialCancel()
	if err != nil {
		t.Fatal(err)
	}
	env := &realtimev1.ClientEnvelope{
		ClientMsgId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Payload:     &realtimev1.ClientEnvelope_JoinRoom{JoinRoom: &realtimev1.JoinRoom{RoomId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}},
	}
	body, err := proto.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := conn.Write(writeCtx, websocket.MessageBinary, body); err != nil {
		writeCancel()
		t.Fatal(err)
	}
	writeCancel()
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	if _, _, err := conn.Read(readCtx); err != nil {
		readCancel()
		t.Fatal(err)
	}
	readCancel()
	cancel()
	code := readClose(t, conn)
	if code != websocket.StatusGoingAway {
		t.Fatalf("close %d", code)
	}
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return")
	}
	rec := httptestNew(t, gateway)
	if rec != http.StatusServiceUnavailable {
		t.Fatalf("readyz after shutdown %d", rec)
	}
}

func httptestNew(t *testing.T, gateway *App) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/readyz", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := &rec{header: http.Header{}}
	gateway.Handler().ServeHTTP(w, req)
	return w.code
}

type rec struct {
	header http.Header
	code   int
}

func (r *rec) Header() http.Header { return r.header }
func (r *rec) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return len(b), nil
}
func (r *rec) WriteHeader(code int) { r.code = code }

func get(t *testing.T, url string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		res, err := http.Get(url) //nolint:gosec // G107: url is the httptest listener, not user input
		if err != nil {
			last = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		return res.StatusCode
	}
	t.Fatal(last)
	return 0
}

func sign(t *testing.T, secret, sub string) string {
	t.Helper()
	tok, err := jwt.NewBuilder().Subject(sub).Issuer("http://issuer.test").Audience([]string{"authenticated"}).Expiration(time.Now().Add(time.Hour)).Claim("email", "ada@example.com").Build()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, []byte(secret)))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func startNATS(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &server.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	t.Cleanup(ns.Shutdown)
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc
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
