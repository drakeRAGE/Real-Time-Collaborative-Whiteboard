// Package httpapi is the gateway's chi router.
package httpapi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/authn"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

// Deps is everything the router needs. Ready reports dependency health.
type Deps struct {
	Log         zerolog.Logger
	Verify      authn.Verifier
	Rooms       roomv1.RoomServiceClient
	Boards      boardv1.BoardServiceClient
	Chats       chatv1.ChatServiceClient
	Ready       func(context.Context) error
	Hub         http.Handler
	FrontendURL string
}

// New returns the public handler.
func New(deps Deps) http.Handler {
	s := &api{log: deps.Log, verify: deps.Verify, rooms: deps.Rooms, boards: deps.Boards, chats: deps.Chats, ready: deps.Ready}
	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(securityHeaders)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{deps.FrontendURL},
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-Request-Id"},
		ExposedHeaders: []string{"X-Request-Id"},
		MaxAge:         300,
	}))
	r.Use(accessLog(deps.Log))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", s.readyz)
	if deps.Hub != nil {
		r.Method(http.MethodGet, "/ws", deps.Hub)
	}
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Post("/rooms", s.createRoom)
		r.Get("/rooms/{roomID}", s.getRoom)
		r.Delete("/rooms/{roomID}", s.deleteRoom)
		r.Post("/rooms/{roomID}/join", s.joinRoom)
		r.Post("/rooms/{roomID}/leave", s.leaveRoom)
		r.Get("/rooms/{roomID}/members", s.listMembers)
		r.Post("/rooms/{roomID}/invites", s.createInvite)
		r.Post("/rooms/{roomID}/transfer", s.transferAdmin)
		r.Post("/invites/validate", s.validateInvite)
		r.Get("/rooms/{roomID}/board/history", s.boardHistory)
		r.Get("/rooms/{roomID}/chat/history", s.chatHistory)
	})
	return r
}

type api struct {
	log    zerolog.Logger
	verify authn.Verifier
	rooms  roomv1.RoomServiceClient
	boards boardv1.BoardServiceClient
	chats  chatv1.ChatServiceClient
	ready  func(context.Context) error
}

func (a *api) readyz(w http.ResponseWriter, r *http.Request) {
	if a.ready == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := a.ready(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearer(r.Header.Get("Authorization"))
		if raw == "" {
			writeJSON(w, http.StatusUnauthorized, errBody("UNAUTHENTICATED", "no token"))
			return
		}
		p, err := a.verify.Verify(r.Context(), raw)
		if err != nil {
			msg := "invalid token"
			if errors.Is(err, authn.ErrExpired) {
				msg = "token expired"
			}
			writeJSON(w, http.StatusUnauthorized, errBody("UNAUTHENTICATED", msg))
			return
		}
		if _, err := uuid.Parse(p.UserID); err != nil {
			writeJSON(w, http.StatusUnauthorized, errBody("UNAUTHENTICATED", "invalid token"))
			return
		}
		user := principal.User{ID: p.UserID, Email: p.Email, Name: principal.NameFromEmail(p.Email)}
		next.ServeHTTP(w, r.WithContext(principal.With(r.Context(), user)))
	})
}

func (a *api) createRoom(w http.ResponseWriter, r *http.Request) {
	var req roomv1.CreateRoomRequest
	if !decode(w, r, &req) {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.CreateRoom(user.GRPC(r.Context()), &req)
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusCreated, resp)
}

func (a *api) getRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRoom(w, r)
	if !ok {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.GetRoom(user.GRPC(r.Context()), &roomv1.GetRoomRequest{RoomId: id})
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) deleteRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRoom(w, r)
	if !ok {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.DeleteRoom(user.GRPC(r.Context()), &roomv1.DeleteRoomRequest{RoomId: id})
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) joinRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRoom(w, r)
	if !ok {
		return
	}
	var req roomv1.JoinRoomRequest
	if !decode(w, r, &req) {
		return
	}
	req.RoomId = id
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.JoinRoom(user.GRPC(r.Context()), &req)
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) leaveRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRoom(w, r)
	if !ok {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.LeaveRoom(user.GRPC(r.Context()), &roomv1.LeaveRoomRequest{RoomId: id})
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) listMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRoom(w, r)
	if !ok {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.ListMembers(user.GRPC(r.Context()), &roomv1.ListMembersRequest{RoomId: id})
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) createInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRoom(w, r)
	if !ok {
		return
	}
	var req roomv1.CreateInviteRequest
	if !decode(w, r, &req) {
		return
	}
	req.RoomId = id
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.CreateInvite(user.GRPC(r.Context()), &req)
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusCreated, resp)
}

