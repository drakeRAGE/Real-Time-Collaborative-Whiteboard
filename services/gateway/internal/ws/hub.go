// Package ws is the gateway WebSocket hub.
//
// Each connection owns three goroutines: a read pump, a write pump, and a
// ping loop. Shutdown cancels them and waits for the pumps to finish.
package ws

import (
	"context"
	"crypto/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/authn"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/authz"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

const (
	maxConnsPerUser     = 3
	sendCap             = 256
	cursorCap           = 1
	maxFrame            = 64 * 1024
	DefaultAuthTimeout  = 5 * time.Second
	DefaultPingInterval = 30 * time.Second
	statusUnauth        = websocket.StatusCode(4401)
)

// Pipe handles one authenticated client event. *authz.Pipeline implements it.
type Pipe interface {
	Handle(ctx context.Context, connID string, user principal.User, env *realtimev1.ClientEnvelope, socks authz.Sockets) *realtimev1.ServerEnvelope
	Disconnect(ctx context.Context, user principal.User, roomID string, socks authz.Sockets) error
}

// Hub tracks sockets by room and by user.
type Hub struct {
	log         zerolog.Logger
	auth        authn.Verifier
	pipe        Pipe
	authTimeout time.Duration
	pingEvery   time.Duration
	origin      atomic.Value

	root       context.Context
	rootCancel context.CancelFunc
	closing    atomic.Bool

	mu     sync.Mutex
	byRoom map[string]map[*conn]struct{}
	byUser map[string][]*conn
	all    map[*conn]struct{}
}

// Options configures the hub. Zero timeouts use the documented defaults.
type Options struct {
	Log         zerolog.Logger
	Auth        authn.Verifier
	Pipe        Pipe
	Origin      string
	AuthTimeout time.Duration
	PingEvery   time.Duration
}

// New returns a hub. The origin is the only browser Origin accepted on /ws.
func New(opts Options) *Hub {
	if opts.AuthTimeout <= 0 {
		opts.AuthTimeout = DefaultAuthTimeout
	}
	if opts.PingEvery <= 0 {
		opts.PingEvery = DefaultPingInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &Hub{
		log:         opts.Log,
		auth:        opts.Auth,
		pipe:        opts.Pipe,
		authTimeout: opts.AuthTimeout,
		pingEvery:   opts.PingEvery,
		root:        ctx,
		rootCancel:  cancel,
		byRoom:      map[string]map[*conn]struct{}{},
		byUser:      map[string][]*conn{},
		all:         map[*conn]struct{}{},
	}
	h.origin.Store(opts.Origin)
	return h
}

// SetAuthTimeout overrides the first-message authentication wait.
func (h *Hub) SetAuthTimeout(d time.Duration) {
	if d > 0 {
		h.authTimeout = d
	}
}

// SetOrigin sets the single allowed browser Origin.
func (h *Hub) SetOrigin(origin string) {
	h.origin.Store(origin)
}

// ServeHTTP upgrades GET /ws.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.closing.Load() {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:    []string{"bearer"},
		OriginPatterns:  h.originPatterns(),
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		h.log.Debug().Err(err).Msg("ws accept")
		return
	}
	ws.SetReadLimit(maxFrame)
	user, ok := h.authenticate(r, ws)
	if !ok {
		return
	}
	h.serve(user, ws)
}

func (h *Hub) originPatterns() []string {
	origin, _ := h.origin.Load().(string)
	if origin == "" {
		return nil
	}
	return []string{origin}
}

