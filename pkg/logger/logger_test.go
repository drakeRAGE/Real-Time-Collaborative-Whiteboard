package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestJSONFieldsIncludeServiceLevelAndTrace(t *testing.T) {
	var buf bytes.Buffer
	base := New("board", "info").Output(&buf)

	tp := sdktrace.NewTracerProvider()
	ctx, span := tp.Tracer("logger-test").Start(context.Background(), "apply")
	defer span.End()

	traced := WithTrace(ctx, base)
	traced.Info().Msg("hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal %q: %v", buf.String(), err)
	}
	if rec["service"] != "board" {
		t.Fatalf("service=%v", rec["service"])
	}
	if rec["level"] != "info" {
		t.Fatalf("level=%v", rec["level"])
	}
	sc := span.SpanContext()
	if rec["trace_id"] != sc.TraceID().String() {
		t.Fatalf("trace_id=%v want %s", rec["trace_id"], sc.TraceID())
	}
	if rec["span_id"] != sc.SpanID().String() {
		t.Fatalf("span_id=%v want %s", rec["span_id"], sc.SpanID())
	}
}

func TestUnknownLevelDefaultsToInfo(t *testing.T) {
	var buf bytes.Buffer
	log := New("gateway", "not-a-level").Output(&buf)
	log.Info().Msg("x")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["level"] != "info" || rec["service"] != "gateway" {
		t.Fatalf("record=%v", rec)
	}
}

func TestWithTraceNoSpan(t *testing.T) {
	var buf bytes.Buffer
	base := New("chat", "debug").Output(&buf)
	traced := WithTrace(context.Background(), base)
	traced.Debug().Msg("plain")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec["trace_id"]; ok {
		t.Fatalf("unexpected trace_id: %v", rec["trace_id"])
	}
	if rec["level"] != "debug" {
		t.Fatalf("level=%v", rec["level"])
	}
}
