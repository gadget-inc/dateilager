package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type CloseFunc func(context.Context)

type DbConnector interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Connect(context.Context) (pgx.Tx, CloseFunc, error)
}
