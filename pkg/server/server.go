package server

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/api"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	KB                       = 1024
	MB                       = KB * KB
	BUFFER_SIZE              = 28 * KB
	INITIAL_WINDOW_SIZE      = 1 * MB
	INITIAL_CONN_WINDOW_SIZE = 2 * INITIAL_WINDOW_SIZE
	MAX_MESSAGE_SIZE         = 300 * MB
	MAX_POOL_SIZE            = 60
)

type DbPoolConnector struct {
	pool       *pgxpool.Pool
	extraTypes []*pgtype.Type
}

func NewDbPoolConnector(ctx context.Context, uri string) (*DbPoolConnector, error) {
	config, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return nil, err
	}

	config.MaxConns = MAX_POOL_SIZE

	if os.Getenv("DL_PGX_TRACING") == "1" {
		config.ConnConfig.Tracer = telemetry.NewQueryTracer()
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	var extraTypes []*pgtype.Type
	for _, typeName := range []string{"hash", "hash[]"} {
		extraType, err := conn.Conn().LoadType(ctx, typeName)
		if err != nil {
			return nil, fmt.Errorf("could not load type %s: %w", typeName, err)
		}

		conn.Conn().TypeMap().RegisterType(extraType)
		extraTypes = append(extraTypes, extraType)
	}

	return &DbPoolConnector{
		pool:       pool,
		extraTypes: extraTypes,
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

	for _, extraType := range d.extraTypes {
		conn.Conn().TypeMap().RegisterType(extraType)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	return tx, func(ctx context.Context) { _ = tx.Rollback(ctx); conn.Release() }, nil
}

func (d *DbPoolConnector) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return d.pool.Query(ctx, sql, args...)
}

func (d *DbPoolConnector) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return d.pool.Exec(ctx, sql, args...)
}

type Server struct {
	Grpc   *grpc.Server
	Health *health.Server
}

func NewServer(ctx context.Context, dbConn *DbPoolConnector, cert *tls.Certificate, pasetoKey ed25519.PublicKey) *Server {
	creds := credentials.NewServerTLSFromCert(cert)

	validator := auth.NewAuthValidator(pasetoKey)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_recovery.UnaryServerInterceptor(),
				otelgrpc.UnaryServerInterceptor(),
				logger.UnaryServerInterceptor(),
				ValidateTokenUnary(validator),
			),
		),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_recovery.StreamServerInterceptor(),
				otelgrpc.StreamServerInterceptor(),
				logger.StreamServerInterceptor(),
				validateTokenStream(validator),
			),
		),
		grpc.ReadBufferSize(BUFFER_SIZE),
		grpc.WriteBufferSize(BUFFER_SIZE),
		grpc.InitialConnWindowSize(INITIAL_CONN_WINDOW_SIZE),
		grpc.InitialWindowSize(INITIAL_WINDOW_SIZE),
		grpc.MaxRecvMsgSize(MAX_MESSAGE_SIZE),
		grpc.MaxSendMsgSize(MAX_MESSAGE_SIZE),
		grpc.Creds(creds),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             2 * time.Second,
			PermitWithoutStream: true,
		}),
	)

	logger.Info(ctx, "register HealthServer")
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	server := &Server{
		Grpc:   grpcServer,
		Health: healthServer,
	}

	server.monitorDbPool(ctx, dbConn)

	return server
}

func (s *Server) monitorDbPool(ctx context.Context, dbConn *DbPoolConnector) {
	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.Health.SetServingStatus("dateilager.server", healthpb.HealthCheckResponse_NOT_SERVING)
				return
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

func ValidateTokenUnary(validator *auth.AuthValidator) grpc.UnaryServerInterceptor {
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
		if err != nil || reqAuth.Role == auth.None {
			logger.Error(ctx, "Auth token validation error", zap.Error(err))
			return nil, status.Errorf(codes.PermissionDenied, "invalid authorization token")
		}

		ctx = context.WithValue(ctx, auth.AuthCtxKey, reqAuth)

		return handler(ctx, req)
	}
}

func validateTokenStream(validator *auth.AuthValidator) grpc.StreamServerInterceptor {
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
		if err != nil || reqAuth.Role == auth.None {
			logger.Error(ctx, "Auth token validation error", zap.Error(err))
			return status.Errorf(codes.PermissionDenied, "invalid authorization token")
		}

		wrapped := grpc_middleware.WrapServerStream(stream)
		wrapped.WrappedContext = context.WithValue(ctx, auth.AuthCtxKey, reqAuth)

		return handler(srv, stream)
	}
}

func getToken(values []string) (string, error) {
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", fmt.Errorf("invalid authorization token")
	}
	return strings.TrimPrefix(values[0], "Bearer "), nil
}
