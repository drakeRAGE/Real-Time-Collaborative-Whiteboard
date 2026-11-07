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

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/config"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/otelx"
	roomcfg "github.com/drakeRAGE/realtime-whiteboard/services/room/internal/config"
	roomgrpc "github.com/drakeRAGE/realtime-whiteboard/services/room/internal/grpc"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/health"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/service"
)

func main() {
	cfg := config.MustLoad[roomcfg.Config]()
	log := logger.New("room", cfg.LogLevel)
	if err := run(cfg, log); err != nil {
		log.Error().Err(err).Msg("room stopped")
		os.Exit(1)
	}
}

func run(cfg roomcfg.Config, log zerolog.Logger) error {
	shutdown, err := otelx.Init("room")
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

	natsOpts, err := natsx.ConnectOptions("room", cfg.NATSCreds)
	if err != nil {
		return err
	}
	nc, err := nats.Connect(cfg.NATSURL, natsOpts...)
	if err != nil {
		return err
	}
	defer func() { nc.Close() }()

	svc := service.New(repo.New(pool), publish.New(nc))
	grpcSrv := grpcx.NewServer(log)
	roomv1.RegisterRoomServiceServer(grpcSrv, roomgrpc.New(svc, log))

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
		Handler:           health.Handler(pool),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if serveErr := httpSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error().Err(serveErr).Msg("http serve")
			stop()
		}
	}()

	log.Info().Str("grpc", lis.Addr().String()).Str("http", cfg.HTTPAddr).Msg("room service listening")
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
