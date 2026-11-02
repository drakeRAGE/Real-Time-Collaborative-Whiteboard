// Package ingest is the board-ingest JetStream consumer.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/service"
)

const (
	streamName  = "BOARD"
	durableName = "board-ingest"
	maxDeliver  = 5
	errorHeader = "Wb-Error"
)

// Ingest subjects only. Applied events stay on the stream for the gateway.
var filterSubjects = []string{
	"wb.board.stroke.v1",
	"wb.board.shape.v1",
	"wb.board.clear.v1",
	"wb.board.undo.v1",
	"wb.board.redo.v1",
}

// applier is the board service. The consumer acks only after a method returns.
type applier interface {
	ApplyStroke(ctx context.Context, principal uuid.UUID, in *realtimev1.StrokeInput) error
	ApplyShape(ctx context.Context, principal uuid.UUID, in *realtimev1.ShapeInput) error
	ApplyClear(ctx context.Context, principal, roomID uuid.UUID, opID string) error
	ApplyUndo(ctx context.Context, principal, roomID uuid.UUID, opID string) error
	ApplyRedo(ctx context.Context, principal, roomID uuid.UUID, opID string) error
}

// Consumer binds board-ingest.
type Consumer struct {
	apply applier
	nc    *nats.Conn
	log   zerolog.Logger
	v     protovalidate.Validator
}

// New returns a consumer that applies messages with svc.
func New(svc *service.Service, nc *nats.Conn, log zerolog.Logger) (*Consumer, error) {
	v, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("validator: %w", err)
	}
	return &Consumer{apply: svc, nc: nc, log: log, v: v}, nil
}

// Run creates the durable consumer and reads until ctx is canceled.
// pkg/natsx.Consume cannot set filter subjects, so this uses the same ack
// policy (explicit ack, max deliver 5, backoff, DLQ) with the CONTRACTS filters.
func Run(ctx context.Context, nc *nats.Conn, svc *service.Service, log zerolog.Logger) error {
	c, err := New(svc, nc, log)
	if err != nil {
		return err
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}
	cons, err := js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable:        durableName,
		FilterSubjects: filterSubjects,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxDeliver:     maxDeliver,
		MaxAckPending:  1,
		BackOff: []time.Duration{
			100 * time.Millisecond,
			200 * time.Millisecond,
			400 * time.Millisecond,
			800 * time.Millisecond,
		},
		AckWait: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("board consumer: %w", err)
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
	err := c.dispatch(ctx, msg.Subject(), msg.Data(), msg.Headers())
	if err == nil {
		_ = msg.Ack()
		return
	}
	if service.IsPermanent(err) {
		c.log.Warn().Err(err).Str("subject", msg.Subject()).Msg("board ingest drop")
		_ = msg.Ack()
		return
	}
	meta, metaErr := msg.Metadata()
	if metaErr != nil {
		_ = msg.Nak()
		return
	}
	if meta.NumDelivered >= maxDeliver {
		if dlqErr := c.toDLQ(msg, err); dlqErr != nil {
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
		return
	}
	_ = msg.NakWithDelay(backoffDelay(meta.NumDelivered))
}

func (c *Consumer) dispatch(ctx context.Context, subject string, data []byte, hdr nats.Header) error {
	if !isIngest(subject) {
		return nil
	}
	userID, err := uuid.Parse(hdr.Get("Wb-User-Id"))
	if err != nil {
		return service.ErrUnauthenticated
	}
	msgID := hdr.Get("Nats-Msg-Id")
	if !service.ValidID(msgID) {
		return service.ErrInvalidArgument
	}
	switch subject {
	case "wb.board.stroke.v1":
		var in realtimev1.StrokeInput
		if err := decode(c.v, data, &in); err != nil {
			return err
		}
		if in.GetStrokeId() != msgID {
			return service.ErrInvalidArgument
		}
		return c.apply.ApplyStroke(ctx, userID, &in)
	case "wb.board.shape.v1":
		var in realtimev1.ShapeInput
		if err := decode(c.v, data, &in); err != nil {
			return err
		}
		if in.GetShapeId() != msgID {
			return service.ErrInvalidArgument
		}
		return c.apply.ApplyShape(ctx, userID, &in)
	case "wb.board.clear.v1":
		roomID, err := roomOf(c.v, data, &realtimev1.ClearInput{})
		if err != nil {
			return err
		}
		return c.apply.ApplyClear(ctx, userID, roomID, msgID)
	case "wb.board.undo.v1":
		roomID, err := roomOf(c.v, data, &realtimev1.UndoInput{})
		if err != nil {
			return err
		}
		return c.apply.ApplyUndo(ctx, userID, roomID, msgID)
	case "wb.board.redo.v1":
		roomID, err := roomOf(c.v, data, &realtimev1.RedoInput{})
		if err != nil {
			return err
		}
		return c.apply.ApplyRedo(ctx, userID, roomID, msgID)
	default:
		return nil
	}
}

func decode(v protovalidate.Validator, data []byte, msg proto.Message) error {
	if err := proto.Unmarshal(data, msg); err != nil {
		return service.ErrInvalidArgument
	}
	if err := v.Validate(msg); err != nil {
		return service.ErrInvalidArgument
	}
	return nil
}

type roomMessage interface {
	proto.Message
	GetRoomId() string
}

func roomOf(v protovalidate.Validator, data []byte, msg roomMessage) (uuid.UUID, error) {
	if err := decode(v, data, msg); err != nil {
		return uuid.Nil, err
	}
	roomID, err := uuid.Parse(msg.GetRoomId())
	if err != nil {
		return uuid.Nil, service.ErrInvalidArgument
	}
	return roomID, nil
}

func isIngest(subject string) bool {
	for _, s := range filterSubjects {
		if s == subject {
			return true
		}
	}
	return false
}

func (c *Consumer) toDLQ(msg jetstream.Msg, cause error) error {
	if c.nc == nil {
		return errors.New("nil conn")
	}
	hdr := nats.Header{}
	for k, values := range msg.Headers() {
		for _, v := range values {
			hdr.Add(k, v)
		}
	}
	hdr.Set(errorHeader, cause.Error())
	if err := c.nc.PublishMsg(&nats.Msg{
		Subject: "wb.dlq." + msg.Subject(),
		Header:  hdr,
		Data:    msg.Data(),
	}); err != nil {
		return fmt.Errorf("dlq: %w", err)
	}
	return nil
}

func backoffDelay(delivered uint64) time.Duration {
	steps := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}
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
