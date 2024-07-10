package cached

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
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

func (s *CachedServer) RegisterCSI(cached *api.Cached) {
	csi.RegisterIdentityServer(s.Grpc, cached)
	csi.RegisterNodeServer(s.Grpc, cached)
}

func (s *CachedServer) Serve(lis net.Listener) error {
	return s.Grpc.Serve(lis)
}

func (s *CachedServer) ServeCSI(listenSocketPath string) error {
	u, err := url.Parse(listenSocketPath)
	if err != nil {
		return fmt.Errorf("unable to parse address: %q", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		addr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return fmt.Errorf("currently only unix domain sockets are supported, have incorrect protocol: %s", u.Scheme)
	} else {
		// remove the socket if it's already there. This can happen if we deploy a new version and the socket was created from the old running plugin.
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove unix domain socket file %s, error: %s", addr, err)
		}
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	return s.Grpc.Serve(listener)
}
