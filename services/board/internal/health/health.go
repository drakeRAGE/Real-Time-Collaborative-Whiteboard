// Package health serves the board-service liveness endpoint.
package health

import (
	"context"
	"net/http"
	"time"
)

// Pinger checks a dependency. *pgxpool.Pool implements it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler returns 200 when postgres and redis answer Ping, and 503 when one does not.
func Handler(postgres, redis Pinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		for _, dep := range []Pinger{postgres, redis} {
			if dep == nil || dep.Ping(ctx) != nil {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
