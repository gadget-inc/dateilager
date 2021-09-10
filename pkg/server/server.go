package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/pkg/api"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	MB = 1000 * 1000
)

type DbPoolConnector struct {
	pool *pgxpool.Pool
}

func NewDbPoolConnector(ctx context.Context, uri string) (*DbPoolConnector, error) {
	pool, err := pgxpool.Connect(ctx, uri)
	if err != nil {
		return nil, err
	}

	return &DbPoolConnector{
		pool: pool,
	}, nil
}

func (d *DbPoolConnector) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

func (d *DbPoolConnector) Close() {
	d.pool.Close()
}

func (d *DbPoolConnector) Connect(ctx context.Context) (pgx.Tx, db.CloseFunc, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	return tx, func() { tx.Rollback(ctx); conn.Release() }, nil
}

type Server struct {
	log    *zap.Logger
	Grpc   *grpc.Server
	Health *health.Server
	Env    environment.Env
}

func NewServer(ctx context.Context, log *zap.Logger, dbConn *DbPoolConnector, cert *tls.Certificate) (*Server, error) {
	creds := credentials.NewServerTLSFromCert(cert)

	validator, err := auth.NewAuthValidator(dbConn)
	if err != nil {
		return nil, fmt.Errorf("build auth validator: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_zap.UnaryServerInterceptor(log),
				grpc_recovery.UnaryServerInterceptor(),
				grpc.UnaryServerInterceptor(validateTokenUnary(log, validator)),
			),
		),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_zap.StreamServerInterceptor(log),
				grpc_recovery.StreamServerInterceptor(),
				grpc.StreamServerInterceptor(validateTokenStream(log, validator)),
			),
		),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_zap.StreamServerInterceptor(log),
				grpc_recovery.StreamServerInterceptor(),
			),
		),
		grpc.MaxRecvMsgSize(50*MB),
		grpc.MaxSendMsgSize(50*MB),
		grpc.Creds(creds),
	)

	log.Info("register HealthServer")
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	server := &Server{
		log:    log,
		Grpc:   grpcServer,
		Health: healthServer,
		Env:    environment.LoadEnvironment(),
	}

	server.monitorDbPool(ctx, dbConn)

	return server, nil
}

func (s *Server) monitorDbPool(ctx context.Context, dbConn *DbPoolConnector) {
	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.Health.SetServingStatus("dateilager.server", healthpb.HealthCheckResponse_NOT_SERVING)
			case <-ticker.C:
				ctxTimeout, cancel := context.WithTimeout(ctx, 800*time.Millisecond)

				status := healthpb.HealthCheckResponse_SERVING
				err := dbConn.Ping(ctxTimeout)
				if err != nil {
					status = healthpb.HealthCheckResponse_NOT_SERVING
				}
				cancel()

				s.Health.SetServingStatus("dateilager.server", status)
			}
		}
	}()
}

func (s *Server) RegisterFs(ctx context.Context, fs *api.Fs) {
	pb.RegisterFsServer(s.Grpc, fs)
}

func (s *Server) Serve(lis net.Listener) error {
	return s.Grpc.Serve(lis)
}

func validateTokenUnary(log *zap.Logger, validator *auth.AuthValidator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "missing request metadata")
		}

		token, err := getToken(md["authorization"])
		if err != nil {
			log.Error("Auth token parse error", zap.Error(err))
			return nil, status.Errorf(codes.Unauthenticated, "missing authorization token")
		}

		reqAuth, err := validator.Validate(ctx, token)
		if err != nil || reqAuth.Role == auth.None {
			log.Error("Auth token validation error", zap.Error(err))
			return nil, status.Errorf(codes.PermissionDenied, "invalid authorization token")
		}

		ctx = context.WithValue(ctx, auth.AuthCtxKey, reqAuth)

		return handler(ctx, req)
	}
}

func validateTokenStream(log *zap.Logger, validator *auth.AuthValidator) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, ok := metadata.FromIncomingContext(stream.Context())
		if !ok {
			return status.Errorf(codes.InvalidArgument, "missing request metadata")
		}

		token, err := getToken(md["authorization"])
		if err != nil {
			log.Error("Auth token parse error", zap.Error(err))
			return status.Errorf(codes.Unauthenticated, "missing authorization token")
		}

		reqAuth, err := validator.Validate(stream.Context(), token)
		if err != nil || reqAuth.Role == auth.None {
			log.Error("Auth token validation error", zap.Error(err))
			return status.Errorf(codes.PermissionDenied, "invalid authorization token")
		}

		wrapped := grpc_middleware.WrapServerStream(stream)
		wrapped.WrappedContext = context.WithValue(stream.Context(), auth.AuthCtxKey, reqAuth)

		return handler(srv, stream)
	}
}

func getToken(values []string) (string, error) {
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", fmt.Errorf("invalid authorization token")
	}
	return strings.TrimPrefix(values[0], "Bearer "), nil
}
