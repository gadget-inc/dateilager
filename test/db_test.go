package test

import (
	"path/filepath"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func latestCacheVersionHashes(t *testing.T, tc util.TestCtx) []string {
	conn := tc.Connect()
	row := conn.QueryRow(tc.Context(), "SELECT hashes FROM dl.cache_versions ORDER BY version DESC LIMIT 1")

	var hashes []string
	err := row.Scan(&hashes)
	assert.NoError(t, err)

	return hashes
}

func writePackedFiles(tc util.TestCtx, project int64, start int64, stop *int64, path string) {
	writePackedObjects(tc, project, start, stop, path, map[string]expectedObject{
		filepath.Join(path, "a"): {content: filepath.Join(path, "a") + "v1"},
		filepath.Join(path, "b"): {content: filepath.Join(path, "b") + "v1"},
	})
}

func TestCreateNodeModulesCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, nil, "node_modules/a")

	writeProject(tc, 2, 2, "node_modules/")
	writePackedFiles(tc, 2, 1, nil, "node_modules/b")

	err := db.CreateNodeModulesCache(tc.Context(), tc.Connect())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(latestCacheVersionHashes(t, tc)))
}

func TestCreateNodeModulesCacheOnlyUsesNodeModules(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, nil, "node_modules/a")

	writeProject(tc, 2, 2, "node_modules/")
	writePackedFiles(tc, 2, 1, nil, "node_modules/b")

	writePackedFiles(tc, 2, 1, nil, "private/")

	err := db.CreateNodeModulesCache(tc.Context(), tc.Connect())

	assert.NoError(t, err)
	assert.Equal(t, 2, len(latestCacheVersionHashes(t, tc)))
}

func TestCreateNodeModulesCacheIgnoresModulesNoLongerUsed(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, i(2), "node_modules/a")
	writePackedFiles(tc, 2, 2, nil, "node_modules/b")

	err := db.CreateNodeModulesCache(tc.Context(), tc.Connect())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(latestCacheVersionHashes(t, tc)))
}
