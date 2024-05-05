package cached

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/client"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// NewCacheServer returns a CSI plugin that contains the necessary gRPC interfaces to interact with Kubernetes over unix domain sockets for managing volumes on behalf of pods
func NewCacheCSIServer(ctx context.Context, client *client.Client, stagingPath string) *CacheServer {
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_recovery.UnaryServerInterceptor(),
				otelgrpc.UnaryServerInterceptor(),
				logger.UnaryServerInterceptor(),
			),
		),
	)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	cached := &api.Cached{
		Client:      client,
		StagingPath: stagingPath,
	}
	pb.RegisterCachedServer(grpcServer, cached)

	csi.RegisterIdentityServer(grpcServer, cached)
	csi.RegisterNodeServer(grpcServer, cached)

	server := &CacheServer{
		Grpc:   grpcServer,
		Health: healthServer,
	}

	return server
}

// Run starts the CSI plugin by communication over the given endpoint
func (s *CacheServer) ServeCSI(ctx context.Context, listenSocketPath string) error {
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
