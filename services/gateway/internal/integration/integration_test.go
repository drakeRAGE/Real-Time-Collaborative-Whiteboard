//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	natsc "github.com/testcontainers/testcontainers-go/modules/nats"
	redisc "github.com/testcontainers/testcontainers-go/modules/redis"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/app"
	gwcfg "github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/config"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/fakeroom"
)

const (
	issuer = "http://issuer.test"
	secret = "test-secret-at-least-32-bytes-long"
	userA  = "11111111-1111-4111-8111-111111111111"
	userB  = "22222222-2222-4222-8222-222222222222"
	userC  = "33333333-3333-4333-8333-333333333333"
)

func TestGatewayRealtime(t *testing.T) {
	stack := startStack(t)

	t.Run("unauth", func(t *testing.T) {
		conn := dial(t, stack.url, nil)
		if code := readCloseWithin(t, conn, 8*time.Second); code != 4401 {
			t.Fatalf("close %d", code)
		}
	})

	t.Run("stroke", func(t *testing.T) {
		roomID := createRoom(t, stack.url, userA)
		a := open(t, stack.url, userA)
		b := open(t, stack.url, userB)
		join(t, a, roomID)
		join(t, b, roomID)
		strokeID := newID(t)
		writeEnv(t, a.conn, &realtimev1.ClientEnvelope{
			ClientMsgId: newID(t),
			Payload: &realtimev1.ClientEnvelope_StrokeInput{StrokeInput: &realtimev1.StrokeInput{
				StrokeId: strokeID,
				RoomId:   roomID,
				Points:   []*boardv1.Point{{X: 1, Y: 2}},
				Color:    "#112233",
				Width:    2,
				Tool:     boardv1.Tool_TOOL_PEN,
			}},
		})
		got := b.wait(5*time.Second, func(env *realtimev1.ServerEnvelope) bool {
			return env.GetStrokeApplied().GetStroke().GetId() == strokeID
		})
		if got == nil {
			t.Fatal("peer did not receive StrokeApplied")
		}
	})

	t.Run("slow", func(t *testing.T) {
		roomID := createRoom(t, stack.url, userA)
		fast := open(t, stack.url, userA)
		slow := dial(t, stack.url, []string{"bearer", token(t, userB)})
		join(t, fast, roomID)
		writeEnv(t, slow, joinEnv(t, roomID))
		readUntil(t, slow, func(env *realtimev1.ServerEnvelope) bool {
			return env.GetAck().GetError() == nil && env.GetAck() != nil
		})
		var n atomic.Int32
		still := make(chan struct{}, 1)
		fast.wait(2*time.Second, func(env *realtimev1.ServerEnvelope) bool { return env.GetAck() != nil })
		go countSlow(fast, &n, still)
		big := &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
			Message: strings.Repeat("x", 16*1024),
		}}}
		for i := 0; i < 400; i++ {
			stack.gateway.Hub().Broadcast(roomID, big, "")
			if i%25 == 24 {
				waitCount(t, &n, int32(i/2))
			}
		}
		if code := readClose(t, slow); code != websocket.StatusTryAgainLater {
			t.Fatalf("slow close %d", code)
		}
		stack.gateway.Hub().Broadcast(roomID, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
			Message: "still-up",
		}}}, "")
		select {
		case <-still:
		case <-time.After(3 * time.Second):
			t.Fatal("fast connection closed")
		}
	})

	t.Run("signal", func(t *testing.T) {
		roomID := createRoom(t, stack.url, userA)
		a := open(t, stack.url, userA)
		b := open(t, stack.url, userB)
		c := open(t, stack.url, userC)
		join(t, a, roomID)
		join(t, b, roomID)
		signalID := newID(t)
		writeEnv(t, a.conn, &realtimev1.ClientEnvelope{
			ClientMsgId: signalID,
			Payload: &realtimev1.ClientEnvelope_CallSignal{CallSignal: &realtimev1.CallSignal{
				RoomId: roomID, TargetUserId: userB, Payload: []byte(`{"type":"offer"}`),
			}},
		})
		if b.wait(3*time.Second, func(env *realtimev1.ServerEnvelope) bool {
			return env.GetCallSignal().GetFromUserId() == userA
		}) == nil {
			t.Fatal("member did not receive signal")
		}
		if a.wait(3*time.Second, func(env *realtimev1.ServerEnvelope) bool {
			return env.GetAck().GetClientMsgId() == signalID && env.GetAck().GetError() == nil
		}) == nil {
			t.Fatal("sender did not get a success ack")
		}
		deniedID := newID(t)
		writeEnv(t, c.conn, &realtimev1.ClientEnvelope{
			ClientMsgId: deniedID,
			Payload: &realtimev1.ClientEnvelope_CallSignal{CallSignal: &realtimev1.CallSignal{
				RoomId: roomID, TargetUserId: userA, Payload: []byte(`{"type":"offer"}`),
			}},
		})
		denied := c.wait(3*time.Second, func(env *realtimev1.ServerEnvelope) bool {
			return env.GetAck().GetClientMsgId() == deniedID
		})
		if denied == nil || denied.GetAck().GetError().GetCode() != realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED {
			t.Fatalf("non-member signal %+v", denied.GetAck().GetError())
		}
	})

	t.Run("manual", func(t *testing.T) {
		roomID := createRoom(t, stack.url, userA)
		a := open(t, stack.url, userA)
		join(t, a, roomID)
		strokeID := newID(t)
		added := &boardv1.StrokeAdded{Seq: 7, Stroke: &boardv1.Stroke{
			Id: strokeID, RoomId: roomID, UserId: userA,
			Points: []*boardv1.Point{{X: 4, Y: 5}}, Color: "#abcdef", Width: 3, Tool: boardv1.Tool_TOOL_PEN,
		}}
		body, err := proto.Marshal(added)
		if err != nil {
			t.Fatal(err)
		}
		msg := &nats.Msg{Subject: "wb.board.stroke_applied.v1", Data: body, Header: nats.Header{}}
		msg.Header.Set("Nats-Msg-Id", strokeID)
		if _, err := stack.js.PublishMsg(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
		if a.wait(5*time.Second, func(env *realtimev1.ServerEnvelope) bool {
			return env.GetStrokeApplied().GetSeq() == 7
		}) == nil {
			t.Fatal("manual publish did not reach the client")
		}
	})
}

