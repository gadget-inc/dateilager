package api

import (
	"context"

	"github.com/jackc/pgx/v4"
)

type CloseFunc func()

type DbConnector interface {
	Connect(context.Context) (*pgx.Conn, CloseFunc, error)
}
