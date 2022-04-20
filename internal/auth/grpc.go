package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/logger"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func UnaryServerInterceptor(validator *AuthValidator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "missing request metadata")
		}

		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}

		token, err := getToken(md["authorization"])
		if err != nil {
			logger.Error(ctx, "Auth token parse error", zap.Error(err))
			return nil, status.Errorf(codes.Unauthenticated, "missing authorization token")
		}

		reqAuth, err := validator.Validate(ctx, token)
		if err != nil || reqAuth.Role == None {
			logger.Error(ctx, "Auth token validation error", zap.Error(err))
			return nil, status.Errorf(codes.PermissionDenied, "invalid authorization token")
		}

		ctx = context.WithValue(ctx, AuthCtxKey, reqAuth)

		return handler(ctx, req)
	}
}

func StreamServerInterceptor(validator *AuthValidator) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := stream.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return status.Errorf(codes.InvalidArgument, "missing request metadata")
		}

		token, err := getToken(md["authorization"])
		if err != nil {
			logger.Error(ctx, "Auth token parse error", zap.Error(err))
			return status.Errorf(codes.Unauthenticated, "missing authorization token")
		}

		reqAuth, err := validator.Validate(ctx, token)
		if err != nil || reqAuth.Role == None {
			logger.Error(ctx, "Auth token validation error", zap.Error(err))
			return status.Errorf(codes.PermissionDenied, "invalid authorization token")
		}

		wrapped := grpc_middleware.WrapServerStream(stream)
		wrapped.WrappedContext = context.WithValue(ctx, AuthCtxKey, reqAuth)

		return handler(srv, stream)
	}
}

func getToken(values []string) (string, error) {
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", fmt.Errorf("invalid authorization token")
	}
	return strings.TrimPrefix(values[0], "Bearer "), nil
}
