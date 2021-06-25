package db

import (
	"context"

	"github.com/jackc/pgx/v4"
)

type CloseFunc func()

type DbConnector interface {
	Connect(context.Context) (pgx.Tx, CloseFunc, error)
}
