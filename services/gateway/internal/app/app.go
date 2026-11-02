// Package app wires the gateway process.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/connectivity"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/authn"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/redisx"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/authz"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/call"
	gwcfg "github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/config"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/fanout"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/httpapi"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/ws"
	"google.golang.org/grpc"
)

// App is one gateway process.
type App struct {
	log      zerolog.Logger
	handler  http.Handler
	http     *http.Server
	hub      *ws.Hub
	fan      *fanout.Fanout
	nc       *nats.Conn
	rdb      *redis.Client
	roomConn *grpc.ClientConn
	notReady atomic.Bool
	serving  atomic.Bool
	once     sync.Once
}

// Handler is the HTTP router, including /ws.
func (a *App) Handler() http.Handler { return a.handler }

// Hub is the websocket hub.
func (a *App) Hub() *ws.Hub { return a.hub }

// New connects NATS, Redis, and room-service and builds the handler.
func New(ctx context.Context, cfg gwcfg.Config, log zerolog.Logger) (*App, error) {
	nc, err := nats.Connect(cfg.NATSURL, nats.Name("gateway"))
	if err != nil {
		return nil, fmt.Errorf("nats: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			nc.Close()
		}
	}()
	jsx, err := natsx.New(nc)
	if err != nil {
		return nil, err
	}
	if err := jsx.EnsureStreams(ctx); err != nil {
		return nil, err
	}
	rdb := redisx.New(cfg.RedisAddr)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis: %w", err)
	}
	defer func() {
		if !ok {
			_ = rdb.Close()
		}
	}()
	roomConn, err := grpcx.Dial(cfg.RoomGRPCAddr)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !ok {
			_ = roomConn.Close()
		}
	}()
	verifier, err := authn.New(ctx, authn.Config{
		JWKSURL:    cfg.JWKSURL,
		Issuer:     cfg.Issuer,
		Audience:   cfg.Audience,
		AllowHS256: cfg.AllowHS256,
		HMACSecret: cfg.HMACSecret,
	})
	if err != nil {
		return nil, err
	}
	rooms := roomv1.NewRoomServiceClient(roomConn)
	checker := authz.NewChecker(rdb, rooms, 0)
	bus, err := publish.New(nc)
	if err != nil {
		return nil, err
	}
	pipe, err := authz.New(authz.Deps{
		Members: checker,
		Rooms:   rooms,
		Bus:     bus,
		Limits:  redisx.NewRateLimiter(rdb),
		Calls:   call.New(rdb),
	})
	if err != nil {
		return nil, err
	}
	hub := ws.New(ws.Options{Log: log, Auth: verifier, Pipe: pipe, Origin: cfg.FrontendURL})
	fan := fanout.New(nc, hub, rooms, checker, log)
	if err := fan.Start(ctx); err != nil {
		hub.Shutdown(context.Background())
		return nil, err
	}
	a := &App{log: log, hub: hub, fan: fan, nc: nc, rdb: rdb, roomConn: roomConn}
	a.handler = httpapi.New(httpapi.Deps{
		Log:         log,
		Verify:      verifier,
		Rooms:       rooms,
		Ready:       a.ready,
		Hub:         hub,
		FrontendURL: cfg.FrontendURL,
	})
	a.http = &http.Server{Handler: a.handler, ReadHeaderTimeout: 5 * time.Second}
	ok = true
	return a, nil
}

// Serve listens until ctx is canceled, then drains for up to 10s.
// SIGTERM cancels ctx in main; that is the shutdown path.
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	a.serving.Store(true)
	errCh := make(chan error, 1)
	go func() {
		err := a.http.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.Shutdown(shut)
	case err := <-errCh:
		return err
	}
}

// Shutdown fails readiness, closes sockets with 1001, and releases dependencies.
func (a *App) Shutdown(ctx context.Context) error {
	var err error
	a.once.Do(func() {
		a.notReady.Store(true)
		if a.hub != nil {
			a.hub.Shutdown(ctx)
		}
		if a.serving.Load() && a.http != nil {
			if shutErr := a.http.Shutdown(ctx); shutErr != nil {
				err = shutErr
			}
		}
		if a.fan != nil {
			a.fan.Stop()
		}
		if a.roomConn != nil {
			_ = a.roomConn.Close()
		}
		if a.rdb != nil {
			_ = a.rdb.Close()
		}
		if a.nc != nil {
			a.nc.Close()
		}
	})
	return err
}

func (a *App) ready(ctx context.Context) error {
	if a.notReady.Load() {
		return errors.New("shutting down")
	}
	if a.nc == nil || !a.nc.IsConnected() {
		return errors.New("nats")
	}
	if err := a.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	a.roomConn.Connect()
	for {
		state := a.roomConn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			return errors.New("room grpc shutdown")
		}
		if !a.roomConn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("room grpc %s", a.roomConn.GetState())
		}
	}
}
