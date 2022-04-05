package db

import (
	"context"

	"github.com/jackc/pgx/v4"
)

type CloseFunc func(ctx context.Context)

type DbConnector interface {
	Connect(context.Context) (pgx.Tx, CloseFunc, error)
}
