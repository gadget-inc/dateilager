package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/testutil"
	dlc "github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewConnTestCommand() *cobra.Command {
	var (
		client *dlc.Client
		server string
	)

	cmd := &cobra.Command{
		Use:               "conn-test",
		Short:             "DateiLager connection test",
		DisableAutoGenTag: true,
		Version:           version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true // silence usage when an error occurs after flags have been parsed

			config := zap.NewDevelopmentConfig()
			config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

			err := logger.Init(config)
			if err != nil {
				return fmt.Errorf("could not initialize logger: %w", err)
			}

			ctx, cancel := context.WithCancel(cmd.Context())

			client, err = dlc.NewClient(ctx, server)
			if err != nil {
				cancel()
				return fmt.Errorf("could not connect to server %s: %w", server, err)
			}

			osSignals := make(chan os.Signal, 1)
			signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-osSignals
				cancel()
			}()

			return testutil.TestConnection(ctx, client)
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			if client != nil {
				client.Close()
			}

			return nil
		},
	}

	flags := cmd.PersistentFlags()

	flags.StringVar(&server, "server", "", "Server GRPC address")

	return cmd
}

func ConnTestExecute() {
	ctx := context.Background()
	cmd := NewConnTestCommand()

	err := cmd.ExecuteContext(ctx)

	if err != nil {
		logger.Fatal(ctx, "connection test failed", zap.Error(err))
	}

	logger.Info(ctx, "connection test complete")
	_ = logger.Sync()
}
