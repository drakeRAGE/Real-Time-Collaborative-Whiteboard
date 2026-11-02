// Package health serves the room-service liveness endpoint.
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

// Handler returns 200 when the database answers Ping, and 503 when it does not.
func Handler(db Pinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
