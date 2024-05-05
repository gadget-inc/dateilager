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
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/server"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type CacheServer struct {
	Grpc   *grpc.Server
	Health *health.Server
	Cached *api.Cached
}

func NewServer(ctx context.Context, client *client.Client, cert *tls.Certificate, stagingPath string, pasetoKey ed25519.PublicKey) *CacheServer {
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

	cached := &api.Cached{
		Client:      client,
		StagingPath: stagingPath,
	}
	pb.RegisterCacheServer(grpcServer, cached)

	server := &CacheServer{
		Grpc:   grpcServer,
		Health: healthServer,
		Cached: cached,
	}

	return server
}

func (s *CacheServer) Serve(lis net.Listener) error {
	return s.Grpc.Serve(lis)
}
