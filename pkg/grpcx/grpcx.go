// Package grpcx builds gRPC servers and clients with the platform interceptor chain.
package grpcx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/drakeRAGE/realtime-whiteboard/pkg/logger"
)

const (
	retryAttempts = 3
	retryBackoff  = 100 * time.Millisecond
)

// NewServer returns a gRPC server whose unary chain is
// recovery, OpenTelemetry, logging, then protovalidate.
func NewServer(log zerolog.Logger, opts ...grpc.ServerOption) *grpc.Server {
	validator, vErr := protovalidate.New()
	base := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			recoveryUnary(log),
			otelUnary(),
			loggingUnary(log),
			validateUnary(validator, vErr),
		),
	}
	return grpc.NewServer(append(base, opts...)...)
}

// Dial returns a client that propagates traces and retries UNAVAILABLE
// three times with 100ms between attempts. Targets on port 443 use TLS;
// all others are plaintext.
func Dial(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	base := []grpc.DialOption{
		grpc.WithTransportCredentials(transportCredentials(target)),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(retryUnavailable),
	}
	conn, err := grpc.NewClient(target, append(base, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("grpcx: dial %s: %w", target, err)
	}
	return conn, nil
}

func transportCredentials(target string) credentials.TransportCredentials {
	if strings.HasSuffix(target, ":443") {
		return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	return insecure.NewCredentials()
}

func recoveryUnary(log zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().Str("panic", fmt.Sprint(rec)).Msg("grpc panic")
				resp = nil
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

func otelUnary() grpc.UnaryServerInterceptor {
	tracer := otel.Tracer("github.com/drakeRAGE/realtime-whiteboard/pkg/grpcx")
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			ctx = otel.GetTextMapPropagator().Extract(ctx, mdCarrier(md))
		}
		ctx, span := tracer.Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		resp, err := handler(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, status.Code(err).String())
		}
		return resp, err
	}
}

func loggingUnary(log zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		entry := logger.WithTrace(ctx, log)
		entry.Info().
			Str("grpc.method", info.FullMethod).
			Str("grpc.code", status.Code(err).String()).
			Dur("grpc.duration", time.Since(start)).
			Msg("grpc")
		return resp, err
	}
}

func validateUnary(v protovalidate.Validator, initErr error) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if initErr != nil {
			return nil, status.Error(codes.Internal, "validator unavailable")
		}
		msg, ok := req.(proto.Message)
		if ok && msg != nil {
			if err := v.Validate(msg); err != nil {
				return nil, status.Error(codes.InvalidArgument, validationMessage(err))
			}
		}
		return handler(ctx, req)
	}
}

func validationMessage(err error) string {
	var ve *protovalidate.ValidationError
	if errors.As(err, &ve) {
		return ve.Error()
	}
	return err.Error()
}

func retryUnavailable(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	var err error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		err = invoker(ctx, method, req, reply, cc, opts...)
		if status.Code(err) != codes.Unavailable {
			return err
		}
		if attempt == retryAttempts-1 {
			return err
		}
		timer := time.NewTimer(retryBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

type mdCarrier metadata.MD

func (c mdCarrier) Get(key string) string {
	vals := metadata.MD(c).Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c mdCarrier) Set(key, value string) { metadata.MD(c).Set(key, value) }

func (c mdCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range metadata.MD(c) {
		keys = append(keys, k)
	}
	return keys
}
