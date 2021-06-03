package api

import (
	"context"
	"crypto/sha256"

	"github.com/jackc/pgx/v4"
)

func HashContents(data []byte) ([]byte, []byte) {
	sha := sha256.Sum256(data)
	return sha[0:16], sha[16:]
}

type CloseFunc func()

type DbConnector interface {
	Connect(context.Context) (pgx.Tx, CloseFunc, error)
}
