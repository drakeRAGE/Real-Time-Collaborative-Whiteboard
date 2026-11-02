package authz

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/call"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

const (
	idULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	roomX  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	userX  = "11111111-1111-4111-8111-111111111111"
	userY  = "22222222-2222-4222-8222-222222222222"
)

func TestEventRoleMatrix(t *testing.T) {
	roles := []roomv1.Role{roomv1.Role_ROLE_NONE, roomv1.Role_ROLE_MEMBER, roomv1.Role_ROLE_ADMIN}
	for name := range Rules() {
		for _, role := range roles {
			t.Run(name+"/"+role.String(), func(t *testing.T) {
				b := &spyBus{}
				p := newPipe(t, role, b, allowLim{})
				env := sample(t, name)
				out := p.Handle(context.Background(), "conn-1", principal.User{ID: userX, Name: "ada"}, env, nil)
				allowed := expectAllow(name, role)
				if allowed != (ackErr(out) == 0) {
					t.Fatalf("allow=%v ack=%v", allowed, out.GetAck())
				}
				if !allowed && len(b.subjects) != 0 {
					t.Fatalf("denied event published %v", b.subjects)
				}
				rule := Rules()[name]
				if allowed && rule.Subject != "" {
					if len(b.subjects) != 1 || b.subjects[0] != rule.Subject || b.ids[0] != messageID(rule, env) {
						t.Fatalf("publish %v %v", b.subjects, b.ids)
					}
				}
				if allowed && rule.Subject == "" && len(b.subjects) != 0 {
					t.Fatalf("non-nats event published %v", b.subjects)
				}
			})
		}
	}
}

func TestDeniedPathsDoNotPublish(t *testing.T) {
	b := &spyBus{}
	p := newPipe(t, roomv1.Role_ROLE_MEMBER, b, denyLim{})
	out := p.Handle(context.Background(), "c", principal.User{ID: userX, Name: "ada"}, sample(t, "stroke_input"), nil)
	if ackErr(out) != realtimev1.ErrorCode_ERROR_CODE_RATE_LIMITED || len(b.subjects) != 0 {
		t.Fatalf("rate %+v bus %v", out.GetAck(), b.subjects)
	}

	p.rules = map[string]Rule{}
	out = p.Handle(context.Background(), "c", principal.User{ID: userX, Name: "ada"}, sample(t, "stroke_input"), nil)
	if ackErr(out) != realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED || len(b.subjects) != 0 {
		t.Fatalf("unknown %+v", out.GetAck())
	}

	b.subjects = nil
	p = newPipe(t, roomv1.Role_ROLE_MEMBER, b, allowLim{})
	bad := sample(t, "stroke_input")
	bad.GetStrokeInput().Color = "red"
	out = p.Handle(context.Background(), "c", principal.User{ID: userX, Name: "ada"}, bad, nil)
	if ackErr(out) != realtimev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT || len(b.subjects) != 0 {
		t.Fatalf("invalid %+v bus %v", out.GetAck(), b.subjects)
	}
}

func TestChatIsNotLimitedHere(t *testing.T) {
	lim := &recordLim{allow: true}
	b := &spyBus{}
	p := newPipe(t, roomv1.Role_ROLE_MEMBER, b, lim)
	out := p.Handle(context.Background(), "c", principal.User{ID: userX, Name: "ada"}, sample(t, "chat_send"), nil)
	if ackErr(out) != 0 || len(lim.keys) != 0 || len(b.subjects) != 1 || b.subjects[0] != "wb.chat.message.v1" {
		t.Fatalf("chat keys %v sub %v ack %v", lim.keys, b.subjects, out.GetAck())
	}
}

func TestCursorLimitIsPerConnection(t *testing.T) {
	lim := &recordLim{allow: true}
	b := &spyBus{}
	p := newPipe(t, roomv1.Role_ROLE_MEMBER, b, lim)
	user := principal.User{ID: userX, Name: "ada"}
	_ = p.Handle(context.Background(), "conn-a", user, sample(t, "cursor_move"), nil)
	_ = p.Handle(context.Background(), "conn-b", user, sample(t, "cursor_move"), nil)
	if len(lim.keys) != 2 || lim.keys[0] == lim.keys[1] || lim.limits[0] != 60 {
		t.Fatalf("cursor limits %+v %+v", lim.keys, lim.limits)
	}
	msg, ok := b.msgs[0].(*presencev1.CursorMoved)
	if !ok || msg.GetUserId() != userX || msg.GetColor() != Color(userX) || msg.GetUsername() != "ada" {
		t.Fatalf("cursor %+v", b.msgs[0])
	}
}

