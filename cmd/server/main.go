package main

import (
	"context"
	"flag"
	"fmt"
	"net"

	"go.uber.org/zap"

	"github.com/angelini/dateilager/pkg/api"
	"github.com/angelini/dateilager/pkg/server"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type DbPoolConnector struct {
	pool *pgxpool.Pool
}

func (d *DbPoolConnector) Connect(ctx context.Context) (pgx.Tx, api.CloseFunc, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	return tx, func() { tx.Rollback(ctx); conn.Release() }, nil
}

type ServerArgs struct {
	port  int
	dbUri string
}

func parseArgs(log *zap.Logger) ServerArgs {
	port := flag.Int("port", 5051, "GRPC server port")
	dbUri := flag.String("dburi", "127.0.0.1:5432", "Postgres URI")

	flag.Parse()

	return ServerArgs{
		port:  *port,
		dbUri: *dbUri,
	}
}

func main() {
	ctx := context.Background()
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	args := parseArgs(log)

	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", args.port))
	if err != nil {
		log.Fatal("failed to listen", zap.String("protocol", "tcp"), zap.Int("port", args.port))
	}

	pool, err := pgxpool.Connect(ctx, args.dbUri)
	if err != nil {
		log.Fatal("cannot connect to DB", zap.String("dburi", args.dbUri))
	}
	defer pool.Close()

	s := server.NewServer(log)
	s.MonitorDbPool(ctx, pool)

	log.Info("register Fs")
	fs := &api.Fs{
		Log:    log,
		DbConn: &DbPoolConnector{pool: pool},
	}
	s.RegisterFs(ctx, fs)

	log.Info("start server", zap.Int("port", args.port))
	if err := s.Serve(listen); err != nil {
		log.Fatal("failed to serve", zap.Error(err))
	}
}
