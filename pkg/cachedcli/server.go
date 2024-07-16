package cachedcli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/cached"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
)

func NewCacheDaemonCommand() *cobra.Command {
	var (
		profilerEnabled   bool = false
		shutdownTelemetry func()
	)

	var (
		level        *zapcore.Level
		encoding     string
		tracing      bool
		profilePath  string
		upstreamHost string
		upstreamPort uint16
		healthzPort  uint16
		timeout      uint
		headlessHost string
		stagingPath  string
		csiSocket    string
	)

	cmd := &cobra.Command{
		Use:               "cached",
		Short:             "DateiLager cache daemon",
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

			if profilePath != "" {
				file, err := os.Create(profilePath)
				if err != nil {
					return fmt.Errorf("cannot open profile path %s: %w", profilePath, err)
				}
				_ = pprof.StartCPUProfile(file)
				profilerEnabled = true
			}

			if tracing {
				shutdownTelemetry = telemetry.Init(ctx, telemetry.Server)
			}

			cl, err := client.NewClient(ctx, upstreamHost, upstreamPort, client.WithheadlessHost(headlessHost))
			if err != nil {
				return err
			}

			s := cached.NewServer(ctx)

			cached := &api.Cached{
				Env:         env,
				Client:      cl,
				StagingPath: stagingPath,
			}

			logger.Info(ctx, "register Cached")
			s.RegisterCached(cached)

			logger.Info(ctx, "register CSI")
			s.RegisterCSI(cached)

			err = cached.Prepare(ctx)
			if err != nil {
				return fmt.Errorf("failed to prepare cache daemon in %s: %w", stagingPath, err)
			}

			group, ctx := errgroup.WithContext(ctx)

			osSignals := make(chan os.Signal, 1)
			signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
			group.Go(func() error {
				<-osSignals
				s.Grpc.GracefulStop()
				return nil
			})

			group.Go(func() error {
				logger.Info(ctx, "start cached server", key.Socket.Field(csiSocket))
				return s.Serve(csiSocket)
			})

			group.Go(func() error {
				mux := http.NewServeMux()
				mux.HandleFunc("/healthz", healthzHandler)

				healthServer := &http.Server{
					Addr:        fmt.Sprintf(":%d", healthzPort),
					Handler:     mux,
					BaseContext: func(l net.Listener) context.Context { return ctx },
				}
				return healthServer.ListenAndServe()
			})

			return group.Wait()
		},
		PostRunE: func(cmd *cobra.Command, _ []string) error {
			if shutdownTelemetry != nil {
				shutdownTelemetry()
			}

			if profilerEnabled {
				pprof.StopCPUProfile()
			}

			return nil
		},
	}

	flags := cmd.PersistentFlags()

	level = zap.LevelFlag("log-level", zap.DebugLevel, "Log level")
	flags.AddGoFlag(flag.CommandLine.Lookup("log-level"))
	flags.StringVar(&encoding, "log-encoding", "console", "Log encoding (console | json)")
	flags.BoolVar(&tracing, "tracing", false, "Whether tracing is enabled")
	flags.StringVar(&profilePath, "profile", "", "CPU profile output path (profiling enabled if set)")

	flags.StringVar(&upstreamHost, "upstream-host", "localhost", "GRPC server hostname")
	flags.Uint16Var(&upstreamPort, "upstream-port", 5051, "GRPC server port")
	flags.StringVar(&headlessHost, "headless-host", "", "Alternative headless hostname to use for round robin connections")
	flags.Uint16Var(&healthzPort, "healthz-port", 5053, "Healthz HTTP port")
	flags.UintVar(&timeout, "timeout", 0, "GRPC client timeout (ms)")

	flags.StringVar(&csiSocket, "csi-socket", "", "path for running the Kubernetes CSI Driver interface")
	flags.StringVar(&stagingPath, "staging-path", "", "path for staging downloaded caches")

	_ = cmd.MarkPersistentFlagRequired("csi-socket")
	_ = cmd.MarkPersistentFlagRequired("staging-path")

	return cmd
}

func CacheDaemonExecute() {
	ctx := context.Background()
	cmd := NewCacheDaemonCommand()

	err := cmd.ExecuteContext(ctx)

	logger.Info(ctx, "shut down server")
	_ = logger.Sync(ctx)

	if err != nil {
		logger.Fatal(ctx, "server failed", zap.Error(err))
	}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type response struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	resp := &response{}

	if ctx.Err() == nil {
		w.WriteHeader(http.StatusOK)
		resp.Status = "healthy"
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		resp.Status = "error"
		resp.Error = ctx.Err().Error()
	}

	data, err := json.MarshalIndent(&resp, "", "  ")
	if err != nil {
		logger.Error(ctx, "failed to marshal healthz response", zap.Error(err))
	}
	_, err = w.Write(data)
	if err != nil {
		logger.Error(ctx, "failed to write healthz response", zap.Error(err))
	}
}
