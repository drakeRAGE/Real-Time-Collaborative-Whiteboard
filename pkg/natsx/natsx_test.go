package natsx

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	presencev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/presence/v1"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestEmbeddedBehaviors(t *testing.T) {
	runSuite(t, startEmbedded(t))
}

func TestPublishGuards(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	msg := &presencev1.CursorMoved{RoomId: "r"}
	if err := c.Publish(ctx, "", msg, "id"); err == nil {
		t.Fatal("expected subject error")
	}
	if err := c.Publish(ctx, "wb.board.stroke.v1", msg, ""); err == nil {
		t.Fatal("expected msg id error")
	}
	if err := c.Publish(ctx, "wb.board.stroke.v1", nil, "id"); err == nil {
		t.Fatal("expected nil message error")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.Publish(canceled, "wb.board.stroke.v1", msg, "id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if _, err := New(nil); err == nil {
		t.Fatal("expected nil conn error")
	}
	if dlqSubject("wb.board.stroke.v1") != "wb.dlq.wb.board.stroke.v1" {
		t.Fatal(dlqSubject("wb.board.stroke.v1"))
	}
}

func runSuite(t *testing.T, url string) {
	t.Helper()
	t.Run("duplicate msg id delivered once", func(t *testing.T) {
		c, _ := connect(t, url)
		resetStreams(t, c)
		assertStreams(t, c)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var n atomic.Int32
		errCh := startConsume(ctx, c, "BOARD", "dup", func(_ context.Context, _ Incoming) error {
			n.Add(1)
			return nil
		})

		msg := &presencev1.CursorMoved{RoomId: "room", UserId: "u", X: 1, Y: 2}
		publish(t, c, context.Background(), "wb.board.stroke.v1", "stroke-1", msg)
		publish(t, c, context.Background(), "wb.board.stroke.v1", "stroke-1", msg)
		waitFor(t, 5*time.Second, func() bool { return n.Load() >= 1 })
		time.Sleep(300 * time.Millisecond)
		if got := n.Load(); got != 1 {
			t.Fatalf("deliveries=%d", got)
		}
		if msgs := streamMsgs(t, c, "BOARD"); msgs != 1 {
			t.Fatalf("stream msgs=%d", msgs)
		}
		cancel()
		waitErr(t, errCh)
	})

	t.Run("five failures go to dlq", func(t *testing.T) {
		c, nc := connect(t, url)
		resetStreams(t, c)
		sub, err := nc.SubscribeSync("wb.dlq.wb.board.stroke.v1")
		if err != nil {
			t.Fatal(err)
		}
		if err := nc.Flush(); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var n atomic.Int32
		errCh := startConsume(ctx, c, "BOARD", "dlq", func(context.Context, Incoming) error {
			n.Add(1)
			return errors.New("db down")
		})

		msg := &presencev1.CursorMoved{RoomId: "room", UserId: "u", X: 3, Y: 4}
		publish(t, c, context.Background(), "wb.board.stroke.v1", "stroke-dlq", msg)
		dlq, err := sub.NextMsg(20 * time.Second)
		if err != nil {
			t.Fatalf("dlq after %d deliveries: %v", n.Load(), err)
		}
		if n.Load() != maxDeliver {
			t.Fatalf("deliveries=%d", n.Load())
		}
		if got := dlq.Header.Get(ErrorHeader); !strings.Contains(got, "db down") {
			t.Fatalf("error header %q", got)
		}
		if string(dlq.Data) == "" && msg.GetRoomId() != "" {
			// CursorMoved with fields marshals to non-empty bytes.
			t.Fatal("expected payload")
		}
		cancel()
		waitErr(t, errCh)
	})

	t.Run("trace header present", func(t *testing.T) {
		c, _ := connect(t, url)
		resetStreams(t, c)

		tp := sdktrace.NewTracerProvider()
		t.Cleanup(func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = tp.Shutdown(shutCtx)
		})
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.TraceContext{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		got := make(chan Incoming, 1)
		errCh := startConsume(ctx, c, "BOARD", "trace", func(_ context.Context, msg Incoming) error {
			got <- msg
			return nil
		})

		spanCtx, span := tp.Tracer("natsx-test").Start(context.Background(), "publish")
		publish(t, c, spanCtx, "wb.board.stroke.v1", "stroke-trace", &presencev1.CursorMoved{RoomId: "room"})
		span.End()

		select {
		case msg := <-got:
			traceparent := msg.Header.Get("traceparent")
			if !strings.Contains(traceparent, span.SpanContext().TraceID().String()) {
				t.Fatalf("traceparent %q", traceparent)
			}
			if msg.Header.Get(msgIDHeader) != "stroke-trace" {
				t.Fatalf("msg id %q", msg.Header.Get(msgIDHeader))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message")
		}
		cancel()
		waitErr(t, errCh)
	})
}

func startEmbedded(t *testing.T) string {
	t.Helper()
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	t.Cleanup(ns.Shutdown)
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats not ready")
	}
	return ns.ClientURL()
}

func connect(t *testing.T, url string) (*Client, *nats.Conn) {
	t.Helper()
	nc, err := nats.Connect(url, nats.Name("natsx-test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	c, err := New(nc)
	if err != nil {
		t.Fatal(err)
	}
	return c, nc
}

func resetStreams(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.js.DeleteStream(ctx, "BOARD")
	_ = c.js.DeleteStream(ctx, "CHAT")
	if err := c.EnsureStreams(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureStreams(ctx); err != nil {
		t.Fatal(err)
	}
}

func assertStreams(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, name := range []string{"BOARD", "CHAT"} {
		st, err := c.js.Stream(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		info, err := st.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if info.Config.MaxAge != 7*24*time.Hour || info.Config.Duplicates != 2*time.Minute {
			t.Fatalf("%s config maxAge=%s dup=%s", name, info.Config.MaxAge, info.Config.Duplicates)
		}
		if info.Config.Retention != jetstream.LimitsPolicy || info.Config.Storage != jetstream.FileStorage {
			t.Fatalf("%s retention/storage", name)
		}
		if info.Config.MaxBytes != streamMaxBytes || info.Config.Replicas != 1 {
			t.Fatalf("%s maxBytes=%d replicas=%d", name, info.Config.MaxBytes, info.Config.Replicas)
		}
	}
}

func streamMsgs(t *testing.T, c *Client, name string) uint64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := c.js.Stream(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	info, err := st.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return info.State.Msgs
}

func publish(t *testing.T, c *Client, parent context.Context, subject, id string, msg *presencev1.CursorMoved) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := c.Publish(ctx, subject, msg, id); err != nil {
		t.Fatal(err)
	}
}

func startConsume(ctx context.Context, c *Client, stream, durable string, h Handler) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Consume(ctx, stream, durable, h)
	}()
	return errCh
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met")
}

func waitErr(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consume did not return")
	}
}