func (h *Hub) authenticate(r *http.Request, ws *websocket.Conn) (principal.User, bool) {
	if tok := protocolToken(r.Header.Get("Sec-WebSocket-Protocol")); tok != "" {
		user, err := h.userFromToken(r.Context(), tok)
		if err != nil {
			h.deny(ws)
			return principal.User{}, false
		}
		return user, true
	}
	type got struct {
		body []byte
		err  error
	}
	readCtx := context.WithoutCancel(r.Context())
	ch := make(chan got, 1)
	go func() {
		_, body, err := ws.Read(readCtx)
		ch <- got{body: body, err: err}
	}()
	timer := time.NewTimer(h.authTimeout)
	defer timer.Stop()
	select {
	case <-h.root.Done():
		h.deny(ws)
		return principal.User{}, false
	case <-timer.C:
		h.deny(ws)
		return principal.User{}, false
	case res := <-ch:
		if res.err != nil {
			h.deny(ws)
			return principal.User{}, false
		}
		user, err := h.userFromToken(context.Background(), strings.TrimSpace(string(res.body)))
		if err != nil {
			h.deny(ws)
			return principal.User{}, false
		}
		return user, true
	}
}

func (h *Hub) userFromToken(ctx context.Context, raw string) (principal.User, error) {
	p, err := h.auth.Verify(ctx, raw)
	if err != nil {
		return principal.User{}, err
	}
	if _, err := uuid.Parse(p.UserID); err != nil {
		return principal.User{}, authn.ErrInvalid
	}
	return principal.User{ID: p.UserID, Email: p.Email, Name: principal.NameFromEmail(p.Email)}, nil
}

func (h *Hub) deny(ws *websocket.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	body, err := proto.Marshal(&realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
		Code:    realtimev1.ErrorCode_ERROR_CODE_UNAUTHENTICATED,
		Message: "unauthenticated",
	}}})
	if err == nil {
		_ = ws.Write(ctx, websocket.MessageBinary, body)
	}
	_ = ws.Close(statusUnauth, "unauthenticated")
}

func protocolToken(header string) string {
	seen := false
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.EqualFold(part, "bearer") {
			seen = true
			continue
		}
		if seen && part != "" {
			return part
		}
	}
	return ""
}

func (h *Hub) serve(user principal.User, ws *websocket.Conn) {
	ctx, cancel := context.WithCancel(h.root)
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		id = ulid.ULID{}
	}
	c := &conn{
		hub:    h,
		ws:     ws,
		user:   user,
		id:     id.String(),
		ctx:    ctx,
		cancel: cancel,
		send:   make(chan []byte, sendCap),
		cursor: make(chan []byte, cursorCap),
		rooms:  map[string]struct{}{},
	}
	c.socks = &adapter{h: h, c: c}
	for _, old := range h.add(c) {
		old.close(websocket.StatusPolicyViolation, "connection limit")
	}
	c.wg.Add(3)
	go func() { defer c.wg.Done(); c.readPump() }()
	go func() { defer c.wg.Done(); c.writePump() }()
	go func() { defer c.wg.Done(); c.pingLoop() }()
	c.wg.Wait()
}

// Broadcast sends env to every socket in the room, skipping excludeUserID when set.
func (h *Hub) Broadcast(roomID string, env *realtimev1.ServerEnvelope, excludeUserID string) {
	body, err := proto.Marshal(env)
	if err != nil {
		h.log.Error().Err(err).Msg("marshal broadcast")
		return
	}
	h.mu.Lock()
	set := h.byRoom[roomID]
	conns := make([]*conn, 0, len(set))
	for c := range set {
		if excludeUserID != "" && c.user.ID == excludeUserID {
			continue
		}
		conns = append(conns, c)
	}
	h.mu.Unlock()
	// Cursors are replaced, not queued. A 20/s cursor stream otherwise sits
	// in front of stroke frames on the single send channel (smoke p99).
	cursor := env.GetCursorMoved() != nil
	for _, c := range conns {
		if cursor {
			c.enqueueCursor(body)
			continue
		}
		c.enqueue(body)
	}
}

