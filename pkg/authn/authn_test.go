package authn

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const testIssuer = "https://example.supabase.co/auth/v1"

func TestNewRejectsBadConfig(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, Config{}); err == nil {
		t.Fatal("expected issuer error")
	}
	if _, err := New(ctx, Config{Issuer: testIssuer}); err == nil {
		t.Fatal("expected jwks error")
	}
	if _, err := New(ctx, Config{Issuer: testIssuer, AllowHS256: true}); err == nil {
		t.Fatal("expected hmac secret error")
	}
}

func TestDefaultRefreshInterval(t *testing.T) {
	if defaultRefreshInterval != 15*time.Minute {
		t.Fatalf("refresh interval %s", defaultRefreshInterval)
	}
}

func TestHS256LegacyAndRejection(t *testing.T) {
	const legacyKey = "legacy-hmac-key"
	good := signHS(t, legacyKey, claims{sub: "user-hs", email: "hs@example.com", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	legacy, err := New(ctx, Config{Issuer: testIssuer, AllowHS256: true, HMACSecret: legacyKey})
	if err != nil {
		t.Fatal(err)
	}
	p, err := legacy.Verify(ctx, good)
	if err != nil {
		t.Fatal(err)
	}
	if p.UserID != "user-hs" || p.Email != "hs@example.com" {
		t.Fatalf("principal %+v", p)
	}
	if err := legacy.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	otherKey := "other-hmac-key"
	wrong, err := New(ctx, Config{Issuer: testIssuer, AllowHS256: true, HMACSecret: otherKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Verify(ctx, good); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong secret: %v", err)
	}

	jwks := serveJWKS(t)
	defer jwks.srv.Close()
	priv := rsaKey(t, "rsa-1")
	setJWKS(jwks, mustPublicJWKS(t, priv))
	asymm, err := New(ctx, Config{Issuer: testIssuer, JWKSURL: jwks.url})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := asymm.Verify(ctx, good); !errors.Is(err, ErrInvalid) {
		t.Fatalf("hs256 against rs256 mode: %v", err)
	}

	var raw rsa.PrivateKey
	if err := priv.Raw(&raw); err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&raw.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	confused := signHS(t, string(pubDER), claims{sub: "attacker", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})
	if _, err := asymm.Verify(ctx, confused); !errors.Is(err, ErrInvalid) {
		t.Fatalf("alg confusion: %v", err)
	}
}

func TestJWKSVerification(t *testing.T) {
	srv := serveJWKS(t)
	defer srv.srv.Close()

	rsaPriv := rsaKey(t, "rsa-1")
	ecPriv := ecKey(t, "ec-1")
	setJWKS(srv, mustPublicJWKS(t, rsaPriv, ecPriv))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v, err := New(ctx, Config{Issuer: testIssuer, JWKSURL: srv.url})
	if err != nil {
		t.Fatal(err)
	}

	validRS := signAsym(t, rsaPriv, claims{sub: "user-1", email: "a@b.c", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})
	p, err := v.Verify(ctx, validRS)
	if err != nil {
		t.Fatal(err)
	}
	if p.UserID != "user-1" || p.Email != "a@b.c" {
		t.Fatalf("principal %+v", p)
	}

	validEC := signAsym(t, ecPriv, claims{sub: "user-ec", email: "ec@b.c", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})
	p, err = v.Verify(ctx, validEC)
	if err != nil {
		t.Fatal(err)
	}
	if p.UserID != "user-ec" || p.Email != "ec@b.c" {
		t.Fatalf("principal %+v", p)
	}

	if _, err := v.Verify(ctx, ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := v.Verify(ctx, "   "); !errors.Is(err, ErrNoToken) {
		t.Fatalf("blank: %v", err)
	}
	if _, err := v.Verify(ctx, "not-a-jwt"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("garbage: %v", err)
	}

	expired := signAsym(t, rsaPriv, claims{sub: "user-1", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(-2 * time.Hour), iat: time.Now().Add(-3 * time.Hour)})
	if _, err := v.Verify(ctx, expired); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}

	wrongIss := signAsym(t, rsaPriv, claims{sub: "user-1", iss: "https://evil.example", aud: "authenticated", exp: time.Now().Add(time.Hour)})
	if _, err := v.Verify(ctx, wrongIss); !errors.Is(err, ErrInvalid) || errors.Is(err, ErrExpired) {
		t.Fatalf("wrong issuer: %v", err)
	}

	wrongAud := signAsym(t, rsaPriv, claims{sub: "user-1", iss: testIssuer, aud: "anon", exp: time.Now().Add(time.Hour)})
	if _, err := v.Verify(ctx, wrongAud); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong audience: %v", err)
	}

	noSub := signAsym(t, rsaPriv, claims{iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})
	if _, err := v.Verify(ctx, noSub); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing sub: %v", err)
	}

	noExp := signAsym(t, rsaPriv, claims{sub: "user-3", iss: testIssuer, aud: "authenticated"})
	if _, err := v.Verify(ctx, noExp); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing exp: %v", err)
	}

	noEmail := signAsym(t, rsaPriv, claims{sub: "user-2", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})
	p, err = v.Verify(ctx, noEmail)
	if err != nil || p.Email != "" || p.UserID != "user-2" {
		t.Fatalf("principal %+v err %v", p, err)
	}

	if _, err := v.Verify(ctx, noneAlgToken()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("none alg: %v", err)
	}

	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := v.Verify(canceled, validRS); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}

