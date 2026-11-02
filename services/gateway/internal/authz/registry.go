package authz

import (
	"fmt"
	"time"
)

// Role is the membership required before an event is accepted.
type Role int

const (
	// RoleNone is any authenticated principal. Join still needs a real room or invite.
	RoleNone Role = iota
	// RoleMember allows members and admins.
	RoleMember
	// RoleAdmin allows only the room admin.
	RoleAdmin
)

// String is the label used by the authz matrix.
func (r Role) String() string {
	switch r {
	case RoleNone:
		return "NONE"
	case RoleMember:
		return "MEMBER"
	case RoleAdmin:
		return "ADMIN"
	default:
		return fmt.Sprintf("role(%d)", int(r))
	}
}

// Scope chooses the rate-limit key.
type Scope int

const (
	// ScopeOff means the gateway does not limit the event.
	ScopeOff Scope = iota
	// ScopeUser limits by principal.
	ScopeUser
	// ScopeConn limits by WebSocket connection.
	ScopeConn
)

// Kind is what the pipeline does after the gates pass.
type Kind int

const (
	// KindPublish sends the client payload to JetStream.
	KindPublish Kind = iota
	// KindJoin calls RoomService.JoinRoom.
	KindJoin
	// KindLeave calls RoomService.LeaveRoom.
	KindLeave
	// KindCallJoin updates the Redis call roster.
	KindCallJoin
	// KindCallLeave updates the Redis call roster.
	KindCallLeave
	// KindCallSignal relays to one member of the same room.
	KindCallSignal
	// KindCallToggle updates mic or camera on the roster.
	KindCallToggle
	// KindCursor publishes a server-stamped cursor on core NATS.
	KindCursor
)

const (
	idClient = "client"
	idStroke = "stroke"
	idShape  = "shape"
	idChat   = "chat"
)

// Rule is one ClientEnvelope event. Events missing from the registry fail closed.
type Rule struct {
	Role     Role
	Limit    int
	Window   time.Duration
	Scope    Scope
	Subject  string
	Kind     Kind
	Core     bool
	ID       string
	LimitTxt string
}

// Rules is the ingress registry. docs/SECURITY.md mirrors it.
func Rules() map[string]Rule {
	s := time.Second
	return map[string]Rule{
		"join_room":    {Role: RoleNone, Limit: 5, Window: 10 * s, Scope: ScopeUser, Kind: KindJoin, ID: idClient, LimitTxt: "5/10s/user"},
		"leave_room":   {Role: RoleMember, Kind: KindLeave, ID: idClient, LimitTxt: "none"},
		"stroke_input": {Role: RoleMember, Limit: 20, Window: s, Scope: ScopeUser, Subject: "wb.board.stroke.v1", Kind: KindPublish, ID: idStroke, LimitTxt: "20/s/user"},
		"shape_input":  {Role: RoleMember, Limit: 20, Window: s, Scope: ScopeUser, Subject: "wb.board.shape.v1", Kind: KindPublish, ID: idShape, LimitTxt: "20/s/user"},
		"clear":        {Role: RoleAdmin, Limit: 1, Window: s, Scope: ScopeUser, Subject: "wb.board.clear.v1", Kind: KindPublish, ID: idClient, LimitTxt: "1/s/user"},
		"undo":         {Role: RoleAdmin, Limit: 5, Window: s, Scope: ScopeUser, Subject: "wb.board.undo.v1", Kind: KindPublish, ID: idClient, LimitTxt: "5/s/user"},
		"redo":         {Role: RoleAdmin, Limit: 5, Window: s, Scope: ScopeUser, Subject: "wb.board.redo.v1", Kind: KindPublish, ID: idClient, LimitTxt: "5/s/user"},
		"chat_send":    {Role: RoleMember, Subject: "wb.chat.message.v1", Kind: KindPublish, ID: idChat, LimitTxt: "delegated to chat-service"},
		"cursor_move":  {Role: RoleMember, Limit: 60, Window: s, Scope: ScopeConn, Subject: "wb.presence.cursor.v1", Kind: KindCursor, Core: true, ID: idClient, LimitTxt: "60/s/conn"},
		"call_join":    {Role: RoleMember, Limit: 2, Window: s, Scope: ScopeUser, Kind: KindCallJoin, ID: idClient, LimitTxt: "2/s/user"},
		"call_leave":   {Role: RoleMember, Limit: 2, Window: s, Scope: ScopeUser, Kind: KindCallLeave, ID: idClient, LimitTxt: "2/s/user"},
		"call_signal":  {Role: RoleMember, Limit: 30, Window: s, Scope: ScopeUser, Kind: KindCallSignal, ID: idClient, LimitTxt: "30/s/user"},
		"call_toggle":  {Role: RoleMember, Limit: 5, Window: s, Scope: ScopeUser, Kind: KindCallToggle, ID: idClient, LimitTxt: "5/s/user"},
	}
}

func limitKey(event string, rule Rule, userID, connID string) string {
	switch rule.Scope {
	case ScopeConn:
		return "rl:" + event + ":" + connID
	case ScopeUser:
		return "rl:" + event + ":" + userID
	default:
		return ""
	}
}
