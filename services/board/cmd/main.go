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

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/config"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/otelx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	boardcache "github.com/drakeRAGE/realtime-whiteboard/services/board/internal/cache"
	boardcfg "github.com/drakeRAGE/realtime-whiteboard/services/board/internal/config"
	boardgrpc "github.com/drakeRAGE/realtime-whiteboard/services/board/internal/grpc"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/health"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/ingest"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/members"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/service"
)

func main() {
	cfg := config.MustLoad[boardcfg.Config]()
	log := logger.New("board", cfg.LogLevel)
	if err := run(cfg, log); err != nil {
		log.Error().Err(err).Msg("board stopped")
		os.Exit(1)
	}
}

func run(cfg boardcfg.Config, log zerolog.Logger) error {
	shutdown, err := otelx.Init("board")
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
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	natsOpts, err := natsx.ConnectOptions("board", cfg.NATSCreds)
	if err != nil {
		return err
	}
	nc, err := nats.Connect(cfg.NATSURL, natsOpts...)
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

	roomConn, err := grpcx.Dial(cfg.RoomGRPCAddr)
	if err != nil {
		return err
	}
	defer func() { _ = roomConn.Close() }()

	svc := service.New(repo.New(pool), members.New(roomv1.NewRoomServiceClient(roomConn)), boardcache.New(rdb), publish.New(jsx))
	go func() {
		if err := ingest.Run(ctx, nc, svc, log); err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("board ingest")
			stop()
		}
	}()

	grpcSrv := grpcx.NewServer(log)
	boardv1.RegisterBoardServiceServer(grpcSrv, boardgrpc.New(svc, log))
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
		Addr: cfg.HTTPAddr,
		Handler: health.Handler(pool, pingFunc(func(ctx context.Context) error {
			return rdb.Ping(ctx).Err()
		})),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if serveErr := httpSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error().Err(serveErr).Msg("http serve")
			stop()
		}
	}()

	log.Info().Str("grpc", lis.Addr().String()).Str("http", cfg.HTTPAddr).Msg("board service listening")
	<-ctx.Done()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	grpcSrv.GracefulStop()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("http shutdown")
	}
	if err := shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("otel shutdown")
	}
	return nil
}

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }
