package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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

func NewClientCommand() *cobra.Command {
	var (
		level        *zapcore.Level
		encoding     string
		tracing      bool
		otelContext  string
		host         string
		port         uint16
		timeout      uint16
		headlessHost string
	)

	var cancel context.CancelFunc

	cmd := &cobra.Command{
		Use:               "client",
		Short:             "DateiLager client",
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

			fmt.Fprintf(os.Stderr, "timeout: %v\n", timeout)
			if timeout != 0 {
				ctx, cancel = context.WithTimeout(cmd.Context(), time.Duration(timeout)*time.Second)
				fmt.Fprintf(os.Stderr, "duration: %v\n", time.Duration(timeout)*time.Second)
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

			ctx, span = telemetry.Start(ctx, "cmd.main")

			if host == "" {
				return fmt.Errorf("required flag(s) \"host\" not set")
			}

			cl, err := client.NewClient(ctx, host, port, client.WithheadlessHost(headlessHost))
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

	level = zap.LevelFlag("log-level", zap.DebugLevel, "Log level")
	flags.AddGoFlag(flag.CommandLine.Lookup("log-level"))
	flags.StringVar(&encoding, "log-encoding", "console", "Log encoding (console | json)")
	flags.BoolVar(&tracing, "tracing", false, "Whether tracing is enabled")
	flags.StringVar(&otelContext, "otel-context", "", "Open Telemetry context")
	flags.StringVar(&host, "host", "", "GRPC server hostname")
	flags.Uint16Var(&port, "port", 5051, "GRPC server port")
	flags.Uint16Var(&timeout, "timeout", 0, "GRPC client timeout")
	flags.StringVar(&headlessHost, "headless-host", "", "Alternative headless hostname to use for round robin connections")

	cmd.AddCommand(NewCmdGet())
	cmd.AddCommand(NewCmdInspect())
	cmd.AddCommand(NewCmdNew())
	cmd.AddCommand(NewCmdRebuild())
	cmd.AddCommand(NewCmdReset())
	cmd.AddCommand(NewCmdSnapshot())
	cmd.AddCommand(NewCmdUpdate())
	cmd.AddCommand(NewCmdCommit())
	cmd.AddCommand(NewCmdGc())
	cmd.AddCommand(NewCmdGetCache())

	return cmd
}

func ClientExecute() {
	ctx := context.Background()
	cmd := NewClientCommand()
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

	_ = logger.Sync()

	if err != nil {
		logger.Fatal(ctx, "command failed", zap.Error(err))
	}
}
