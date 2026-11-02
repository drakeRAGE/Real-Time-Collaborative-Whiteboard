package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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

	bad := Handler(ping{}, ping{err: errors.New("down")})
	rec = httptest.NewRecorder()
	bad.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy %d", rec.Code)
	}
}
