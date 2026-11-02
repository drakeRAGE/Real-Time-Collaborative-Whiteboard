package authz

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"time"

	"buf.build/go/protovalidate"
	"github.com/oklog/ulid/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/call"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

var ulidPattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

// Rooms is the subset of RoomService the pipeline calls.
type Rooms interface {
	JoinRoom(ctx context.Context, in *roomv1.JoinRoomRequest, opts ...grpc.CallOption) (*roomv1.JoinRoomResponse, error)
	LeaveRoom(ctx context.Context, in *roomv1.LeaveRoomRequest, opts ...grpc.CallOption) (*roomv1.LeaveRoomResponse, error)
	GetRoom(ctx context.Context, in *roomv1.GetRoomRequest, opts ...grpc.CallOption) (*roomv1.GetRoomResponse, error)
	ListMembers(ctx context.Context, in *roomv1.ListMembersRequest, opts ...grpc.CallOption) (*roomv1.ListMembersResponse, error)
}

// Membership is the cached role lookup.
type Membership interface {
	Role(ctx context.Context, user principal.User, roomID string) (roomv1.Role, error)
	Remember(ctx context.Context, userID, roomID string, role roomv1.Role) error
	Forget(ctx context.Context, userID, roomID string) error
}

// Publisher is the only path that puts client events on NATS.
type Publisher interface {
	Publish(ctx context.Context, subject, msgID string, msg proto.Message, user principal.User, core bool) error
}

// Limiter is a token bucket. redisx.RateLimiter implements it.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Calls is the room call roster.
type Calls interface {
	Join(ctx context.Context, roomID string, p call.Participant) (call.JoinResult, error)
	Leave(ctx context.Context, roomID, userID string) (call.LeaveResult, error)
	Toggle(ctx context.Context, roomID, userID string, kind realtimev1.MediaKind, enabled bool) (realtimev1.CallToggled, error)
}

// Sockets is the connection the event arrived on.
type Sockets interface {
	Bind(roomID string)
	Unbind(roomID string)
	Send(env *realtimev1.ServerEnvelope)
	Broadcast(roomID string, env *realtimev1.ServerEnvelope, excludeUserID string)
	SendUser(userID string, env *realtimev1.ServerEnvelope)
}

// Pipeline is the ingress path for every ClientEnvelope.
// Nothing is published unless decode, protovalidate, membership, role, and
// the rate limit have all passed.
type Pipeline struct {
	rules   map[string]Rule
	members Membership
	rooms   Rooms
	bus     Publisher
	limits  Limiter
	calls   Calls
	v       protovalidate.Validator
}

// Deps wires the pipeline. A nil Rules map uses the package registry.
type Deps struct {
	Rules   map[string]Rule
	Members Membership
	Rooms   Rooms
	Bus     Publisher
	Limits  Limiter
	Calls   Calls
}

// New checks that every ClientEnvelope oneof case has a rule.
func New(deps Deps) (*Pipeline, error) {
	rules := deps.Rules
	if rules == nil {
		rules = Rules()
	}
	if err := covers(rules); err != nil {
		return nil, err
	}
	v, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("authz: validator: %w", err)
	}
	return &Pipeline{
		rules:   rules,
		members: deps.Members,
		rooms:   deps.Rooms,
		bus:     deps.Bus,
		limits:  deps.Limits,
		calls:   deps.Calls,
		v:       v,
	}, nil
}

func covers(rules map[string]Rule) error {
	md := (&realtimev1.ClientEnvelope{}).ProtoReflect().Descriptor()
	one := md.Oneofs().ByName("payload")
	if one == nil {
		return errors.New("authz: payload oneof missing")
	}
	for i := 0; i < one.Fields().Len(); i++ {
		name := string(one.Fields().Get(i).Name())
		if _, ok := rules[name]; !ok {
			return fmt.Errorf("authz: no rule for %s", name)
		}
	}
	return nil
}

