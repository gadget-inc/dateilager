package cachedcli

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

func NewCachedClientCommand() *cobra.Command {
	var (
		level       *zapcore.Level
		encoding    string
		tracing     bool
		otelContext string
		socket      string
		timeout     uint
	)

	var cancel context.CancelFunc

	cmd := &cobra.Command{
		Use:               "cachedclient",
		Short:             "DateiLager cached client",
		DisableAutoGenTag: true,
		Version:           version.Version,
		SilenceErrors:     true,
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

			if timeout != 0 {
				ctx, cancel = context.WithTimeout(cmd.Context(), time.Duration(timeout)*time.Millisecond)
			}

			if tracing {
				shutdownTelemetry = telemetry.Init(ctx, telemetry.Client)
			}

			if otelContext != "" {
				var mapCarrier propagation.MapCarrier
				err := json.NewDecoder(strings.NewReader(otelContext)).Decode(&mapCarrier)
				if err != nil {
					return fmt.Errorf("failed to decode otel-context: %w", err)
				}

				ctx = otel.GetTextMapPropagator().Extract(ctx, mapCarrier)
			}

			ctx, span = telemetry.Start(ctx, "cached-cmd.main")

			if socket == "" {
				return fmt.Errorf("required flag(s) \"socket\" not set")
			}

			cl, err := client.NewCachedUnixClient(ctx, socket)
			if err != nil {
				return err
			}
			ctx = client.CachedIntoContext(ctx, cl)

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

	level = zap.LevelFlag("log-level", zap.DebugLevel, "Log level")
	flags.AddGoFlag(flag.CommandLine.Lookup("log-level"))
	flags.StringVar(&encoding, "log-encoding", "console", "Log encoding (console | json)")
	flags.BoolVar(&tracing, "tracing", false, "Whether tracing is enabled")
	flags.StringVar(&otelContext, "otel-context", "", "Open Telemetry context")

	flags.StringVar(&socket, "socket", "", "Unix domain socket path")
	flags.UintVar(&timeout, "timeout", 0, "GRPC client timeout (ms)")

	_ = cmd.MarkFlagRequired("socket")

	cmd.AddCommand(NewCmdPopulate())
	cmd.AddCommand(NewCmdProbe())

	return cmd
}

func ClientExecute() {
	ctx := context.Background()
	cmd := NewCachedClientCommand()
	err := cmd.ExecuteContext(ctx)

	client := client.FromContext(cmd.Context())
	if client != nil {
		client.Close()
	}

	if span != nil {
		span.End()
	}

	if shutdownTelemetry != nil {
		shutdownTelemetry()
	}

	_ = logger.Sync(ctx)

	if err != nil {
		logger.Fatal(ctx, "cached client failed", zap.Error(err))
	}
}
