package cli

import (
	"context"
	"flag"
	"fmt"

	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewCachedCommand() *cobra.Command {
	var (
		logLevel      *zapcore.Level
		logEncoding   string
		enableTracing bool
		upstreamHost  string
		upstreamPort  uint16
	)

	var cancel context.CancelFunc

	cmd := &cobra.Command{
		Use:               "cached",
		Short:             "DateiLager cache daemon",
		DisableAutoGenTag: true,
		Version:           version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceErrors = true // silence cobra errors and usage after flags have been parsed and validated
			cmd.SilenceUsage = true

			err := logger.Init(environment.LoadOrProduction(), logEncoding, zap.NewAtomicLevelAt(*logLevel))
			if err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}

			ctx := cmd.Context()

			if enableTracing {
				shutdownTelemetry = telemetry.Init(ctx, telemetry.Client)
			}

			cl, err := client.NewClient(ctx, upstreamHost, upstreamPort)
			if err != nil {
				return err
			}

			ctx = client.IntoContext(ctx, cl)

			cmd.SetContext(ctx)

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	}

	flags := cmd.PersistentFlags()

	logLevel = zap.LevelFlag("log-level", zap.InfoLevel, "Log level")
	flags.AddGoFlag(flag.CommandLine.Lookup("log-level"))
	flags.StringVar(&logEncoding, "log-encoding", "console", "Log encoding (console | json)")
	flags.BoolVar(&enableTracing, "tracing", false, "Whether tracing is enabled")
	flags.StringVar(&upstreamHost, "upstream-host", "", "Upstream dateilager server hostname")
	flags.Uint16Var(&upstreamPort, "upstream-port", 5051, "Upstream dateilager server port")

	_ = cmd.MarkFlagRequired("upstream-host")
	_ = cmd.MarkFlagRequired("upstream-port")

	cmd.AddCommand(NewCachedPrepareCommand())
	cmd.AddCommand(NewCachedServerCommand())

	return cmd
}

func CachedExecute() {
	ctx := context.Background()
	cmd := NewCachedCommand()
	err := cmd.ExecuteContext(ctx)

	client := client.FromContext(cmd.Context())
	if client != nil {
		client.Close()
	}

	if shutdownTelemetry != nil {
		shutdownTelemetry()
	}

	_ = logger.Sync(ctx)

	if err != nil {
		logger.Fatal(ctx, "command failed", zap.Error(err))
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
