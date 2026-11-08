package natsx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestConsumeValidation(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	h := func(context.Context, Incoming) error { return nil }
	if err := c.Consume(ctx, "", "d", h); err == nil {
		t.Fatal("expected stream error")
	}
	if err := c.Consume(ctx, "BOARD", "", h); err == nil {
		t.Fatal("expected durable error")
	}
	if err := c.Consume(ctx, "BOARD", "d", nil); err == nil {
		t.Fatal("expected handler error")
	}
}

func TestBackoffDelaySteps(t *testing.T) {
	if backoffDelay(1) != 100*time.Millisecond || backoffDelay(2) != 200*time.Millisecond || backoffDelay(4) != 800*time.Millisecond {
		t.Fatal()
	}
}

func TestHeaderCarrier(t *testing.T) {
	c := headerCarrier(make(nats.Header))
	c.Set("traceparent", "00-abc")
	if c.Get("traceparent") != "00-abc" {
		t.Fatal(c.Get("traceparent"))
	}
	keys := c.Keys()
	if len(keys) != 1 || keys[0] != "traceparent" {
		t.Fatalf("keys %v", keys)
	}
}

type metaFailMsg struct {
	fakeJetMsg
}

func (m *metaFailMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return nil, errors.New("no metadata")
}

type fakeJetMsg struct {
	subject string
	data    []byte
	hdr     nats.Header
	acked   bool
	naked   bool
}

func (m *fakeJetMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: 1}, nil
}
func (m *fakeJetMsg) Data() []byte         { return m.data }
func (m *fakeJetMsg) Headers() nats.Header { return m.hdr }
func (m *fakeJetMsg) Subject() string      { return m.subject }
func (m *fakeJetMsg) Reply() string        { return "" }
func (m *fakeJetMsg) Ack() error           { m.acked = true; return nil }
func (m *fakeJetMsg) DoubleAck(context.Context) error { return nil }
func (m *fakeJetMsg) Nak() error           { m.naked = true; return nil }
func (m *fakeJetMsg) NakWithDelay(time.Duration) error { m.naked = true; return nil }
func (m *fakeJetMsg) InProgress() error    { return nil }
func (m *fakeJetMsg) Term() error          { return nil }
func (m *fakeJetMsg) TermWithReason(string) error { return nil }

func TestOnMsgMetadataFailure(t *testing.T) {
	c, _ := connect(t, startEmbedded(t))
	msg := &metaFailMsg{fakeJetMsg: fakeJetMsg{subject: "wb.board.stroke.v1"}}
	c.onMsg(context.Background(), msg, func(context.Context, Incoming) error { return nil })
	if msg.acked || !msg.naked {
		t.Fatalf("ack=%v nak=%v", msg.acked, msg.naked)
	}
}