func TestKeyRotation(t *testing.T) {
	srv := serveJWKS(t)
	defer srv.srv.Close()

	first := rsaKey(t, "kid-1")
	setJWKS(srv, mustPublicJWKS(t, first))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v, err := New(ctx, Config{Issuer: testIssuer, JWKSURL: srv.url})
	if err != nil {
		t.Fatal(err)
	}
	oldTok := signAsym(t, first, claims{sub: "user-1", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})
	if _, err := v.Verify(ctx, oldTok); err != nil {
		t.Fatal(err)
	}

	second := rsaKey(t, "kid-2")
	setJWKS(srv, mustPublicJWKS(t, second))
	if err := v.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	newTok := signAsym(t, second, claims{sub: "user-1", iss: testIssuer, aud: "authenticated", exp: time.Now().Add(time.Hour)})
	if _, err := v.Verify(ctx, newTok); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
	if _, err := v.Verify(ctx, oldTok); !errors.Is(err, ErrInvalid) {
		t.Fatalf("old key should fail after rotation, got %v", err)
	}
}

func TestFetchJWKSFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := New(context.Background(), Config{Issuer: testIssuer, JWKSURL: srv.URL})
	if err == nil {
		t.Fatal("expected fetch error")
	}
}

type claims struct {
	sub, email, iss, aud string
	exp, iat             time.Time
}

func signAsym(t *testing.T, key jwk.Key, c claims) string {
	t.Helper()
	tok := buildToken(t, c)
	alg := key.Algorithm()
	signed, err := jwt.Sign(tok, jwt.WithKey(alg, key))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func signHS(t *testing.T, key string, c claims) string {
	t.Helper()
	tok := buildToken(t, c)
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, []byte(key)))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func buildToken(t *testing.T, c claims) jwt.Token {
	t.Helper()
	b := jwt.NewBuilder()
	if c.sub != "" {
		b = b.Subject(c.sub)
	}
	if c.iss != "" {
		b = b.Issuer(c.iss)
	}
	if c.aud != "" {
		b = b.Audience([]string{c.aud})
	}
	if !c.exp.IsZero() {
		b = b.Expiration(c.exp)
	}
	if !c.iat.IsZero() {
		b = b.IssuedAt(c.iat)
	}
	if c.email != "" {
		b = b.Claim("email", c.email)
	}
	tok, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func noneAlgToken() string {
	return "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ4In0."
}

type jwksServer struct {
	srv *httptest.Server
	url string
	mu  sync.Mutex
	raw []byte
}

func serveJWKS(t *testing.T) *jwksServer {
	t.Helper()
	s := &jwksServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		body := append([]byte(nil), s.raw...)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	s.url = s.srv.URL
	return s
}

func setJWKS(s *jwksServer, raw []byte) {
	s.mu.Lock()
	s.raw = raw
	s.mu.Unlock()
}

func rsaKey(t *testing.T, kid string) jwk.Key {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return mustJWK(t, priv, jwa.RS256, kid)
}

func ecKey(t *testing.T, kid string) jwk.Key {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return mustJWK(t, priv, jwa.ES256, kid)
}

func mustJWK(t *testing.T, raw any, alg jwa.KeyAlgorithm, kid string) jwk.Key {
	t.Helper()
	key, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := key.Set(jwk.AlgorithmKey, alg); err != nil {
		t.Fatal(err)
	}
	if err := key.Set(jwk.KeyIDKey, kid); err != nil {
		t.Fatal(err)
	}
	return key
}

func mustPublicJWKS(t *testing.T, keys ...jwk.Key) []byte {
	t.Helper()
	set := jwk.NewSet()
	for _, key := range keys {
		pub, err := key.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := pub.Set(jwk.AlgorithmKey, key.Algorithm()); err != nil {
			t.Fatal(err)
		}
		if err := pub.Set(jwk.KeyIDKey, key.KeyID()); err != nil {
			t.Fatal(err)
		}
		if err := set.AddKey(pub); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}
