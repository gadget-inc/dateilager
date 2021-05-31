package api

import (
	"context"

	"github.com/jackc/pgx/v4"
)

type CancelFunc func()

type DbConnector interface {
	Connect(context.Context) (*pgx.Conn, CancelFunc, error)
}
