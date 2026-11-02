package publish

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"

	chatv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/chat/v1"
	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
)

const (
	subjectMessageSent = "wb.chat.message_sent.v1"
	subjectRejected    = "wb.chat.rejected.v1"
	reasonHeader       = "Wb-Reason"
	msgIDHeader        = "Nats-Msg-Id"
)

// NATS publishes chat fanout events through JetStream.
type NATS struct {
	js *natsx.Client
	nc *nats.Conn
}

// New returns a publisher bound to js and nc.
func New(js *natsx.Client, nc *nats.Conn) *NATS {
	return &NATS{js: js, nc: nc}
}

// MessageSent emits wb.chat.message_sent.v1.
func (p *NATS) MessageSent(ctx context.Context, msg *chatv1.Message) error {
	if err := p.js.Publish(ctx, subjectMessageSent, msg, msg.GetId()); err != nil {
		return fmt.Errorf("publish message sent: %w", err)
	}
	return nil
}

// Rejected emits wb.chat.rejected.v1 with a reason header.
func (p *NATS) Rejected(ctx context.Context, msg *chatv1.Message, reason string) error {
	if reason == "" {
		reason = "rejected"
	}
	js, err := jetstream.New(p.nc)
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	hdr := nats.Header{}
	hdr.Set(msgIDHeader, msg.GetId())
	hdr.Set(reasonHeader, reason)
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(hdr))
	if _, err := js.PublishMsg(ctx, &nats.Msg{Subject: subjectRejected, Header: hdr, Data: data}); err != nil {
		return fmt.Errorf("publish rejected: %w", err)
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
