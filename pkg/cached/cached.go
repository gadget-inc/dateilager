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
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

type CachedServer struct {
	Grpc *grpc.Server
}

func NewServer(ctx context.Context) *CachedServer {
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_recovery.UnaryServerInterceptor(),
				otelgrpc.UnaryServerInterceptor(),
				logger.UnaryServerInterceptor(),
			),
		),
	)

	server := &CachedServer{
		Grpc: grpcServer,
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

func (s *CachedServer) Serve(socketPath string) error {
	u, err := url.Parse(socketPath)
	if err != nil {
		return fmt.Errorf("unable to parse socket address: %q", err)
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
