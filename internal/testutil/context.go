package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/angelini/dateilager/pkg/api"
	"github.com/jackc/pgx/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type TestCtx struct {
	t      *testing.T
	log    *zap.Logger
	dbConn *DbTestConnector
	ctx    context.Context
}

func NewTestCtx(t *testing.T) TestCtx {
	ctx := context.Background()

	dbConn, err := newDbTestConnector(ctx, os.Getenv("DB_URI"))
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}

	return TestCtx{
		t:      t,
		log:    zaptest.NewLogger(t),
		dbConn: dbConn,
		ctx:    ctx,
	}
}

func (tc *TestCtx) Logger() *zap.Logger {
	return tc.log
}

func (tc *TestCtx) Connector() api.DbConnector {
	return tc.dbConn
}

func (tc *TestCtx) Context() context.Context {
	return tc.ctx
}

func (tc *TestCtx) Connect() pgx.Tx {
	tx, _, err := tc.dbConn.Connect(tc.ctx)
	if err != nil {
		tc.Fatalf("connecting to db: %w", err)
	}
	return tx
}

func (tc *TestCtx) Fatalf(format string, args ...interface{}) {
	tc.t.Errorf(format, args...)
}

func (tc *TestCtx) Close() {
	tc.dbConn.close(tc.ctx)
}

func (tc *TestCtx) FsApi() *api.Fs {
	return &api.Fs{
		Log:    tc.Logger(),
		DbConn: tc.Connector(),
	}
}