// SendUser sends env to every socket of userID.
func (h *Hub) SendUser(userID string, env *realtimev1.ServerEnvelope) {
	body, err := proto.Marshal(env)
	if err != nil {
		return
	}
	h.mu.Lock()
	list := append([]*conn(nil), h.byUser[userID]...)
	h.mu.Unlock()
	for _, c := range list {
		c.enqueue(body)
	}
}

// AnyUser returns one connected member of the room.
func (h *Hub) AnyUser(roomID string) (principal.User, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.byRoom[roomID] {
		return c.user, true
	}
	return principal.User{}, false
}

// CloseRoom closes every socket currently joined to roomID.
func (h *Hub) CloseRoom(roomID string) {
	h.mu.Lock()
	set := h.byRoom[roomID]
	conns := make([]*conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		c.close(websocket.StatusGoingAway, "room deleted")
	}
}

// Shutdown marks the hub closed, sends close 1001, and waits for pumps.
func (h *Hub) Shutdown(ctx context.Context) {
	h.closing.Store(true)
	conns := h.snapshot()
	var wg sync.WaitGroup
	for _, c := range conns {
		wg.Add(1)
		go func(c *conn) {
			defer wg.Done()
			c.close(websocket.StatusGoingAway, "shutdown")
			c.wg.Wait()
		}(c)
	}
	h.rootCancel()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (h *Hub) snapshot() []*conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*conn, 0, len(h.all))
	for c := range h.all {
		out = append(out, c)
	}
	return out
}

func (h *Hub) add(c *conn) []*conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.byUser[c.user.ID]
	var dropped []*conn
	for len(list) >= maxConnsPerUser {
		dropped = append(dropped, list[0])
		list = list[1:]
	}
	h.byUser[c.user.ID] = append(list, c)
	h.all[c] = struct{}{}
	return dropped
}

func (h *Hub) bind(c *conn, roomID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c.rooms[roomID] = struct{}{}
	if h.byRoom[roomID] == nil {
		h.byRoom[roomID] = map[*conn]struct{}{}
	}
	h.byRoom[roomID][c] = struct{}{}
}

func (h *Hub) unbind(c *conn, roomID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := c.rooms[roomID]; !ok {
		return false
	}
	others := 0
	for _, peer := range h.byUser[c.user.ID] {
		if peer != c {
			if _, in := peer.rooms[roomID]; in {
				others++
			}
		}
	}
	delete(c.rooms, roomID)
	if set := h.byRoom[roomID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.byRoom, roomID)
		}
	}
	return others == 0
}

func (h *Hub) detachAll(c *conn) []string {
	h.mu.Lock()
	rooms := make([]string, 0, len(c.rooms))
	for id := range c.rooms {
		rooms = append(rooms, id)
	}
	h.mu.Unlock()
	var last []string
	for _, id := range rooms {
		if h.unbind(c, id) {
			last = append(last, id)
		}
	}
	h.mu.Lock()
	h.removeUser(c)
	delete(h.all, c)
	h.mu.Unlock()
	return last
}

func (h *Hub) removeUser(c *conn) {
	list := h.byUser[c.user.ID]
	kept := list[:0]
	for _, peer := range list {
		if peer != c {
			kept = append(kept, peer)
		}
	}
	if len(kept) == 0 {
		delete(h.byUser, c.user.ID)
		return
	}
	h.byUser[c.user.ID] = kept
}

func (h *Hub) deliver(c *conn, env *realtimev1.ServerEnvelope) {
	if c == nil || env == nil {
		return
	}
	body, err := proto.Marshal(env)
	if err != nil {
		h.log.Error().Err(err).Msg("marshal envelope")
		return
	}
	c.enqueue(body)
}

type conn struct {
	hub    *Hub
	ws     *websocket.Conn
	user   principal.User
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	send   chan []byte
	cursor chan []byte
	rooms  map[string]struct{}
	socks  *adapter
	once   sync.Once
	wg     sync.WaitGroup
	slow   atomic.Bool
}

