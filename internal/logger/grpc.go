package logger

import (
	"context"
	"path"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}

		fields := []zap.Field{
			zap.String("grpc.service", path.Dir(info.FullMethod)[1:]),
			zap.String("grpc.method", path.Base(info.FullMethod)),
		}

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			fields = append(fields, zap.String("trace.trace_id", span.SpanContext().TraceID().String()))
		}

		ctx = With(ctx, fields...)

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := status.Code(err)

		Write(ctx, grpc_zap.DefaultCodeToLevel(code), "finished unary call",
			zap.Stringer("grpc.code", code),
			zap.Duration("grpc.duration", duration),
			zap.Error(err),
		)

		return resp, err
	}
}

func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(srv, stream)
		}

		fields := []zap.Field{
			zap.String("grpc.service", path.Dir(info.FullMethod)[1:]),
			zap.String("grpc.method", path.Base(info.FullMethod)),
		}

		ctx := stream.Context()
		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			fields = append(fields, zap.String("trace.trace_id", span.SpanContext().TraceID().String()))
		}

		ctx = With(ctx, fields...)

		wrapped := grpc_middleware.WrapServerStream(stream)
		wrapped.WrappedContext = ctx

		start := time.Now()
		err := handler(srv, wrapped)
		duration := time.Since(start)

		code := status.Code(err)

		Write(ctx, grpc_zap.DefaultCodeToLevel(code), "finished streaming call",
			zap.Stringer("grpc.code", code),
			zap.Duration("grpc.duration", duration),
			zap.Error(err),
		)

		return err
	}
}
