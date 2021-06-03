package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/angelini/dateilager/pkg/api"
	"github.com/jackc/pgx/v4"
	"go.uber.org/zap"
)

type TestCtx struct {
	t      *testing.T
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
		dbConn: dbConn,
		ctx:    ctx,
	}
}

func (tc *TestCtx) Connector() api.DbConnector {
	return tc.dbConn
}

func (tc *TestCtx) Connect() *pgx.Conn {
	conn, _, err := tc.dbConn.Connect(tc.ctx)
	if err != nil {
		tc.Fatalf("connecting to db: %w", err)
	}
	return conn
}

func (tc *TestCtx) Fatalf(format string, args ...interface{}) {
	tc.t.Errorf(format, args...)
}

func (tc *TestCtx) Context() context.Context {
	return tc.ctx
}

func (tc *TestCtx) Close() {
	tc.dbConn.close(tc.ctx)
}

func (tc *TestCtx) FsApi(log *zap.Logger) *api.Fs {
	return &api.Fs{
		Log:    log,
		DbConn: tc.Connector(),
	}
}
