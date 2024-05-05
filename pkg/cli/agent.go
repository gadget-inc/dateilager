package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/agent"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewCmdAgent() *cobra.Command {
	var (
		dir  string
		port int
	)

	cmd := &cobra.Command{
		Use: "agent",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c := client.FromContext(ctx)

			// Wait for the server to be available
			version := int64(-1)
			var err error

			for i := 0; i < 20; i++ {
				version, err = c.GetCache(ctx, dir)
				if err != nil {
					logger.Error(ctx, "get cache err", zap.Error(err))
					parentErr := errors.Unwrap(err)
					statusErr, ok := status.FromError(parentErr)
					if ok {
						if statusErr.Code() == codes.Unavailable {
							time.Sleep(3 * time.Second)
							continue
						}
					}
					return err
				}
				break
			}

			if err != nil {
				return err
			}

			logger.Info(ctx, "cache downloaded", key.Version.Field(version), key.Directory.Field(dir))

			a := agent.NewAgent("/home/main/varlib", dir, port)

			backgroundCtx, cancel := context.WithCancel(ctx)
			server := a.Server(backgroundCtx)

			osSignals := make(chan os.Signal, 1)
			signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)

			go func() {
				<-osSignals
				logger.Info(ctx, "received interrupt signal")

				cancel()

				err := server.Shutdown(ctx)
				if err != nil {
					logger.Error(ctx, "error shutting down server", zap.Error(err))
				}
			}()

			logger.Info(ctx, "start agent", zap.Int("port", port), key.Directory.Field(dir))
			return server.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Cache directory")
	cmd.Flags().IntVar(&port, "port", 8080, "API server port")

	_ = cmd.MarkFlagRequired("path")

	return cmd
}
