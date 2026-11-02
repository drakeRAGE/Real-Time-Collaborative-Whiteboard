// Package health serves the presence-service liveness endpoint.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// Handler returns 200 when redis answers Ping.
func Handler(rdb *redis.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
