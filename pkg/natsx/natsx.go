// Package natsx publishes and consumes JetStream messages for the whiteboard.
package natsx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"
)

const (
	maxDeliver = 5
	// ErrorHeader is set on wb.dlq.<subject> when a message exhausts delivery attempts.
	ErrorHeader = "Wb-Error"
	msgIDHeader = "Nats-Msg-Id"
)

// Incoming is one JetStream delivery.
type Incoming struct {
	Subject string
	Data    []byte
	Header  nats.Header
}

// Handler handles one delivery. A non-nil error is a failure: the message is
// redelivered with backoff up to maxDeliver, then republished to wb.dlq.<subject>.
type Handler func(ctx context.Context, msg Incoming) error

// Client is a JetStream helper bound to one connection.
type Client struct {
	nc *nats.Conn
	js jetstream.JetStream
}

// ConnectOptions returns nats.Connect options for a named client. creds is the
// contents of a .creds file (user JWT and nkey seed); empty means no auth.
func ConnectOptions(name, creds string) ([]nats.Option, error) {
	opts := []nats.Option{nats.Name(name)}
	if creds == "" {
		return opts, nil
	}
	userJWT, err := nkeys.ParseDecoratedJWT([]byte(creds))
	if err != nil {
		return nil, fmt.Errorf("natsx: creds jwt: %w", err)
	}
	kp, err := nkeys.ParseDecoratedUserNKey([]byte(creds))
	if err != nil {
		return nil, fmt.Errorf("natsx: creds seed: %w", err)
	}
	seed, err := kp.Seed()
	if err != nil {
		return nil, fmt.Errorf("natsx: creds seed: %w", err)
	}
	return append(opts, nats.UserJWTAndSeed(userJWT, string(seed))), nil
}

// New wraps nc. The caller owns the connection and closes it.
func New(nc *nats.Conn) (*Client, error) {
	if nc == nil {
		return nil, errors.New("natsx: nil conn")
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("natsx: jetstream: %w", err)
	}
	return &Client{nc: nc, js: js}, nil
}

// EnsureStreams creates or updates the BOARD and CHAT streams from docs/CONTRACTS.md.
func (c *Client) EnsureStreams(ctx context.Context) error {
	for _, cfg := range streamConfigs() {
		if _, err := c.js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return fmt.Errorf("natsx: stream %s: %w", cfg.Name, err)
		}
	}
	return nil
}

func streamConfigs() []jetstream.StreamConfig {
	window := 7 * 24 * time.Hour
	dup := 2 * time.Minute
	return []jetstream.StreamConfig{
		{
			Name:       "BOARD",
			Subjects:   []string{"wb.board.>"},
			Retention:  jetstream.LimitsPolicy,
			MaxAge:     window,
			Duplicates: dup,
			Storage:    jetstream.FileStorage,
		},
		{
			Name:       "CHAT",
			Subjects:   []string{"wb.chat.>"},
			Retention:  jetstream.LimitsPolicy,
			MaxAge:     window,
			Duplicates: dup,
			Storage:    jetstream.FileStorage,
		},
	}
}

// Publish marshals msg and sets Nats-Msg-Id so JetStream can drop duplicates.
// OpenTelemetry trace context is written onto the NATS headers.
func (c *Client) Publish(ctx context.Context, subject string, msg proto.Message, msgID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if subject == "" {
		return errors.New("natsx: subject required")
	}
	if msgID == "" {
		return errors.New("natsx: msg id required")
	}
	if msg == nil {
		return errors.New("natsx: nil message")
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("natsx: marshal: %w", err)
	}
	hdr := nats.Header{}
	hdr.Set(msgIDHeader, msgID)
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(hdr))
	if _, err := c.js.PublishMsg(ctx, &nats.Msg{Subject: subject, Header: hdr, Data: data}); err != nil {
		return fmt.Errorf("natsx: publish %s: %w", subject, err)
	}
	return nil
}

// Consume binds a durable consumer and calls handler until ctx is canceled.
// The consumer acks explicitly, allows 5 deliveries, and backs off between them.
// The final failed delivery is acked after it is republished to wb.dlq.<subject>.
func (c *Client) Consume(ctx context.Context, stream, durable string, handler Handler) error {
	if stream == "" || durable == "" {
		return errors.New("natsx: stream and consumer required")
	}
	if handler == nil {
		return errors.New("natsx: nil handler")
	}
	cons, err := c.js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Durable:    durable,
		AckPolicy:  jetstream.AckExplicitPolicy,
		MaxDeliver: maxDeliver,
		BackOff:    redeliveryBackoff(),
		AckWait:    30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("natsx: consumer: %w", err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		c.onMsg(ctx, msg, handler)
	})
	if err != nil {
		return fmt.Errorf("natsx: consume: %w", err)
	}
	defer cc.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-cc.Closed():
		return nil
	}
}

func (c *Client) onMsg(ctx context.Context, msg jetstream.Msg, handler Handler) {
	if ctx.Err() != nil {
		return
	}
	meta, err := msg.Metadata()
	if err != nil {
		_ = msg.Nak()
		return
	}
	hctx := otel.GetTextMapPropagator().Extract(ctx, headerCarrier(msg.Headers()))
	herr := handler(hctx, Incoming{
		Subject: msg.Subject(),
		Data:    msg.Data(),
		Header:  msg.Headers(),
	})
	if herr == nil {
		_ = msg.Ack()
		return
	}
	if meta.NumDelivered >= maxDeliver {
		if dlqErr := c.republishDLQ(msg, herr); dlqErr != nil {
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
		return
	}
	// Nak with the consumer backoff. Server-side BackOff does not apply to a plain Nak.
	_ = msg.NakWithDelay(backoffDelay(meta.NumDelivered))
}

func (c *Client) republishDLQ(msg jetstream.Msg, cause error) error {
	hdr := nats.Header{}
	for k, values := range msg.Headers() {
		for _, v := range values {
			hdr.Add(k, v)
		}
	}
	hdr.Set(ErrorHeader, cause.Error())
	return c.nc.PublishMsg(&nats.Msg{
		Subject: dlqSubject(msg.Subject()),
		Header:  hdr,
		Data:    msg.Data(),
	})
}

func dlqSubject(orig string) string {
	return "wb.dlq." + orig
}

func redeliveryBackoff() []time.Duration {
	return []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}
}

func backoffDelay(delivered uint64) time.Duration {
	steps := redeliveryBackoff()
	switch delivered {
	case 0, 1:
		return steps[0]
	case 2:
		return steps[1]
	case 3:
		return steps[2]
	default:
		return steps[len(steps)-1]
	}
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
