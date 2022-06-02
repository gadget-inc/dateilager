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
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewRootCommand() *cobra.Command {
	var (
		level    *zapcore.Level
		encoding string
	)

	cmd := &cobra.Command{
		Use:               "client",
		Short:             "DateiLager client",
		DisableAutoGenTag: true,
		Version:           version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true // silence usage when an error occurs after flags have been parsed
			cmd.SilenceErrors = true

			err := initLogger(*level, encoding)
			if err != nil {
				return fmt.Errorf("could not initialize logger: %w", err)
			}

			return nil
		},
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			_ = logger.Sync()
		},
	}

	level = zap.LevelFlag("log", zap.DebugLevel, "Log level")
	cmd.PersistentFlags().AddGoFlag(flag.CommandLine.Lookup("log"))

	cmd.PersistentFlags().StringVar(&encoding, "encoding", "console", "Log encoding (console | json)")
	_ = cmd.PersistentFlags().Bool("tracing", false, "Whether tracing is enabled")
	_ = cmd.PersistentFlags().String("otel-context", "", "Open Telemetry context")
	server := cmd.PersistentFlags().String("server", "", "Server GRPC address")

	cmd.AddCommand(NewCmdGet(server))
	cmd.AddCommand(NewCmdInspect(server))
	cmd.AddCommand(NewCmdNew(server))
	cmd.AddCommand(NewCmdRebuild(server))
	cmd.AddCommand(NewCmdReset(server))
	cmd.AddCommand(NewCmdSnapshot(server))
	cmd.AddCommand(NewCmdUpdate(server))

	return cmd
}

func initLogger(level zapcore.Level, encoding string) error {
	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.Level = zap.NewAtomicLevelAt(level)
	config.Encoding = encoding

	return logger.Init(config)
}

func Execute() {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	cmd := NewRootCommand()

	_ = cmd.Flags().Parse(os.Args[1:])

	tracing, _ := cmd.Flags().GetBool("tracing")
	if tracing {
		shutdown, err := telemetry.Init(ctx, telemetry.Client)
		if err != nil {
			logger.Fatal(ctx, "could not initialize telemetry", zap.Error(err))
		}
		defer shutdown()
	}

	otelContext, _ := cmd.Flags().GetString("otel-context")
	if otelContext != "" {
		var mapCarrier propagation.MapCarrier
		err := json.NewDecoder(strings.NewReader(otelContext)).Decode(&mapCarrier)
		if err != nil {
			logger.Fatal(ctx, "failed to decode otel-context", zap.Error(err))
		}

		ctx = otel.GetTextMapPropagator().Extract(ctx, mapCarrier)
	}

	ctx, span := telemetry.Start(ctx, "cmd.main")
	defer span.End()

	err := cmd.ExecuteContext(ctx)
	if err != nil {
		logger.Fatal(ctx, "command failed", zap.Error(err))
	}
}
