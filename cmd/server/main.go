package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"go.uber.org/zap"

	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/server"
)

type ServerArgs struct {
	port  int
	dbUri string
	prof  string
}

func parseArgs(log *zap.Logger) ServerArgs {
	port := flag.Int("port", 5051, "GRPC server port")
	dbUri := flag.String("dburi", "127.0.0.1:5432", "Postgres URI")
	prof := flag.String("prof", "", "Output CPU profile to this path")

	flag.Parse()

	return ServerArgs{
		port:  *port,
		dbUri: *dbUri,
		prof:  *prof,
	}
}

func main() {
	ctx := context.Background()
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	args := parseArgs(log)

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

	pool, err := server.NewDbPoolConnector(ctx, args.dbUri)
	if err != nil {
		log.Fatal("cannot connect to DB", zap.String("dburi", args.dbUri))
	}
	defer pool.Close()

	s := server.NewServer(log)
	s.MonitorDbPool(ctx, pool)

	log.Info("register Fs")
	fs := &api.Fs{
		Log:    log,
		DbConn: pool,
	}
	s.RegisterFs(ctx, fs)

	osSignals := make(chan os.Signal)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-osSignals
		s.Grpc.Stop()
		os.Exit(0)
	}()

	log.Info("start server", zap.Int("port", args.port))
	if err := s.Serve(listen); err != nil {
		log.Fatal("failed to serve", zap.Error(err))
	}
}
