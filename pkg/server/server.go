package server

import (
	"context"
	"net"
	"time"

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
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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

func NewServer(log *zap.Logger) *Server {
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_zap.UnaryServerInterceptor(log),
				grpc_recovery.UnaryServerInterceptor(),
			),
		),
		grpc.MaxRecvMsgSize(50*MB),
		grpc.MaxSendMsgSize(50*MB),
	)

	log.Info("register HealthServer")
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	return &Server{log: log, Grpc: grpcServer, Health: healthServer, Env: environment.LoadEnvironment()}
}

func (s *Server) MonitorDbPool(ctx context.Context, pool *DbPoolConnector) {
	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.Health.SetServingStatus("dateilager.server", healthpb.HealthCheckResponse_NOT_SERVING)
			case <-ticker.C:
				ctxTimeout, cancel := context.WithTimeout(ctx, 800*time.Millisecond)

				status := healthpb.HealthCheckResponse_SERVING
				err := pool.Ping(ctxTimeout)
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