func TestSignalRequiresTargetMember(t *testing.T) {
	mem := &roleMem{by: map[string]roomv1.Role{
		userX: roomv1.Role_ROLE_MEMBER,
		userY: roomv1.Role_ROLE_NONE,
	}}
	b := &spyBus{}
	p := newPipeMem(t, mem, b, allowLim{})
	socks := &sockRec{}
	out := p.Handle(context.Background(), "c", principal.User{ID: userX, Name: "ada"}, sample(t, "call_signal"), socks)
	if ackErr(out) != realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED || len(socks.direct) != 0 || len(b.subjects) != 0 {
		t.Fatalf("signal %+v direct %d", out.GetAck(), len(socks.direct))
	}
	mem.by[userY] = roomv1.Role_ROLE_MEMBER
	out = p.Handle(context.Background(), "c", principal.User{ID: userX, Name: "ada"}, sample(t, "call_signal"), socks)
	if ackErr(out) != 0 || len(socks.direct) != 1 || socks.direct[0].GetCallSignal().GetFromUserId() != userX {
		t.Fatalf("relay %+v %+v", out.GetAck(), socks.direct)
	}
}

func TestJoinNotFoundDoesNotPublish(t *testing.T) {
	b := &spyBus{}
	p, err := New(Deps{
		Members: &roleMem{role: roomv1.Role_ROLE_NONE},
		Rooms:   &fakeRooms{err: status.Error(codes.NotFound, "not found")},
		Bus:     b,
		Limits:  allowLim{},
		Calls:   okCalls{},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := p.Handle(context.Background(), "c", principal.User{ID: userX, Name: "ada"}, sample(t, "join_room"), &sockRec{})
	if ackErr(out) != realtimev1.ErrorCode_ERROR_CODE_NOT_FOUND || len(b.subjects) != 0 {
		t.Fatalf("join %+v", out.GetAck())
	}
}

func TestDisconnectPublishesOffline(t *testing.T) {
	b := &spyBus{}
	p := newPipe(t, roomv1.Role_ROLE_MEMBER, b, allowLim{})
	user := principal.User{ID: userX, Name: "ada"}
	if err := p.Disconnect(context.Background(), user, roomX, nil); err != nil {
		t.Fatal(err)
	}
	if len(b.subjects) != 1 || b.subjects[0] != "wb.presence.user_offline.v1" || !b.core[0] {
		t.Fatalf("offline %v", b.subjects)
	}
	if err := p.Disconnect(context.Background(), principal.User{}, roomX, nil); err == nil {
		t.Fatal("expected principal error")
	}
}

func TestMatrixMatchesDocs(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	sec, err := os.ReadFile(filepath.Join(root, "docs", "SECURITY.md")) //nolint:gosec // G304: repo docs path, not user input
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := os.ReadFile(filepath.Join(root, "docs", "CONTRACTS.md")) //nolint:gosec // G304: repo docs path, not user input
	if err != nil {
		t.Fatal(err)
	}
	for name, rule := range Rules() {
		needle := "| " + name + " | " + rule.Role.String() + " | " + rule.LimitTxt + " |"
		if !strings.Contains(string(sec), needle) {
			t.Fatalf("matrix missing %s", needle)
		}
		if rule.Subject != "" && !strings.Contains(string(contracts), rule.Subject) {
			t.Fatalf("subject %s missing from CONTRACTS", rule.Subject)
		}
	}
	for _, line := range []string{"GET /healthz", "GET /readyz", "GET /ws", "DELETE /api/v1/rooms/{roomID}"} {
		if !strings.Contains(string(sec), line) {
			t.Fatalf("matrix missing %s", line)
		}
	}
}

func TestColorMatchesPresencePalette(t *testing.T) {
	got := map[string]string{
		"":                                     "#e6194b",
		"00000000-0000-4000-8000-000000000001": "#fabebe",
		"550e8400-e29b-41d4-a716-446655440000": "#f032e6",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8": "#fffac8",
		"ffffffff-ffff-ffff-ffff-ffffffffffff": "#f58231",
	}
	for id, want := range got {
		if Color(id) != want {
			t.Fatalf("Color(%s)=%s want %s", id, Color(id), want)
		}
	}
}

func TestCheckerCache(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	api := &countAPI{role: roomv1.Role_ROLE_MEMBER}
	c := NewChecker(rdb, api, 0)
	user := principal.User{ID: userX, Name: "ada"}
	ctx := context.Background()
	role, err := c.Role(ctx, user, roomX)
	if err != nil || role != roomv1.Role_ROLE_MEMBER || api.n != 1 {
		t.Fatalf("miss %s %d %v", role, api.n, err)
	}
	if _, err := c.Role(ctx, user, roomX); err != nil || api.n != 1 {
		t.Fatalf("hit calls %d %v", api.n, err)
	}
	if err := c.Forget(ctx, userX, roomX); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Role(ctx, user, roomX); err != nil || api.n != 2 {
		t.Fatalf("after forget %d %v", api.n, err)
	}
	other := principal.User{ID: userY, Name: "bea"}
	api.role = roomv1.Role_ROLE_ADMIN
	if _, err := c.Role(ctx, other, roomX); err != nil {
		t.Fatal(err)
	}
	if err := c.ForgetRoom(ctx, roomX); err != nil {
		t.Fatal(err)
	}
	if n, err := rdb.Exists(ctx, "authz:"+userX+":"+roomX, "authz:"+userY+":"+roomX).Result(); err != nil || n != 0 {
		t.Fatalf("keys left %d %v", n, err)
	}
}

func expectAllow(name string, role roomv1.Role) bool {
	switch name {
	case "join_room":
		return true
	case "clear", "undo", "redo":
		return role == roomv1.Role_ROLE_ADMIN
	default:
		return role == roomv1.Role_ROLE_MEMBER || role == roomv1.Role_ROLE_ADMIN
	}
}

func ackErr(env *realtimev1.ServerEnvelope) realtimev1.ErrorCode {
	if env.GetError() != nil {
		return env.GetError().GetCode()
	}
	if env.GetAck().GetError() != nil {
		return env.GetAck().GetError().GetCode()
	}
	return 0
}

func sample(t *testing.T, name string) *realtimev1.ClientEnvelope {
	t.Helper()
	env := &realtimev1.ClientEnvelope{ClientMsgId: idULID}
	pt := &boardv1.Point{X: 1, Y: 2}
	switch name {
	case "join_room":
		env.Payload = &realtimev1.ClientEnvelope_JoinRoom{JoinRoom: &realtimev1.JoinRoom{RoomId: roomX}}
	case "leave_room":
		env.Payload = &realtimev1.ClientEnvelope_LeaveRoom{LeaveRoom: &realtimev1.LeaveRoom{RoomId: roomX}}
	case "stroke_input":
		env.Payload = &realtimev1.ClientEnvelope_StrokeInput{StrokeInput: &realtimev1.StrokeInput{
			StrokeId: idULID, RoomId: roomX, Points: []*boardv1.Point{pt}, Color: "#112233", Width: 2, Tool: boardv1.Tool_TOOL_PEN,
		}}
	case "shape_input":
		env.Payload = &realtimev1.ClientEnvelope_ShapeInput{ShapeInput: &realtimev1.ShapeInput{
			ShapeId: idULID, RoomId: roomX, Kind: boardv1.ShapeKind_SHAPE_KIND_LINE, Start: pt, End: &boardv1.Point{X: 3, Y: 4}, Color: "#112233", Width: 2,
		}}
	case "clear":
		env.Payload = &realtimev1.ClientEnvelope_Clear{Clear: &realtimev1.ClearInput{RoomId: roomX}}
	case "undo":
		env.Payload = &realtimev1.ClientEnvelope_Undo{Undo: &realtimev1.UndoInput{RoomId: roomX}}
	case "redo":
		env.Payload = &realtimev1.ClientEnvelope_Redo{Redo: &realtimev1.RedoInput{RoomId: roomX}}
	case "chat_send":
		env.Payload = &realtimev1.ClientEnvelope_ChatSend{ChatSend: &realtimev1.ChatSend{MessageId: idULID, RoomId: roomX, Body: "hi"}}
	case "cursor_move":
		env.Payload = &realtimev1.ClientEnvelope_CursorMove{CursorMove: &realtimev1.CursorMove{RoomId: roomX, X: 1, Y: 2}}
	case "call_join":
		env.Payload = &realtimev1.ClientEnvelope_CallJoin{CallJoin: &realtimev1.CallJoin{RoomId: roomX}}
	case "call_leave":
		env.Payload = &realtimev1.ClientEnvelope_CallLeave{CallLeave: &realtimev1.CallLeave{RoomId: roomX}}
	case "call_signal":
		env.Payload = &realtimev1.ClientEnvelope_CallSignal{CallSignal: &realtimev1.CallSignal{RoomId: roomX, TargetUserId: userY, Payload: []byte{1}}}
	case "call_toggle":
		env.Payload = &realtimev1.ClientEnvelope_CallToggle{CallToggle: &realtimev1.CallToggle{RoomId: roomX, Media: realtimev1.MediaKind_MEDIA_KIND_MIC, Enabled: true}}
	default:
		t.Fatalf("no sample for %s", name)
	}
	return env
}

func newPipe(t *testing.T, role roomv1.Role, b Publisher, lim Limiter) *Pipeline {
	t.Helper()
	return newPipeMem(t, &roleMem{role: role}, b, lim)
}

func newPipeMem(t *testing.T, mem Membership, b Publisher, lim Limiter) *Pipeline {
	t.Helper()
	p, err := New(Deps{Members: mem, Rooms: &fakeRooms{}, Bus: b, Limits: lim, Calls: okCalls{}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type roleMem struct {
	role roomv1.Role
	by   map[string]roomv1.Role
}

func (m *roleMem) Role(_ context.Context, user principal.User, _ string) (roomv1.Role, error) {
	if m.by != nil {
		if role, ok := m.by[user.ID]; ok {
			return role, nil
		}
		return roomv1.Role_ROLE_NONE, nil
	}
	return m.role, nil
}

func (m *roleMem) Remember(context.Context, string, string, roomv1.Role) error { return nil }
func (m *roleMem) Forget(context.Context, string, string) error                { return nil }

type fakeRooms struct{ err error }

func (f *fakeRooms) JoinRoom(_ context.Context, in *roomv1.JoinRoomRequest, _ ...grpc.CallOption) (*roomv1.JoinRoomResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &roomv1.JoinRoomResponse{Room: &roomv1.Room{Id: in.GetRoomId(), Title: "t"}, Role: roomv1.Role_ROLE_MEMBER}, nil
}

func (f *fakeRooms) LeaveRoom(context.Context, *roomv1.LeaveRoomRequest, ...grpc.CallOption) (*roomv1.LeaveRoomResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &roomv1.LeaveRoomResponse{}, nil
}

func (f *fakeRooms) GetRoom(_ context.Context, in *roomv1.GetRoomRequest, _ ...grpc.CallOption) (*roomv1.GetRoomResponse, error) {
	return &roomv1.GetRoomResponse{Room: &roomv1.Room{Id: in.GetRoomId(), Title: "t"}}, nil
}

func (f *fakeRooms) ListMembers(context.Context, *roomv1.ListMembersRequest, ...grpc.CallOption) (*roomv1.ListMembersResponse, error) {
	return &roomv1.ListMembersResponse{}, nil
}

type spyBus struct {
	subjects []string
	ids      []string
	msgs     []proto.Message
	core     []bool
}

func (b *spyBus) Publish(_ context.Context, subject, msgID string, msg proto.Message, _ principal.User, core bool) error {
	b.subjects = append(b.subjects, subject)
	b.ids = append(b.ids, msgID)
	b.msgs = append(b.msgs, msg)
	b.core = append(b.core, core)
	return nil
}

type allowLim struct{}

func (allowLim) Allow(context.Context, string, int, time.Duration) (bool, error) { return true, nil }

type denyLim struct{}

func (denyLim) Allow(context.Context, string, int, time.Duration) (bool, error) { return false, nil }

type recordLim struct {
	allow  bool
	keys   []string
	limits []int
}

func (l *recordLim) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, error) {
	l.keys = append(l.keys, key)
	l.limits = append(l.limits, limit)
	return l.allow, nil
}

type okCalls struct{}

func (okCalls) Join(_ context.Context, roomID string, p call.Participant) (call.JoinResult, error) {
	return call.JoinResult{State: &realtimev1.CallState{RoomId: roomID}, Joined: &realtimev1.CallUserJoined{RoomId: roomID, UserId: p.UserID, Username: p.Name}}, nil
}

func (okCalls) Leave(_ context.Context, roomID, userID string) (call.LeaveResult, error) {
	return call.LeaveResult{Left: &realtimev1.CallUserLeft{RoomId: roomID, UserId: userID}}, nil
}

func (okCalls) Toggle(_ context.Context, roomID, userID string, kind realtimev1.MediaKind, enabled bool) (realtimev1.CallToggled, error) {
	return realtimev1.CallToggled{RoomId: roomID, UserId: userID, Media: kind, Enabled: enabled}, nil
}

type sockRec struct {
	direct []*realtimev1.ServerEnvelope
}

func (s *sockRec) Bind(string)                                          {}
func (s *sockRec) Unbind(string)                                        {}
func (s *sockRec) Send(*realtimev1.ServerEnvelope)                      {}
func (s *sockRec) Broadcast(string, *realtimev1.ServerEnvelope, string) {}
func (s *sockRec) SendUser(_ string, env *realtimev1.ServerEnvelope) {
	s.direct = append(s.direct, env)
}

type countAPI struct {
	role roomv1.Role
	n    int
}

func (c *countAPI) CheckMembership(_ context.Context, _ *roomv1.CheckMembershipRequest, _ ...grpc.CallOption) (*roomv1.CheckMembershipResponse, error) {
	c.n++
	return &roomv1.CheckMembershipResponse{Role: c.role}, nil
}
