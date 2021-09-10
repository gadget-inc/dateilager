package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/server"
)

type ServerArgs struct {
	port     int
	dbUri    string
	certFile string
	keyFile  string
	prof     string
	level    zapcore.Level
}

func parseArgs() ServerArgs {
	port := flag.Int("port", 5051, "GRPC server port")
	dbUri := flag.String("dburi", "postgres://postgres@127.0.0.1:5432/dl", "Postgres URI")
	certFile := flag.String("cert", "dev/server.crt", "TLS cert file")
	keyFile := flag.String("key", "dev/server.key", "TLS key file")
	prof := flag.String("prof", "", "Output CPU profile to this path")

	level := zap.LevelFlag("log", zap.DebugLevel, "Set the log level")

	flag.Parse()

	return ServerArgs{
		port:     *port,
		dbUri:    *dbUri,
		certFile: *certFile,
		keyFile:  *keyFile,
		prof:     *prof,
		level:    *level,
	}
}

func main() {
	ctx := context.Background()
	args := parseArgs()
	log, _ := zap.NewDevelopment(zap.IncreaseLevel(args.level))
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
		log.Fatal("failed to listen", zap.String("protocol", "tcp"), zap.Int("port", args.port))
	}

	dbConn, err := server.NewDbPoolConnector(ctx, args.dbUri)
	if err != nil {
		log.Fatal("cannot connect to DB", zap.String("dburi", args.dbUri))
	}
	defer dbConn.Close()

	cert, err := tls.LoadX509KeyPair(args.certFile, args.keyFile)
	if err != nil {
		log.Fatal("cannot open TLS cert and key files", zap.String("cert", args.certFile), zap.String("key", args.keyFile), zap.Error(err))
	}

	s, err := server.NewServer(ctx, log, dbConn, &cert)
	if err != nil {
		log.Fatal("cannot start server", zap.Error(err))
	}

	log.Info("register Fs")
	fs := &api.Fs{
		Env:    s.Env,
		Log:    log,
		DbConn: dbConn,
	}
	s.RegisterFs(ctx, fs)

	osSignals := make(chan os.Signal)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-osSignals
		s.Grpc.Stop()
		os.Exit(0)
	}()

	log.Info("start server", zap.Int("port", args.port), zap.String("env", s.Env.String()))
	if err := s.Serve(listen); err != nil {
		log.Fatal("failed to serve", zap.Error(err))
	}
}
