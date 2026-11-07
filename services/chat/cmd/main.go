package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/config"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/otelx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	chatcfg "github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/config"
	chatgrpc "github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/grpc"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/health"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/ingest"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/ratelimit"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/service"
)

func main() {
	cfg := config.MustLoad[chatcfg.Config]()
	log := logger.New("chat", cfg.LogLevel)
	if err := run(cfg, log); err != nil {
		log.Error().Err(err).Msg("chat stopped")
		os.Exit(1)
	}
}

func run(cfg chatcfg.Config, log zerolog.Logger) error {
	shutdown, err := otelx.Init("chat")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		return err
	}

	rdb, err := redisx.New(cfg.RedisAddr)
	if err != nil {
		return err
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	nc, err := nats.Connect(cfg.NATSURL, nats.Name("chat"))
	if err != nil {
		return err
	}
	defer nc.Close()

	jsx, err := natsx.New(nc)
	if err != nil {
		return err
	}
	if err := jsx.EnsureStreams(ctx); err != nil {
		return err
	}

	pub := publish.New(jsx, nc)
	svc := service.New(repo.New(pool), pub, ratelimit.New(rdb))
	go func() {
		if err := ingest.Run(ctx, nc, svc); err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("chat ingest")
			stop()
		}
	}()

	grpcSrv := grpcx.NewServer(log)
	chatv1.RegisterChatServiceServer(grpcSrv, chatgrpc.New(svc, log))
	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	go func() {
		if serveErr := grpcSrv.Serve(lis); serveErr != nil {
			log.Error().Err(serveErr).Msg("grpc serve")
			stop()
		}
	}()

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           health.Handler(pool, health.RedisPinger(rdb)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if serveErr := httpSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error().Err(serveErr).Msg("http serve")
			stop()
		}
	}()

	log.Info().Str("grpc", lis.Addr().String()).Str("http", cfg.HTTPAddr).Msg("chat service listening")
	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	grpcSrv.GracefulStop()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("http shutdown")
	}
	if err := shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("otel shutdown")
	}
	return nil
}
