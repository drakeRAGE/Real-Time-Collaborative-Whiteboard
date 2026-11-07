package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/drakeRAGE/realtime-whiteboard/pkg/config"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/otelx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	presencecfg "github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/config"
	"github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/health"
	"github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/service"
	"github.com/drakeRAGE/realtime-whiteboard/services/presence/internal/store"
)

const (
	cursorSubject = "wb.presence.cursor.v1"
	offlineAfter  = 15 * time.Second
	sweepEvery    = 5 * time.Second
)

func main() {
	cfg := config.MustLoad[presencecfg.Config]()
	log := logger.New("presence", cfg.LogLevel)
	if err := run(cfg, log); err != nil {
		log.Error().Err(err).Msg("presence stopped")
		os.Exit(1)
	}
}

func run(cfg presencecfg.Config, log zerolog.Logger) error {
	shutdown, err := otelx.Init("presence")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb, err := redisx.New(cfg.RedisAddr)
	if err != nil {
		return err
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	natsOpts, err := natsx.ConnectOptions("presence", cfg.NATSCreds)
	if err != nil {
		return err
	}
	nc, err := nats.Connect(cfg.NATSURL, natsOpts...)
	if err != nil {
		return err
	}
	defer nc.Close()

	st := store.New(rdb)
	svc := service.New(st, nc)

	if _, err := nc.Subscribe(cursorSubject, func(msg *nats.Msg) {
		if err := svc.HandleCursor(ctx, msg.Data, msg.Header); err != nil {
			log.Warn().Err(err).Msg("cursor")
		}
	}); err != nil {
		return err
	}

	go func() {
		ticker := time.NewTicker(sweepEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := svc.SweepOffline(ctx, offlineAfter); err != nil {
					log.Warn().Err(err).Msg("presence sweep")
				}
			}
		}
	}()

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           health.Handler(rdb),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if serveErr := httpSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error().Err(serveErr).Msg("http serve")
			stop()
		}
	}()

	log.Info().Str("http", cfg.HTTPAddr).Msg("presence service listening")
	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("http shutdown")
	}
	if err := shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("otel shutdown")
	}
	return nil
}
