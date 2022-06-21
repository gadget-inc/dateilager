package cli

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/gadget-inc/dateilager/pkg/web"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewWebuiCommand() *cobra.Command {
	var cl *client.Client

	var (
		level     *zapcore.Level
		encoding  string
		port      int
		server    string
		assetsDir string
	)

	cmd := &cobra.Command{
		Use:               "webui",
		Short:             "DateiLager webui",
		DisableAutoGenTag: true,
		Version:           version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true // silence usage when an error occurs after flags have been parsed

			env, err := environment.LoadEnvironment()
			if err != nil {
				return fmt.Errorf("could not load environment: %w", err)
			}

			var config zap.Config
			if env == environment.Prod {
				config = zap.NewProductionConfig()
			} else {
				config = zap.NewDevelopmentConfig()
			}

			config.Encoding = encoding
			config.Level = zap.NewAtomicLevelAt(*level)
			config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

			err = logger.Init(config)
			if err != nil {
				return fmt.Errorf("could not initialize logger: %w", err)
			}

			ctx := cmd.Context()

			cl, err = client.NewClient(ctx, server)
			if err != nil {
				return fmt.Errorf("could not connect to server %s: %w", server, err)
			}

			handler, err := web.NewWebServer(ctx, cl, assetsDir)
			if err != nil {
				return fmt.Errorf("cannot setup web server: %w", err)
			}

			logger.Info(ctx, "start webui", zap.Int("port", port), zap.String("assets", assetsDir))
			return http.ListenAndServe(":"+strconv.Itoa(port), handler)
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			if cl != nil {
				cl.Close()
			}

			return nil
		},
	}

	flags := cmd.PersistentFlags()

	level = zap.LevelFlag("log-level", zap.DebugLevel, "Log level")
	flags.AddGoFlag(flag.CommandLine.Lookup("log-level"))
	flags.StringVar(&encoding, "log-encoding", "console", "Log encoding (console | json)")

	flags.IntVar(&port, "port", 3333, "Web UI port")
	flags.StringVar(&server, "server", "localhost:5051", "GRPC server address and port")
	flags.StringVar(&assetsDir, "assets", "assets", "Assets directory")

	return cmd
}

func WebUIExecute() {
	ctx := context.Background()
	cmd := NewWebuiCommand()

	err := cmd.ExecuteContext(ctx)

	_ = logger.Sync()

	if err != nil {
		logger.Fatal(ctx, "command failed", zap.Error(err))
	}
}
