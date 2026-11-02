// Package health serves the chat-service liveness endpoint.
package health

import (
	"context"
	"net/http"
	"time"
)

// Pinger checks a dependency.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler returns 200 when postgres and redis answer Ping.
func Handler(postgres, redis Pinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		for _, dep := range []Pinger{postgres, redis} {
			if err := dep.Ping(ctx); err != nil {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
