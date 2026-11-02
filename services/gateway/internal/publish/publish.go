// Package publish sends gateway ingress onto NATS.
//
// pkg/natsx.Publish cannot attach principal headers, and presence is core NATS
// rather than JetStream. This publisher sets the same Nats-Msg-Id and trace
// headers as natsx, plus Wb-User-Id and Wb-Username.
package publish

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"

	"github.com/drakeRAGE/realtime-whiteboard/services/gateway/internal/principal"
)

const (
	headerMsgID    = "Nats-Msg-Id"
	headerUserID   = "Wb-User-Id"
	headerUsername = "Wb-Username"
)

// Bus publishes one protobuf message.
type Bus struct {
	nc *nats.Conn
	js jetstream.JetStream
}

// New binds nc. The caller owns the connection.
func New(nc *nats.Conn) (*Bus, error) {
	if nc == nil {
		return nil, errors.New("publish: nil conn")
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("publish: jetstream: %w", err)
	}
	return &Bus{nc: nc, js: js}, nil
}

// Publish writes msg. core selects core NATS (presence); otherwise JetStream.
func (b *Bus) Publish(ctx context.Context, subject, msgID string, msg proto.Message, user principal.User, core bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil || b.nc == nil {
		return errors.New("publish: nil conn")
	}
	if subject == "" || msgID == "" || msg == nil {
		return errors.New("publish: subject, msg id, and message are required")
	}
	if user.ID == "" {
		return errors.New("publish: principal required")
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("publish: marshal: %w", err)
	}
	out := &nats.Msg{
		Subject: subject,
		Header:  headers(ctx, msgID, user),
		Data:    data,
	}
	if core {
		if err := b.nc.PublishMsg(out); err != nil {
			return fmt.Errorf("publish %s: %w", subject, err)
		}
		flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := b.nc.FlushWithContext(flushCtx); err != nil {
			return fmt.Errorf("publish flush %s: %w", subject, err)
		}
		return nil
	}
	if _, err := b.js.PublishMsg(ctx, out); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}

func headers(ctx context.Context, msgID string, user principal.User) nats.Header {
	hdr := nats.Header{}
	hdr.Set(headerMsgID, msgID)
	hdr.Set(headerUserID, user.ID)
	hdr.Set(headerUsername, user.Name)
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(hdr))
	return hdr
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
