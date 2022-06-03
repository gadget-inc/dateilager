package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	shutdownTelemetry func()
	span              trace.Span
)

func NewRootCommand() *cobra.Command {
	var (
		level       *zapcore.Level
		tracing     bool
		otelContext string
		encoding    string
		server      string
	)

	cmd := &cobra.Command{
		Use:               "client",
		Short:             "DateiLager client",
		DisableAutoGenTag: true,
		Version:           version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true // silence usage when an error occurs after flags have been parsed

			config := zap.NewProductionConfig()
			config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
			config.Level = zap.NewAtomicLevelAt(*level)
			config.Encoding = encoding

			err := logger.Init(config)
			if err != nil {
				return fmt.Errorf("could not initialize logger: %w", err)
			}

			ctx := cmd.Context()

			if tracing {
				shutdownTelemetry, err = telemetry.Init(ctx, telemetry.Client)
				if err != nil {
					return fmt.Errorf("could not initialize telemetry: %w", err)
				}
			}

			if otelContext != "" {
				var mapCarrier propagation.MapCarrier
				err := json.NewDecoder(strings.NewReader(otelContext)).Decode(&mapCarrier)
				if err != nil {
					return fmt.Errorf("failed to decode otel-context: %w", err)
				}

				ctx = otel.GetTextMapPropagator().Extract(ctx, mapCarrier)
			}

			ctx, span = telemetry.Start(ctx, "cmd.main")

			cl, err := client.NewClient(ctx, server)
			if err != nil {
				return err
			}

			ctx = client.IntoContext(ctx, cl)

			cmd.SetContext(ctx)

			return nil
		},
	}

	level = zap.LevelFlag("log-level", zap.DebugLevel, "Log level")
	cmd.PersistentFlags().AddGoFlag(flag.CommandLine.Lookup("log-level"))

	cmd.PersistentFlags().StringVar(&encoding, "log-encoding", "console", "Log encoding (console | json)")
	cmd.PersistentFlags().BoolVar(&tracing, "tracing", false, "Whether tracing is enabled")
	cmd.PersistentFlags().StringVar(&otelContext, "otel-context", "", "Open Telemetry context")
	cmd.PersistentFlags().StringVar(&server, "server", "", "Server GRPC address")

	cmd.AddCommand(NewCmdGet())
	cmd.AddCommand(NewCmdInspect())
	cmd.AddCommand(NewCmdNew())
	cmd.AddCommand(NewCmdRebuild())
	cmd.AddCommand(NewCmdReset())
	cmd.AddCommand(NewCmdSnapshot())
	cmd.AddCommand(NewCmdUpdate())

	return cmd
}

func Execute() {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	rootCmd := NewRootCommand()

	err := rootCmd.ExecuteContext(ctx)

	client := client.FromContext(rootCmd.Context())
	if client != nil {
		client.Close()
	}

	if span != nil {
		span.End()
	}

	if shutdownTelemetry != nil {
		shutdownTelemetry()
	}

	_ = logger.Sync()

	if err != nil {
		logger.Fatal(ctx, "command failed", zap.Error(err))
	}
}