// Handle runs one client event and returns the single Ack (or a connection Error
// when there is no usable client_msg_id).
func (p *Pipeline) Handle(ctx context.Context, connID string, user principal.User, env *realtimev1.ClientEnvelope, socks Sockets) *realtimev1.ServerEnvelope {
	if env == nil {
		return errorEnv(realtimev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "empty envelope")
	}
	if err := p.v.Validate(env); err != nil {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid envelope")
	}
	name := eventName(env)
	rule, ok := p.rules[name]
	if !ok || name == "" {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "unknown event")
	}
	roomID := roomOf(env)
	if rule.Role != RoleNone {
		role, err := p.members.Role(ctx, user, roomID)
		if err != nil {
			return ack(env.GetClientMsgId(), codeOf(err), messageOf(err))
		}
		if !allows(rule.Role, role) {
			return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "forbidden")
		}
	}
	if rule.Scope != ScopeOff {
		key := limitKey(name, rule, user.ID, connID)
		allowed, err := p.limits.Allow(ctx, key, rule.Limit, rule.Window)
		if err != nil {
			return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INTERNAL, "rate limit")
		}
		if !allowed {
			return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_RATE_LIMITED, "rate limited")
		}
	}
	switch rule.Kind {
	case KindJoin:
		return p.join(ctx, user, env, roomID, socks)
	case KindLeave:
		return p.leave(ctx, user, env, roomID, socks)
	case KindCallJoin:
		return p.callJoin(ctx, user, env, roomID, socks)
	case KindCallLeave:
		return p.callLeave(ctx, user, env, roomID, socks)
	case KindCallSignal:
		return p.callSignal(ctx, user, env, roomID, socks)
	case KindCallToggle:
		return p.callToggle(ctx, user, env, roomID, socks)
	case KindPublish, KindCursor:
		return p.publish(ctx, user, env, rule)
	default:
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "unknown event")
	}
}

// Disconnect publishes user_offline and drops the user from the call.
// The hub calls it only for an authenticated principal's last socket in the room.
func (p *Pipeline) Disconnect(ctx context.Context, user principal.User, roomID string, socks Sockets) error {
	if user.ID == "" || roomID == "" {
		return errors.New("disconnect: principal and room required")
	}
	res, err := p.calls.Leave(ctx, roomID, user.ID)
	if err != nil {
		return err
	}
	broadcastCall(socks, roomID, res)
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		return fmt.Errorf("disconnect id: %w", err)
	}
	msg := &presencev1.UserOffline{RoomId: roomID, UserId: user.ID}
	if err := p.bus.Publish(ctx, "wb.presence.user_offline.v1", id.String(), msg, user, true); err != nil {
		return err
	}
	return nil
}

func (p *Pipeline) join(ctx context.Context, user principal.User, env *realtimev1.ClientEnvelope, roomID string, socks Sockets) *realtimev1.ServerEnvelope {
	resp, err := p.rooms.JoinRoom(user.GRPC(ctx), &roomv1.JoinRoomRequest{
		RoomId:      roomID,
		InviteToken: env.GetJoinRoom().GetInviteToken(),
	})
	if err != nil {
		return ack(env.GetClientMsgId(), codeOf(err), messageOf(err))
	}
	_ = p.members.Remember(ctx, user.ID, roomID, resp.GetRole())
	if socks != nil {
		socks.Bind(roomID)
		if state, err := LoadRoomState(ctx, p.rooms, user, roomID); err == nil {
			socks.Send(&realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_RoomState{RoomState: state}})
		}
	}
	return ack(env.GetClientMsgId(), 0, "")
}

func (p *Pipeline) leave(ctx context.Context, user principal.User, env *realtimev1.ClientEnvelope, roomID string, socks Sockets) *realtimev1.ServerEnvelope {
	if _, err := p.rooms.LeaveRoom(user.GRPC(ctx), &roomv1.LeaveRoomRequest{RoomId: roomID}); err != nil {
		return ack(env.GetClientMsgId(), codeOf(err), messageOf(err))
	}
	_ = p.members.Forget(ctx, user.ID, roomID)
	if socks != nil {
		socks.Unbind(roomID)
	}
	return ack(env.GetClientMsgId(), 0, "")
}

