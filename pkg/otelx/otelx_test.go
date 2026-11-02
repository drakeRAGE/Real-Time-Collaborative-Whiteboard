package otelx

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestInitRequiresService(t *testing.T) {
	if _, err := Init("  "); err == nil {
		t.Fatal("expected error")
	}
}

func TestInitReturnsShutdown(t *testing.T) {
	shutdown, err := Init("board")
	if err != nil {
		t.Fatal(err)
	}
	if shutdown == nil {
		t.Fatal("nil shutdown")
	}
	_, span := otel.Tracer("otelx-test").Start(context.Background(), "op")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}