func (c *conn) enqueue(body []byte) {
	select {
	case <-c.ctx.Done():
		return
	case c.send <- body:
	default:
		c.hub.log.Warn().Str("user_id", c.user.ID).Msg("slow consumer")
		c.slow.Store(true)
	}
}

func (c *conn) enqueueCursor(body []byte) {
	select {
	case <-c.ctx.Done():
		return
	case c.cursor <- body:
		return
	default:
	}
	select {
	case <-c.cursor:
	default:
	}
	select {
	case <-c.ctx.Done():
	case c.cursor <- body:
	default:
	}
}

func (c *conn) close(code websocket.StatusCode, reason string) {
	c.once.Do(func() {
		last := c.hub.detachAll(c)
		// Close before cancel. Canceling a read or write context drops TCP
		// and the peer would see EOF instead of this status. Close waits
		// until the write pump finishes its current frame, which a slow
		// reader unblocks by reading.
		_ = c.ws.Close(code, reason)
		c.cancel()
		if c.hub.pipe == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, roomID := range last {
			if err := c.hub.pipe.Disconnect(ctx, c.user, roomID, c.socks); err != nil {
				c.hub.log.Warn().Err(err).Str("room_id", roomID).Msg("disconnect")
			}
		}
	})
}

func (c *conn) readPump() {
	defer c.close(websocket.StatusNormalClosure, "")
	for {
		_, body, err := c.ws.Read(context.Background())
		if err != nil {
			return
		}
		env, err := authz.Decode(body)
		if err != nil || c.hub.pipe == nil {
			c.hub.deliver(c, &realtimev1.ServerEnvelope{Payload: &realtimev1.ServerEnvelope_Error{Error: &realtimev1.Error{
				Code:    realtimev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT,
				Message: "invalid envelope",
			}}})
			continue
		}
		c.hub.deliver(c, c.hub.pipe.Handle(c.ctx, c.id, c.user, env, c.socks))
	}
}

func (c *conn) writePump() {
	for {
		// Stroke, ack, and room frames on send are written before a cursor.
		select {
		case <-c.ctx.Done():
			return
		case body := <-c.send:
			if !c.writeBody(body) {
				return
			}
			continue
		default:
		}
		select {
		case <-c.ctx.Done():
			return
		case body := <-c.send:
			if !c.writeBody(body) {
				return
			}
		case body := <-c.cursor:
			if !c.writeBody(body) {
				return
			}
		}
	}
}

func (c *conn) writeBody(body []byte) bool {
	if err := c.ws.Write(context.Background(), websocket.MessageBinary, body); err != nil {
		c.close(websocket.StatusGoingAway, "write")
		return false
	}
	if c.slow.Load() {
		c.close(websocket.StatusTryAgainLater, "slow consumer")
		return false
	}
	return true
}

func (c *conn) pingLoop() {
	ticker := time.NewTicker(c.hub.pingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.ws.Ping(ctx)
			cancel()
			if err != nil {
				c.close(websocket.StatusGoingAway, "ping")
				return
			}
		}
	}
}

type adapter struct {
	h *Hub
	c *conn
}

func (a *adapter) Bind(roomID string) { a.h.bind(a.c, roomID) }

func (a *adapter) Unbind(roomID string) {
	if !a.h.unbind(a.c, roomID) || a.h.pipe == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.h.pipe.Disconnect(ctx, a.c.user, roomID, a); err != nil {
		a.h.log.Warn().Err(err).Str("room_id", roomID).Msg("leave")
	}
}

func (a *adapter) Send(env *realtimev1.ServerEnvelope) { a.h.deliver(a.c, env) }

func (a *adapter) Broadcast(roomID string, env *realtimev1.ServerEnvelope, excludeUserID string) {
	a.h.Broadcast(roomID, env, excludeUserID)
}

func (a *adapter) SendUser(userID string, env *realtimev1.ServerEnvelope) {
	a.h.SendUser(userID, env)
}
