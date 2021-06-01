package testutil

import (
	"context"

	"github.com/angelini/dateilager/pkg/api"
	"github.com/jackc/pgx/v4"
)

type DbTestConnector struct {
	conn *pgx.Conn
	tx   pgx.Tx
}

func newDbTestConnector(ctx context.Context, uri string) (*DbTestConnector, error) {
	conn, err := pgx.Connect(ctx, uri)
	if err != nil {
		return nil, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	return &DbTestConnector{conn: conn, tx: tx}, nil
}

func (d *DbTestConnector) Connect(ctx context.Context) (*pgx.Conn, api.CloseFunc, error) {
	return d.conn, func() {}, nil
}

func (d *DbTestConnector) close(ctx context.Context) {
	d.tx.Rollback(ctx)
	d.conn.Close(ctx)
}
