package cached

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"net"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/server"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type CachedServer struct {
	Grpc   *grpc.Server
	Health *health.Server
}

func NewServer(ctx context.Context, cert *tls.Certificate, pasetoKey ed25519.PublicKey) *CachedServer {
	creds := credentials.NewServerTLSFromCert(cert)
	validator := auth.NewAuthValidator(pasetoKey)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_recovery.UnaryServerInterceptor(),
				otelgrpc.UnaryServerInterceptor(),
				logger.UnaryServerInterceptor(),
				server.ValidateTokenUnary(validator),
			),
		),
		grpc.ReadBufferSize(server.BUFFER_SIZE),
		grpc.WriteBufferSize(server.BUFFER_SIZE),
		grpc.InitialConnWindowSize(server.INITIAL_CONN_WINDOW_SIZE),
		grpc.InitialWindowSize(server.INITIAL_WINDOW_SIZE),
		grpc.MaxRecvMsgSize(server.MAX_MESSAGE_SIZE),
		grpc.MaxSendMsgSize(server.MAX_MESSAGE_SIZE),
		grpc.Creds(creds),
	)

	logger.Info(ctx, "register HealthServer")
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	server := &CachedServer{
		Grpc:   grpcServer,
		Health: healthServer,
	}

	return server
}

func (s *CachedServer) RegisterCached(cached *api.Cached) {
	pb.RegisterCachedServer(s.Grpc, cached)
}

func (s *CachedServer) Serve(lis net.Listener) error {
	return s.Grpc.Serve(lis)
}