func (p *Pipeline) callJoin(ctx context.Context, user principal.User, env *realtimev1.ClientEnvelope, roomID string, socks Sockets) *realtimev1.ServerEnvelope {
	res, err := p.calls.Join(ctx, roomID, call.Participant{UserID: user.ID, Name: user.Name})
	if err != nil {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INTERNAL, "call")
	}
	if socks != nil && res.State != nil {
		socks.Send(&realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_CallState{CallState: res.State}})
		if res.Joined != nil {
			socks.Broadcast(roomID, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_CallUserJoined{CallUserJoined: res.Joined}}, user.ID)
		}
	}
	return ack(env.GetClientMsgId(), 0, "")
}

func (p *Pipeline) callLeave(ctx context.Context, user principal.User, env *realtimev1.ClientEnvelope, roomID string, socks Sockets) *realtimev1.ServerEnvelope {
	res, err := p.calls.Leave(ctx, roomID, user.ID)
	if err != nil {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INTERNAL, "call")
	}
	broadcastCall(socks, roomID, res)
	return ack(env.GetClientMsgId(), 0, "")
}

func (p *Pipeline) callSignal(ctx context.Context, user principal.User, env *realtimev1.ClientEnvelope, roomID string, socks Sockets) *realtimev1.ServerEnvelope {
	target := env.GetCallSignal().GetTargetUserId()
	role, err := p.members.Role(ctx, principal.User{ID: target, Name: "user"}, roomID)
	if err != nil {
		return ack(env.GetClientMsgId(), codeOf(err), messageOf(err))
	}
	if role != roomv1.Role_ROLE_MEMBER && role != roomv1.Role_ROLE_ADMIN {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "target is not a member")
	}
	if socks != nil {
		socks.SendUser(target, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_CallSignal{CallSignal: &realtimev1.CallSignalRelayed{
			RoomId:     roomID,
			FromUserId: user.ID,
			Payload:    env.GetCallSignal().GetPayload(),
		}}})
	}
	return ack(env.GetClientMsgId(), 0, "")
}

func (p *Pipeline) callToggle(ctx context.Context, user principal.User, env *realtimev1.ClientEnvelope, roomID string, socks Sockets) *realtimev1.ServerEnvelope {
	in := env.GetCallToggle()
	tog, err := p.calls.Toggle(ctx, roomID, user.ID, in.GetMedia(), in.GetEnabled())
	if errors.Is(err, call.ErrNotInCall) {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION, "not in call")
	}
	if err != nil {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INTERNAL, "call")
	}
	if socks != nil {
		socks.Broadcast(roomID, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_CallToggled{CallToggled: &tog}}, "")
	}
	return ack(env.GetClientMsgId(), 0, "")
}

func (p *Pipeline) publish(ctx context.Context, user principal.User, env *realtimev1.ClientEnvelope, rule Rule) *realtimev1.ServerEnvelope {
	msgID := messageID(rule, env)
	if !ulidPattern.MatchString(msgID) {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "missing id")
	}
	body := payload(rule, env, user)
	if body == nil {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "empty payload")
	}
	if err := p.bus.Publish(ctx, rule.Subject, msgID, body, user, rule.Core); err != nil {
		return ack(env.GetClientMsgId(), realtimev1.ErrorCode_ERROR_CODE_INTERNAL, "publish")
	}
	return ack(env.GetClientMsgId(), 0, "")
}

func payload(rule Rule, env *realtimev1.ClientEnvelope, user principal.User) proto.Message {
	if rule.Kind == KindCursor {
		in := env.GetCursorMove()
		return &presencev1.CursorMoved{
			RoomId:   in.GetRoomId(),
			UserId:   user.ID,
			X:        in.GetX(),
			Y:        in.GetY(),
			Username: user.Name,
			Color:    Color(user.ID),
		}
	}
	ref := env.ProtoReflect()
	fd := ref.WhichOneof(ref.Descriptor().Oneofs().ByName("payload"))
	if fd == nil {
		return nil
	}
	return ref.Get(fd).Message().Interface()
}

