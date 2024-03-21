package testutil

import (
	"context"
	"fmt"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DbTestConnector struct {
	conn    *pgx.Conn
	tx      pgx.Tx
	innerTx pgx.Tx
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
	d.innerTx = innerTx
	return innerTx, func(context.Context) {}, nil
}

func (d *DbTestConnector) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if d.innerTx != nil {
		return d.innerTx.Query(ctx, sql, args...)
	} else {
		return d.tx.Query(ctx, sql, args...)
	}
}

func (d *DbTestConnector) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if d.innerTx != nil {
		return d.innerTx.Exec(ctx, sql, args...)
	} else {
		return d.tx.Exec(ctx, sql, args...)
	}
}

func (d *DbTestConnector) close(ctx context.Context) {
	_ = d.tx.Rollback(ctx)
	d.conn.Close(ctx)
}
