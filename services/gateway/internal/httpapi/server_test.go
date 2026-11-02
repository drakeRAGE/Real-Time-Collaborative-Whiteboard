package httpapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/authn"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/fakeroom"
)

const (
	issuer = "http://issuer.test"
	secret = "test-secret-at-least-32-bytes-long"
	userA  = "11111111-1111-4111-8111-111111111111"
	userB  = "22222222-2222-4222-8222-222222222222"
	origin = "http://localhost:5173"
)

func TestHTTPMatrix(t *testing.T) {
	ctx := context.Background()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	roomv1.RegisterRoomServiceServer(gs, fakeroom.New())
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	verifier, err := authn.New(ctx, authn.Config{
		Issuer: issuer, Audience: "authenticated", AllowHS256: true, HMACSecret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	ready := true
	handler := New(Deps{
		Verify:      verifier,
		Rooms:       roomv1.NewRoomServiceClient(conn),
		FrontendURL: origin,
		Ready: func(context.Context) error {
			if !ready {
				return context.DeadlineExceeded
			}
			return nil
		},
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	health := get(t, srv.URL+"/healthz", "", "")
	if health.StatusCode != http.StatusOK {
		t.Fatalf("healthz %d", health.StatusCode)
	}
	assertHeaders(t, health)

	readyResp := get(t, srv.URL+"/readyz", "", "")
	if readyResp.StatusCode != http.StatusOK {
		t.Fatalf("readyz %d", readyResp.StatusCode)
	}
	ready = false
	down := get(t, srv.URL+"/readyz", "", "")
	if down.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz down %d", down.StatusCode)
	}
	ready = true

	noToken := get(t, srv.URL+"/api/v1/rooms/"+userA, "", "")
	if noToken.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token %d", noToken.StatusCode)
	}

	tokA := token(t, userA, "ada@example.com")
	created := do(t, http.MethodPost, srv.URL+"/api/v1/rooms", tokA, `{"title":"demo"}`, "")
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %s", created.StatusCode, created.Body)
	}
	var room roomv1.CreateRoomResponse
	if err := protojson.Unmarshal([]byte(created.Body), &room); err != nil {
		t.Fatal(err)
	}
	roomID := room.GetRoom().GetId()
	got := get(t, srv.URL+"/api/v1/rooms/"+roomID, tokA, "")
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get %d %s", got.StatusCode, got.Body)
	}

	tokB := token(t, userB, "bea@example.com")
	del := do(t, http.MethodDelete, srv.URL+"/api/v1/rooms/"+roomID, tokB, "", "")
	if del.StatusCode != http.StatusForbidden {
		t.Fatalf("delete %d %s", del.StatusCode, del.Body)
	}
	joined := do(t, http.MethodPost, srv.URL+"/api/v1/rooms/"+roomID+"/join", tokB, `{}`, "")
	if joined.StatusCode != http.StatusOK {
		t.Fatalf("join %d %s", joined.StatusCode, joined.Body)
	}
	members := get(t, srv.URL+"/api/v1/rooms/"+roomID+"/members", tokA, "")
	if members.StatusCode != http.StatusOK {
		t.Fatalf("members %d %s", members.StatusCode, members.Body)
	}
	inv := do(t, http.MethodPost, srv.URL+"/api/v1/rooms/"+roomID+"/invites", tokA, `{}`, "")
	if inv.StatusCode != http.StatusCreated {
		t.Fatalf("invite %d %s", inv.StatusCode, inv.Body)
	}
	valid := do(t, http.MethodPost, srv.URL+"/api/v1/invites/validate", tokA, `{"token":"invite-token-0123456789"}`, "")
	if valid.StatusCode != http.StatusOK {
		t.Fatalf("validate %d %s", valid.StatusCode, valid.Body)
	}
	moved := do(t, http.MethodPost, srv.URL+"/api/v1/rooms/"+roomID+"/transfer", tokA, `{"newAdminUserId":"`+userB+`"}`, "")
	if moved.StatusCode != http.StatusOK {
		t.Fatalf("transfer %d %s", moved.StatusCode, moved.Body)
	}
	left := do(t, http.MethodPost, srv.URL+"/api/v1/rooms/"+roomID+"/leave", tokA, "", "")
	if left.StatusCode != http.StatusOK {
		t.Fatalf("leave %d %s", left.StatusCode, left.Body)
	}
	badID := get(t, srv.URL+"/api/v1/rooms/not-a-uuid", tokA, "")
	if badID.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id %d", badID.StatusCode)
	}

	bad := do(t, http.MethodOptions, srv.URL+"/api/v1/rooms", "", "", "https://evil.example")
	if bad.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("evil origin allowed: %s", bad.Header.Get("Access-Control-Allow-Origin"))
	}
	good := do(t, http.MethodOptions, srv.URL+"/api/v1/rooms", "", "", origin)
	if good.Header.Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("allowed origin %q", good.Header.Get("Access-Control-Allow-Origin"))
	}
	if strings.Contains(good.Header.Get("Access-Control-Allow-Origin"), "*") {
		t.Fatal("wildcard cors")
	}
}

func assertHeaders(t *testing.T, res result) {
	t.Helper()
	h := res.Header
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("security headers %+v", h)
	}
	if h.Get("Content-Security-Policy") != "default-src 'self'" {
		t.Fatalf("csp %s", h.Get("Content-Security-Policy"))
	}
	if !strings.Contains(h.Get("Strict-Transport-Security"), "max-age=") {
		t.Fatalf("hsts %s", h.Get("Strict-Transport-Security"))
	}
	if h.Get("X-Request-Id") == "" {
		t.Fatal("missing request id")
	}
}

type result struct {
	StatusCode int
	Header     http.Header
	Body       string
}

func get(t *testing.T, url, rawToken, origin string) result {
	t.Helper()
	return do(t, http.MethodGet, url, rawToken, "", origin)
}

func do(t *testing.T, method, url, rawToken, body, origin string) result {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if rawToken != "" {
		req.Header.Set("Authorization", "Bearer "+rawToken)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return result{StatusCode: res.StatusCode, Header: res.Header, Body: string(raw)}
}

func token(t *testing.T, sub, email string) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Subject(sub).
		Issuer(issuer).
		Audience([]string{"authenticated"}).
		Expiration(time.Now().Add(time.Hour)).
		Claim("email", email).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, []byte(secret)))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}
