package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type TestCtx struct {
	*assert.Assertions
	require *require.Assertions

	log    *zap.Logger
	dbConn *DbTestConnector
	ctx    context.Context
}

func NewTestCtx(t *testing.T, role auth.Role, projects ...int64) TestCtx {
	var project *int64
	if len(projects) > 0 {
		project = &projects[0]
	}

	ctx := context.WithValue(context.Background(), auth.AuthCtxKey, auth.Auth{
		Role:    role,
		Project: project,
	})

	dbConn, err := newDbTestConnector(ctx, os.Getenv("DB_URI"))
	require.NoError(t, err, "connecting to DB")

	return TestCtx{
		Assertions: assert.New(t),
		require:    require.New(t),
		log:        zaptest.NewLogger(t),
		dbConn:     dbConn,
		ctx:        ctx,
	}
}

func (tc *TestCtx) Logger() *zap.Logger {
	return tc.log
}

func (tc *TestCtx) Connector() db.DbConnector {
	return tc.dbConn
}

func (tc *TestCtx) Context() context.Context {
	return tc.ctx
}

func (tc *TestCtx) Connect() pgx.Tx {
	tx, _, err := tc.dbConn.Connect(tc.ctx)
	tc.require.NoError(err, "connecting to db")

	return tx
}

func (tc *TestCtx) Require() *require.Assertions {
	return tc.require
}

func (tc *TestCtx) Close() {
	tc.dbConn.close(tc.ctx)
}

func (tc *TestCtx) FsApi() *api.Fs {
	return &api.Fs{
		Env:    environment.Test,
		DbConn: tc.Connector(),
	}
}
