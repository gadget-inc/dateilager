package logger

import (
	"context"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"path"
	"time"
)

func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}

		ctx = context.WithValue(ctx, key, Logger(ctx).With(
			zap.String("grpc.service", path.Dir(info.FullMethod)[1:]),
			zap.String("grpc.method", path.Base(info.FullMethod))),
		)

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := status.Code(err)

		Check(ctx, grpc_zap.DefaultCodeToLevel(code), "finished unary call",
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

		ctx := stream.Context()
		ctx = context.WithValue(ctx, key, Logger(ctx).With(
			zap.String("grpc.service", path.Dir(info.FullMethod)[1:]),
			zap.String("grpc.method", path.Base(info.FullMethod))),
		)

		wrapped := grpc_middleware.WrapServerStream(stream)
		wrapped.WrappedContext = ctx

		start := time.Now()
		err := handler(srv, wrapped)
		duration := time.Since(start)

		code := status.Code(err)

		Check(ctx, grpc_zap.DefaultCodeToLevel(code), "finished streaming call",
			zap.Stringer("grpc.code", code),
			zap.Duration("grpc.duration", duration),
			zap.Error(err),
		)

		return err
	}
}
