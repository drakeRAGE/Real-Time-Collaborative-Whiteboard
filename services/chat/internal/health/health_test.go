package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type ping struct{ err error }

func (p ping) Ping(context.Context) error { return p.err }

func TestHealthz(t *testing.T) {
	ok := Handler(ping{}, ping{})
	rec := httptest.NewRecorder()
	ok.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthy %d %q", rec.Code, rec.Body.String())
	}
	bad := Handler(ping{err: errors.New("down")}, ping{})
	rec = httptest.NewRecorder()
	bad.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy %d", rec.Code)
	}
	miss := httptest.NewRecorder()
	ok.ServeHTTP(miss, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if miss.Code == http.StatusOK {
		t.Fatal("post should not be the liveness route")
	}
}

func TestRedisPinger(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := RedisPinger(rdb).Ping(ctx); err != nil {
		t.Fatalf("redis %s: %v", addr, err)
	}
}
