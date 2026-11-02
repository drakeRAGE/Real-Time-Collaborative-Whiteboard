package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/drakeRAGE/realtime-whiteboard/pkg/config"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/otelx"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/app"
	gwcfg "github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/config"
)

func main() {
	cfg := config.MustLoad[gwcfg.Config]()
	log := logger.New("gateway", cfg.LogLevel)
	if err := run(cfg, log); err != nil {
		log.Error().Err(err).Msg("gateway stopped")
		os.Exit(1)
	}
}

func run(cfg gwcfg.Config, log zerolog.Logger) error {
	shutdown, err := otelx.Init("gateway")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gateway, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		_ = gateway.Shutdown(context.Background())
		return err
	}
	log.Info().Str("http", ln.Addr().String()).Msg("gateway listening")
	err = gateway.Serve(ctx, ln)

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if oerr := shutdown(shutCtx); oerr != nil {
		log.Error().Err(oerr).Msg("otel shutdown")
	}
	return err
}