type stack struct {
	url     string
	gateway *app.App
	js      jetstream.JetStream
}

func startStack(t *testing.T) *stack {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	natsURL := startNATS(t, ctx)
	redisAddr := startRedis(t, ctx)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	roomv1.RegisterRoomServiceServer(gs, fakeroom.New())
	boardv1.RegisterBoardServiceServer(gs, fakeBoard{})
	chatv1.RegisterChatServiceServer(gs, fakeChat{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	httpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + httpLn.Addr().String()
	gateway, err := app.New(ctx, gwcfg.Config{
		HTTPAddr:      httpLn.Addr().String(),
		NATSURL:       natsURL,
		RedisAddr:     redisAddr,
		RoomGRPCAddr:  lis.Addr().String(),
		BoardGRPCAddr: lis.Addr().String(),
		ChatGRPCAddr:  lis.Addr().String(),
		FrontendURL:   origin,
		Issuer:        issuer,
		Audience:      "authenticated",
		AllowHS256:    true,
		HMACSecret:    secret,
	}, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	serveCtx, serveCancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- gateway.Serve(serveCtx, httpLn) }()
	t.Cleanup(func() {
		serveCancel()
		select {
		case <-serveErr:
		case <-time.After(15 * time.Second):
			t.Errorf("serve did not drain")
		}
	})

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nc.Subscribe("wb.board.stroke.v1", func(msg *nats.Msg) {
		in := &realtimev1.StrokeInput{}
		if err := proto.Unmarshal(msg.Data, in); err != nil {
			return
		}
		added := &boardv1.StrokeAdded{Seq: 1, Stroke: &boardv1.Stroke{
			Id: in.GetStrokeId(), RoomId: in.GetRoomId(), UserId: msg.Header.Get("Wb-User-Id"),
			Points: in.GetPoints(), Color: in.GetColor(), Width: in.GetWidth(), Tool: in.GetTool(),
		}}
		body, err := proto.Marshal(added)
		if err != nil {
			return
		}
		out := &nats.Msg{Subject: "wb.board.stroke_applied.v1", Data: body, Header: nats.Header{}}
		out.Header.Set("Nats-Msg-Id", in.GetStrokeId()+":applied")
		_, _ = js.PublishMsg(context.Background(), out)
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	return &stack{url: origin, gateway: gateway, js: js}
}

type sock struct {
	conn *websocket.Conn
	mu   sync.Mutex
	envs []*realtimev1.ServerEnvelope
	err  error
}

func open(t *testing.T, rawURL, user string) *sock {
	t.Helper()
	s := &sock{conn: dial(t, rawURL, []string{"bearer", token(t, user)})}
	s.conn.SetReadLimit(1 << 20)
	go s.loop()
	return s
}

func (s *sock) loop() {
	for {
		env, err := readEnv(s.conn)
		s.mu.Lock()
		if err != nil {
			s.err = err
			s.mu.Unlock()
			return
		}
		s.envs = append(s.envs, env)
		s.mu.Unlock()
	}
}

func (s *sock) wait(d time.Duration, pred func(*realtimev1.ServerEnvelope) bool) *realtimev1.ServerEnvelope {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, env := range s.envs {
			if pred(env) {
				s.mu.Unlock()
				return env
			}
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			return nil
		}
		time.Sleep(15 * time.Millisecond)
	}
	return nil
}

func countSlow(s *sock, n *atomic.Int32, still chan struct{}) {
	deadline := time.Now().Add(8 * time.Second)
	seen := 0
	for time.Now().Before(deadline) {
		s.mu.Lock()
		envs := append([]*realtimev1.ServerEnvelope(nil), s.envs...)
		s.mu.Unlock()
		for _, env := range envs[seen:] {
			seen++
			if env.GetError().GetMessage() == "still-up" {
				still <- struct{}{}
				return
			}
			if env.GetError() != nil {
				n.Add(1)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func join(t *testing.T, s *sock, roomID string) {
	t.Helper()
	id := newID(t)
	writeEnv(t, s.conn, &realtimev1.ClientEnvelope{
		ClientMsgId: id,
		Payload:     &realtimev1.ClientEnvelope_JoinRoom{JoinRoom: &realtimev1.JoinRoom{RoomId: roomID}},
	})
	got := s.wait(3*time.Second, func(env *realtimev1.ServerEnvelope) bool {
		return env.GetAck().GetClientMsgId() == id && env.GetAck().GetError() == nil
	})
	if got == nil {
		t.Fatal("join was not acked")
	}
}

func joinEnv(t *testing.T, roomID string) *realtimev1.ClientEnvelope {
	t.Helper()
	return &realtimev1.ClientEnvelope{
		ClientMsgId: newID(t),
		Payload:     &realtimev1.ClientEnvelope_JoinRoom{JoinRoom: &realtimev1.JoinRoom{RoomId: roomID}},
	}
}

func createRoom(t *testing.T, rawURL, user string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, rawURL+"/api/v1/rooms", strings.NewReader(`{"title":"demo"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token(t, user))
	res, err := http.DefaultClient.Do(req) //nolint:gosec // G704: url is the test gateway, not user input
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %s", res.StatusCode, body)
	}
	var room roomv1.CreateRoomResponse
	if err := protojson.Unmarshal(body, &room); err != nil {
		t.Fatal(err)
	}
	return room.GetRoom().GetId()
}

func dial(t *testing.T, rawURL string, protocols []string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(rawURL, "http")+"/ws", &websocket.DialOptions{Subprotocols: protocols})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	return conn
}

type fakeBoard struct {
	boardv1.UnimplementedBoardServiceServer
}

func (fakeBoard) GetHistory(context.Context, *boardv1.GetHistoryRequest) (*boardv1.GetHistoryResponse, error) {
	return &boardv1.GetHistoryResponse{}, nil
}

type fakeChat struct {
	chatv1.UnimplementedChatServiceServer
}

func (fakeChat) GetHistory(context.Context, *chatv1.GetHistoryRequest) (*chatv1.GetHistoryResponse, error) {
	return &chatv1.GetHistoryResponse{}, nil
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

func readEnv(conn *websocket.Conn) (*realtimev1.ServerEnvelope, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

func readUntil(t *testing.T, conn *websocket.Conn, pred func(*realtimev1.ServerEnvelope) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		env, err := readEnv(conn)
		if err != nil {
			t.Fatal(err)
		}
		if pred(env) {
			return
		}
	}
	t.Fatal("timed out")
}

func readClose(t *testing.T, conn *websocket.Conn) websocket.StatusCode {
	t.Helper()
	return readCloseWithin(t, conn, 3*time.Second)
}

func readCloseWithin(t *testing.T, conn *websocket.Conn, d time.Duration) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
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

func waitCount(t *testing.T, n *atomic.Int32, min int32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if n.Load() >= min {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fast reader count %d want >= %d", n.Load(), min)
}

func token(t *testing.T, sub string) string {
	t.Helper()
	tok, err := jwt.NewBuilder().Subject(sub).Issuer(issuer).Audience([]string{"authenticated"}).Expiration(time.Now().Add(time.Hour)).Claim("email", sub[:8]+"@example.com").Build()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, []byte(secret)))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func newID(t *testing.T) string {
	t.Helper()
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func startNATS(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := natsc.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ctr.Terminate(cctx)
	})
	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return url
}

func startRedis(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := redisc.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ctr.Terminate(cctx)
	})
	endpoint, err := ctr.Endpoint(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}
