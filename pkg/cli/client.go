package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/environment"
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
		timeout      uint
		headlessHost string
	)

	var cancel context.CancelFunc

	cmd := &cobra.Command{
		Use:               "client",
		Short:             "DateiLager client",
		DisableAutoGenTag: true,
		Version:           version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceErrors = true // silence cobra errors and usage after flags have been parsed and validated
			cmd.SilenceUsage = true

			err := logger.Init(environment.LoadOrProduction(), encoding, zap.NewAtomicLevelAt(*level))
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
	flags.StringVar(&headlessHost, "headless-host", "", "Alternative headless hostname to use for round robin connections")
	flags.UintVar(&timeout, "timeout", 0, "GRPC client timeout (ms)")

	_ = cmd.MarkFlagRequired("host")

	cmd.AddCommand(NewCmdGet())
	cmd.AddCommand(NewCmdInspect())
	cmd.AddCommand(NewCmdNew())
	cmd.AddCommand(NewCmdRebuild())
	cmd.AddCommand(NewCmdReset())
	cmd.AddCommand(NewCmdSnapshot())
	cmd.AddCommand(NewCmdUpdate())
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

	_ = logger.Sync(ctx)

	if err != nil {
		logger.Fatal(ctx, "command failed", zap.Error(err))
	}
}
