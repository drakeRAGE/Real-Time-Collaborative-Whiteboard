// Package otelx initializes an OpenTelemetry tracer provider.
package otelx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Init installs a process-wide tracer provider and W3C propagator.
// Spans export over OTLP gRPC to OTEL_EXPORTER_OTLP_ENDPOINT (SDK default
// localhost:4317 when unset). The returned function flushes and shuts the provider down.
func Init(service string) (func(context.Context) error, error) {
	if strings.TrimSpace(service) == "" {
		return nil, errors.New("otelx: service name required")
	}

	ctx := context.Background()
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithTimeout(time.Second))
	if err != nil {
		return nil, fmt.Errorf("otelx: exporter: %w", err)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(service),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}