func (a *api) transferAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathRoom(w, r)
	if !ok {
		return
	}
	var req roomv1.TransferAdminRequest
	if !decode(w, r, &req) {
		return
	}
	req.RoomId = id
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.TransferAdmin(user.GRPC(r.Context()), &req)
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) boardHistory(w http.ResponseWriter, r *http.Request) {
	if a.boards == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("UNAVAILABLE", "board history unavailable"))
		return
	}
	roomID, ok := pathRoom(w, r)
	if !ok {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.boards.GetHistory(user.GRPC(r.Context()), &boardv1.GetHistoryRequest{
		RoomId: roomID,
		Limit:  500,
	})
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) chatHistory(w http.ResponseWriter, r *http.Request) {
	if a.chats == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("UNAVAILABLE", "chat history unavailable"))
		return
	}
	roomID, ok := pathRoom(w, r)
	if !ok {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.chats.GetHistory(user.GRPC(r.Context()), &chatv1.GetHistoryRequest{
		RoomId: roomID,
		Limit:  200,
	})
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (a *api) validateInvite(w http.ResponseWriter, r *http.Request) {
	var req roomv1.ValidateInviteRequest
	if !decode(w, r, &req) {
		return
	}
	user, _ := principal.From(r.Context())
	resp, err := a.rooms.ValidateInvite(user.GRPC(r.Context()), &req)
	if err != nil {
		writeGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func pathRoom(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "roomID")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("INVALID_ARGUMENT", "invalid room id"))
		return "", false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, msg proto.Message) bool {
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("INVALID_ARGUMENT", "invalid json"))
		return false
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := protojson.Unmarshal(raw, msg); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("INVALID_ARGUMENT", "invalid json"))
		return false
	}
	return true
}

func writeProto(w http.ResponseWriter, code int, msg proto.Message) {
	raw, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("INTERNAL", "internal error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(raw) //nolint:gosec // G705: protojson bytes, Content-Type is application/json
}

func writeGRPC(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errBody("INTERNAL", "internal error"))
		return
	}
	code := http.StatusInternalServerError
	msg := "internal error"
	switch st.Code() {
	case codes.Unauthenticated:
		code, msg = http.StatusUnauthorized, st.Message()
	case codes.PermissionDenied:
		code, msg = http.StatusForbidden, st.Message()
	case codes.NotFound:
		code, msg = http.StatusNotFound, st.Message()
	case codes.InvalidArgument:
		code, msg = http.StatusBadRequest, st.Message()
	case codes.FailedPrecondition:
		code, msg = http.StatusPreconditionFailed, st.Message()
	case codes.Unavailable:
		code, msg = http.StatusServiceUnavailable, "unavailable"
	}
	if msg == "" {
		msg = st.Code().String()
	}
	writeJSON(w, code, errBody(st.Code().String(), msg))
}

func writeJSON(w http.ResponseWriter, code int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func errBody(code, message string) map[string]string {
	return map[string]string{"code": code, "message": message}
}

func bearer(header string) string {
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

type ridKey struct{}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := cleanID(r.Header.Get("X-Request-Id"))
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ridKey{}, id)))
	})
}

func cleanID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 128 || strings.ContainsAny(s, " \r\n") {
		id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
		if err != nil {
			return "request"
		}
		return id.String()
	}
	return s
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func accessLog(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(sw, r)
			id, _ := r.Context().Value(ridKey{}).(string)
			lg := logger.WithTrace(r.Context(), log)
			lg.Info().
				Str("request_id", id).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", sw.status).
				Dur("duration", time.Since(start)).
				Msg("http")
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw := w.ResponseWriter
	for {
		if h, ok := rw.(http.Hijacker); ok {
			return h.Hijack()
		}
		u, ok := rw.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil, nil, errors.New("http: hijacker not supported")
		}
		rw = u.Unwrap()
	}
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
