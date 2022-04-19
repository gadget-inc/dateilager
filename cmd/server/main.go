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

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/server"
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

func buildLogger(env environment.Env, level zapcore.Level, encoding string) (log *zap.Logger, err error) {
	var config zap.Config
	if env == environment.Prod {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}

	config.Encoding = encoding
	config.Level = zap.NewAtomicLevelAt(level)

	return config.Build()
}

func main() {
	ctx := context.Background()
	args := parseArgs()

	env, err := environment.LoadEnvironment()
	if err != nil {
		stdlog.Fatal(err.Error())
	}

	log, err := buildLogger(env, args.level, args.encoding)
	if err != nil {
		stdlog.Fatal(err.Error())
	}
	defer log.Sync()

	if args.prof != "" {
		file, err := os.Create(args.prof)
		if err != nil {
			log.Fatal("open pprof file", zap.String("file", args.prof), zap.Error(err))
		}
		pprof.StartCPUProfile(file)
		defer pprof.StopCPUProfile()
	}

	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", args.port))
	if err != nil {
		log.Fatal("failed to listen", zap.String("protocol", "tcp"), zap.Int("port", args.port), zap.Error(err))
	}

	dbConn, err := server.NewDbPoolConnector(ctx, args.dbUri)
	if err != nil {
		log.Fatal("cannot connect to DB", zap.String("dburi", args.dbUri), zap.Error(err))
	}
	defer dbConn.Close()

	cert, err := tls.LoadX509KeyPair(args.certFile, args.keyFile)
	if err != nil {
		log.Fatal("cannot open TLS cert and key files", zap.String("cert", args.certFile), zap.String("key", args.keyFile), zap.Error(err))
	}

	pasetoKey, err := parsePublicKey(args.pasetoFile)
	if err != nil {
		log.Fatal("cannot parse Paseto public key", zap.String("path", args.pasetoFile), zap.Error(err))
	}

	s := server.NewServer(ctx, log, dbConn, &cert, pasetoKey)

	log.Info("register Fs")
	fs := &api.Fs{
		Env:    env,
		Log:    log,
		DbConn: dbConn,
	}
	s.RegisterFs(ctx, fs)

	osSignals := make(chan os.Signal)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-osSignals
		s.Grpc.Stop()
		if args.prof != "" {
			pprof.StopCPUProfile()
		}
		os.Exit(0)
	}()

	log.Info("start server", zap.Int("port", args.port), zap.String("env", env.String()))
	if err := s.Serve(listen); err != nil {
		log.Fatal("failed to serve", zap.Error(err))
	}
}
