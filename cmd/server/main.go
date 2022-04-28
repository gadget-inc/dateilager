package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	stdlog "log"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/server"
	"github.com/gadget-inc/dateilager/pkg/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ServerArgs struct {
	port       int
	dbUri      string
	certFile   string
	keyFile    string
	pasetoFile string
	prof       string
	level      zapcore.Level
	encoding   string
	tracing    bool
}

func parseArgs() ServerArgs {
	port := flag.Int("port", 5051, "GRPC server port")
	dbUri := flag.String("dburi", "postgres://postgres@127.0.0.1:5432/dl", "Postgres URI")
	certFile := flag.String("cert", "dev/server.crt", "TLS cert file")
	keyFile := flag.String("key", "dev/server.key", "TLS key file")
	pasetoFile := flag.String("paseto", "dev/paseto.pub", "Paseto public key file")
	prof := flag.String("prof", "", "Output CPU profile to this path")

	level := zap.LevelFlag("log", zap.DebugLevel, "Log level")
	encoding := flag.String("encoding", "console", "Log encoding (console | json)")
	tracing := flag.Bool("tracing", false, "Whether tracing is enabled")

	flag.Parse()

	return ServerArgs{
		port:       *port,
		dbUri:      *dbUri,
		certFile:   *certFile,
		keyFile:    *keyFile,
		pasetoFile: *pasetoFile,
		prof:       *prof,
		level:      *level,
		encoding:   *encoding,
		tracing:    *tracing,
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

func initLogger(env environment.Env, level zapcore.Level, encoding string) error {
	var config zap.Config
	if env == environment.Prod {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}

	config.Encoding = encoding
	config.Level = zap.NewAtomicLevelAt(level)

	return logger.Init(config)
}

func main() {
	if os.Args[1] == "version" {
		fmt.Println(version.Version)
		return
	}

	ctx := context.Background()
	args := parseArgs()

	env, err := environment.LoadEnvironment()
	if err != nil {
		stdlog.Fatal(err.Error())
	}

	err = initLogger(env, args.level, args.encoding)
	if err != nil {
		stdlog.Fatal(err.Error())
	}
	defer logger.Sync()

	if args.prof != "" {
		file, err := os.Create(args.prof)
		if err != nil {
			logger.Fatal(ctx, "open pprof file", zap.String("file", args.prof), zap.Error(err))
		}
		pprof.StartCPUProfile(file)
		defer pprof.StopCPUProfile()
	}

	var shutdown func()
	if args.tracing {
		shutdown, err = telemetry.Init(ctx, telemetry.Server)
		if err != nil {
			logger.Fatal(ctx, "could not initialize telemetry", zap.Error(err))
		}
		defer shutdown()
	}

	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", args.port))
	if err != nil {
		logger.Fatal(ctx, "failed to listen", zap.String("protocol", "tcp"), zap.Int("port", args.port), zap.Error(err))
	}

	dbConn, err := server.NewDbPoolConnector(ctx, args.dbUri)
	if err != nil {
		logger.Fatal(ctx, "cannot connect to DB", zap.String("dburi", args.dbUri), zap.Error(err))
	}
	defer dbConn.Close()

	cert, err := tls.LoadX509KeyPair(args.certFile, args.keyFile)
	if err != nil {
		logger.Fatal(ctx, "cannot open TLS cert and key files", zap.String("cert", args.certFile), zap.String("key", args.keyFile), zap.Error(err))
	}

	pasetoKey, err := parsePublicKey(args.pasetoFile)
	if err != nil {
		logger.Fatal(ctx, "cannot parse Paseto public key", zap.String("path", args.pasetoFile), zap.Error(err))
	}

	s := server.NewServer(ctx, dbConn, &cert, pasetoKey)

	logger.Info(ctx, "register Fs")
	fs := &api.Fs{
		Env:    env,
		DbConn: dbConn,
	}
	s.RegisterFs(ctx, fs)

	osSignals := make(chan os.Signal)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-osSignals
		s.Grpc.Stop()
		dbConn.Close()
		if shutdown != nil {
			shutdown()
		}
		if args.prof != "" {
			pprof.StopCPUProfile()
		}
		logger.Sync()
		os.Exit(0)
	}()

	logger.Info(ctx, "start server", key.Port.Field(args.port), key.Environment.Field(env.String()))
	if err := s.Serve(listen); err != nil {
		logger.Fatal(ctx, "failed to serve", zap.Error(err))
	}
}
