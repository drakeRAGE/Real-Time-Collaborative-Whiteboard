// Package publish sends room lifecycle events on core NATS.
// wb.room.> is not a JetStream stream (see docs/CONTRACTS.md).
package publish

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
)

// Publisher delivers one core-NATS message.
type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte, msgID, roomID string) error
}

// Core publishes on nc. The caller owns the connection.
type Core struct {
	nc *nats.Conn
}

// New wraps nc.
func New(nc *nats.Conn) *Core {
	return &Core{nc: nc}
}

// Publish sets Nats-Msg-Id and Wb-Room-Id, injects the trace context, and flushes.
func (c *Core) Publish(ctx context.Context, subject string, data []byte, msgID, roomID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil || c.nc == nil {
		return errors.New("publish: nil conn")
	}
	if subject == "" || msgID == "" {
		return errors.New("publish: subject and msg id required")
	}
	hdr := nats.Header{}
	hdr.Set("Nats-Msg-Id", msgID)
	if roomID != "" {
		hdr.Set("Wb-Room-Id", roomID)
	}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(hdr))
	if err := c.nc.PublishMsg(&nats.Msg{Subject: subject, Header: hdr, Data: data}); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := c.nc.FlushWithContext(flushCtx); err != nil {
		return fmt.Errorf("flush %s: %w", subject, err)
	}
	return nil
}

type headerCarrier nats.Header

func (c headerCarrier) Get(key string) string { return nats.Header(c).Get(key) }

func (c headerCarrier) Set(key, value string) { nats.Header(c).Set(key, value) }

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range nats.Header(c) {
		keys = append(keys, k)
	}
	return keys
}
