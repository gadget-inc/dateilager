package testutil

import (
	"context"
	"fmt"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/jackc/pgx/v5"
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

	for _, typeName := range []string{"hash", "hash[]"} {
		extraType, err := conn.LoadType(ctx, typeName)
		if err != nil {
			return nil, fmt.Errorf("could not load type %s: %w", typeName, err)
		}
		conn.TypeMap().RegisterType(extraType)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	return &DbTestConnector{conn: conn, tx: tx}, nil
}

func (d *DbTestConnector) Connect(ctx context.Context) (pgx.Tx, db.CloseFunc, error) {
	innerTx, err := d.tx.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	return innerTx, func(context.Context) {}, nil
}

func (d *DbTestConnector) TransactionlessConnect(ctx context.Context) (*pgx.Conn, error) {
	return d.conn, nil
}

func (d *DbTestConnector) close(ctx context.Context) {
	_ = d.tx.Rollback(ctx)
	d.conn.Close(ctx)
}