func messageID(rule Rule, env *realtimev1.ClientEnvelope) string {
	switch rule.ID {
	case idStroke:
		return env.GetStrokeInput().GetStrokeId()
	case idShape:
		return env.GetShapeInput().GetShapeId()
	case idChat:
		return env.GetChatSend().GetMessageId()
	default:
		return env.GetClientMsgId()
	}
}

func allows(gate Role, role roomv1.Role) bool {
	switch gate {
	case RoleNone:
		return true
	case RoleMember:
		return role == roomv1.Role_ROLE_MEMBER || role == roomv1.Role_ROLE_ADMIN
	case RoleAdmin:
		return role == roomv1.Role_ROLE_ADMIN
	default:
		return false
	}
}

func broadcastCall(socks Sockets, roomID string, res call.LeaveResult) {
	if socks == nil {
		return
	}
	if res.Ended != nil {
		socks.Broadcast(roomID, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_CallEnded{CallEnded: res.Ended}}, "")
		return
	}
	if res.Left != nil {
		socks.Broadcast(roomID, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_CallUserLeft{CallUserLeft: res.Left}}, "")
	}
}

// LoadRoomState reads the authoritative member list for a room state frame.
func LoadRoomState(ctx context.Context, rooms Rooms, user principal.User, roomID string) (*realtimev1.RoomState, error) {
	ctx = user.GRPC(ctx)
	room, err := rooms.GetRoom(ctx, &roomv1.GetRoomRequest{RoomId: roomID})
	if err != nil {
		return nil, err
	}
	members, err := rooms.ListMembers(ctx, &roomv1.ListMembersRequest{RoomId: roomID})
	if err != nil {
		return nil, err
	}
	state := &realtimev1.RoomState{RoomId: roomID, AdminId: room.GetRoom().GetAdminId()}
	for _, m := range members.GetMembers() {
		name := m.GetUsername()
		if name == "" {
			name = "user"
		}
		state.Members = append(state.Members, &realtimev1.PresentMember{
			UserId:   m.GetUserId(),
			Username: name,
			Color:    Color(m.GetUserId()),
			Role:     m.GetRole(),
		})
	}
	return state, nil
}

func ack(id string, code realtimev1.ErrorCode, msg string) *realtimev1.ServerEnvelope {
	if code != 0 && !ulidPattern.MatchString(id) {
		return errorEnv(code, msg)
	}
	out := &realtimev1.Ack{ClientMsgId: id}
	if code != 0 {
		out.Error = &realtimev1.Error{Code: code, Message: clip(msg)}
	}
	return &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Ack{Ack: out}}
}

func errorEnv(code realtimev1.ErrorCode, msg string) *realtimev1.ServerEnvelope {
	return &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
		Code:    code,
		Message: clip(msg),
	}}}
}

func clip(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}

func codeOf(err error) realtimev1.ErrorCode {
	switch status.Code(err) {
	case codes.Unauthenticated:
		return realtimev1.ErrorCode_ERROR_CODE_UNAUTHENTICATED
	case codes.PermissionDenied:
		return realtimev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	case codes.NotFound:
		return realtimev1.ErrorCode_ERROR_CODE_NOT_FOUND
	case codes.InvalidArgument:
		return realtimev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT
	case codes.FailedPrecondition:
		return realtimev1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION
	default:
		return realtimev1.ErrorCode_ERROR_CODE_INTERNAL
	}
}

func messageOf(err error) string {
	if codeOf(err) == realtimev1.ErrorCode_ERROR_CODE_INTERNAL {
		return "internal error"
	}
	if st, ok := status.FromError(err); ok && st.Message() != "" {
		return st.Message()
	}
	return "request failed"
}
