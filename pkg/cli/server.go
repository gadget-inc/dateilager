package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/server"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewServerCommand() *cobra.Command {
	var (
		profilerEnabled   bool = false
		shutdownTelemetry func()
	)

	var (
		level          *zapcore.Level
		encoding       string
		tracing        bool
		profilePath    string
		memProfilePath string
		port           int
		dbUri          string
		certFile       string
		keyFile        string
		pasetoFile     string
	)

	cmd := &cobra.Command{
		Use:               "server",
		Short:             "DateiLager server",
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

			listen, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				return fmt.Errorf("failed to listen on TCP port %d: %w", port, err)
			}

			dbConn, err := server.NewDbPoolConnector(ctx, dbUri)
			if err != nil {
				return fmt.Errorf("cannot connect to DB %s: %w", dbUri, err)
			}
			defer dbConn.Close()

			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return fmt.Errorf("cannot open TLS cert and key files (%s, %s): %w", certFile, keyFile, err)
			}

			pasetoKey, err := parsePublicKey(pasetoFile)
			if err != nil {
				return fmt.Errorf("cannot parse Paseto public key %s: %w", pasetoFile, err)
			}

			contentLookup, err := db.NewContentLookup()
			if err != nil {
				return fmt.Errorf("cannot setup content lookup: %w", err)
			}

			s := server.NewServer(ctx, dbConn, &cert, pasetoKey)
			logger.Info(ctx, "register Fs")
			fs := &api.Fs{
				Env:           env,
				DbConn:        dbConn,
				ContentLookup: contentLookup,
			}
			s.RegisterFs(fs)

			osSignals := make(chan os.Signal, 1)
			signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-osSignals
				s.Grpc.GracefulStop()
			}()

			if memProfilePath != "" {
				memSnapshotSignals := make(chan os.Signal, 1)
				signal.Notify(memSnapshotSignals, syscall.SIGUSR2)

				go func() {
					for {
						<-memSnapshotSignals

						logger.Info(ctx, "SIGUSR2 received, building heap profile", zap.String("path", memProfilePath))

						memProfile, err := os.Create(memProfilePath)
						if err != nil {
							logger.Error(ctx, "cannot create heap profile file", zap.Error(err), zap.String("path", memProfilePath))
						}

						runtime.GC()

						err = pprof.WriteHeapProfile(memProfile)
						if err != nil {
							logger.Error(ctx, "cannot write heap profile", zap.Error(err), zap.String("path", memProfilePath))
						}

						memProfile.Close()
					}
				}()
			}

			logger.Info(ctx, "start fs server", key.Port.Field(port), key.Environment.Field(env.String()))
			return s.Serve(listen)
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
	flags.StringVar(&profilePath, "profile", "", "CPU profile output path (CPU profiling enabled if set)")
	flags.StringVar(&memProfilePath, "memprofile", "mem.pb.gz", "Memory profile output path")

	flags.IntVar(&port, "port", 5051, "GRPC server port")
	flags.StringVar(&dbUri, "dburi", "postgres://postgres@127.0.0.1:5432/dl", "Postgres URI")
	flags.StringVar(&certFile, "cert", "development/server.crt", "TLS cert file")
	flags.StringVar(&keyFile, "key", "development/server.key", "TLS key file")
	flags.StringVar(&pasetoFile, "paseto", "development/paseto.pub", "Paseto public key file")

	return cmd
}

func ServerExecute() {
	ctx := context.Background()
	cmd := NewServerCommand()

	err := cmd.ExecuteContext(ctx)

	logger.Info(ctx, "shut down server")
	_ = logger.Sync(ctx)

	if err != nil {
		logger.Fatal(ctx, "server failed", zap.Error(err))
	}
}

func parsePublicKey(path string) (ed25519.PublicKey, error) {
	pubKeyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open Paseto public key file: %w", err)
	}

	block, _ := pem.Decode(pubKeyBytes)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("error decoding Paseto public key PEM")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing Paseto public key: %w", err)
	}

	return pub.(ed25519.PublicKey), nil
}
