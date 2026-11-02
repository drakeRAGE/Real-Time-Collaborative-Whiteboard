package ingest

import (
	"context"
	"fmt"
	"time"

	"buf.build/go/protovalidate"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/chat/internal/service"
)

const (
	streamName    = "CHAT"
	consumerName  = "chat-ingest"
	filterSubject = "wb.chat.message.v1"
	maxDeliver    = 5
)

// Consumer runs the chat-ingest JetStream consumer.
type Consumer struct {
	svc       *service.Service
	validator protovalidate.Validator
}

// New returns a consumer handler.
func New(svc *service.Service) (*Consumer, error) {
	v, err := protovalidate.New()
	if err != nil {
		return nil, err
	}
	return &Consumer{svc: svc, validator: v}, nil
}

// Run binds chat-ingest on wb.chat.message.v1 until ctx is canceled.
func Run(ctx context.Context, nc *nats.Conn, svc *service.Service) error {
	c, err := New(svc)
	if err != nil {
		return err
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}
	cons, err := js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable:        consumerName,
		FilterSubjects: []string{filterSubject},
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxDeliver:     maxDeliver,
		BackOff: []time.Duration{
			100 * time.Millisecond,
			200 * time.Millisecond,
			400 * time.Millisecond,
			800 * time.Millisecond,
		},
		AckWait: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("consumer: %w", err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		c.onMsg(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	defer cc.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-cc.Closed():
		return nil
	}
}

func (c *Consumer) onMsg(ctx context.Context, msg jetstream.Msg) {
	if ctx.Err() != nil {
		return
	}
	var send realtimev1.ChatSend
	if err := proto.Unmarshal(msg.Data(), &send); err != nil {
		_ = msg.Ack()
		return
	}
	if err := c.validator.Validate(&send); err != nil {
		_ = msg.Ack()
		return
	}
	userID, username, err := service.PrincipalFromNATS(msg.Headers())
	if err != nil {
		_ = msg.Ack()
		return
	}
	if err := c.svc.HandleIngest(ctx, &send, userID, username); err != nil {
		meta, _ := msg.Metadata()
		if meta != nil && meta.NumDelivered >= maxDeliver {
			_ = msg.Ack()
			return
		}
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}
