package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type CloseFunc func(context.Context)

type DbConnector interface {
	Connect(context.Context) (pgx.Tx, CloseFunc, error)
	TransactionlessConnect(context.Context) (*pgx.Conn, error)
}
