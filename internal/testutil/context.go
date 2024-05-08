package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type TestCtx struct {
	t      *testing.T
	log    *zap.Logger
	dbConn *DbTestConnector
	lookup *db.ContentLookup
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

	log := zaptest.NewLogger(t)
	zap.ReplaceGlobals(log)

	dbConn, err := newDbTestConnector(ctx, os.Getenv("DB_URI"))
	require.NoError(t, err, "connecting to DB")

	lookup, err := db.NewContentLookup()
	require.NoError(t, err, "create content lookup")

	return TestCtx{
		t:      t,
		log:    log,
		dbConn: dbConn,
		lookup: lookup,
		ctx:    ctx,
	}
}

func (tc *TestCtx) Logger() *zap.Logger {
	return tc.log
}

func (tc *TestCtx) Connector() db.DbConnector {
	return tc.dbConn
}

func (tc *TestCtx) ContentLookup() *db.ContentLookup {
	return tc.lookup
}

func (tc *TestCtx) Context() context.Context {
	return tc.ctx
}

func (tc *TestCtx) Connect() pgx.Tx {
	tx, _, err := tc.dbConn.Connect(tc.ctx)
	require.NoError(tc.t, err, "connecting to db")

	return tx
}

func (tc *TestCtx) T() *testing.T {
	return tc.t
}

func (tc *TestCtx) Close() {
	tc.dbConn.close(tc.ctx)
}

func (tc *TestCtx) FsApi() *api.Fs {
	return &api.Fs{
		Env:           environment.Test,
		DbConn:        tc.Connector(),
		ContentLookup: tc.ContentLookup(),
	}
}

func (tc *TestCtx) CachedApi(cl *client.Client, stagingPath string) *api.Cached {
	return &api.Cached{
		Env:         environment.Test,
		Client:      cl,
		StagingPath: stagingPath,
	}
}
