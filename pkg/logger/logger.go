// Package logger builds JSON loggers with OpenTelemetry trace fields.
package logger

import (
	"context"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

// New returns a JSON logger writing to stdout. An empty or unknown level
// falls back to info. The service name is included on every record.
func New(service, level string) zerolog.Logger {
	return zerolog.New(os.Stdout).Level(parseLevel(level)).With().
		Timestamp().
		Str("service", service).
		Logger()
}

// WithTrace returns a logger that carries the span's trace_id and span_id
// when ctx holds a valid OpenTelemetry span. Otherwise it returns log unchanged.
func WithTrace(ctx context.Context, log zerolog.Logger) zerolog.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return log
	}
	return log.With().
		Str("trace_id", sc.TraceID().String()).
		Str("span_id", sc.SpanID().String()).
		Logger()
}

func parseLevel(level string) zerolog.Level {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return zerolog.InfoLevel
	}
	parsed, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.InfoLevel
	}
	return parsed
}
