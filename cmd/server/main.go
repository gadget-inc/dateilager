package main

import (
	"context"
	"net"
	"os"

	"go.uber.org/zap"

	"github.com/angelini/dateilager/pkg/api"
	"github.com/angelini/dateilager/pkg/server"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type DbPoolConnector struct {
	pool *pgxpool.Pool
}

func (d *DbPoolConnector) Connect(ctx context.Context) (*pgx.Conn, api.CancelFunc, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	return conn.Conn(), func() { conn.Release() }, nil
}

func main() {
	ctx := context.Background()

	log, _ := zap.NewDevelopment()
	defer log.Sync()

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("missing PORT env variable")
	}

	dbUri := os.Getenv("DB_URI")

	listen, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatal("failed to listen", zap.String("protocol", "tcp"), zap.String("port", port))
	}

	pool, err := pgxpool.Connect(ctx, dbUri)
	if err != nil {
		log.Fatal("cannot connect to DB", zap.String("uri", dbUri))
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

	log.Info("start server", zap.String("port", port))
	if err := s.Serve(listen); err != nil {
		log.Fatal("failed to serve", zap.Error(err))
	}
}
