package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func latestCacheVersionHashes(t *testing.T, tc util.TestCtx) [][]byte {
	conn := tc.Connect()
	rows, err := conn.Query(tc.Context(), "\tSELECT (hash).h1, (hash).h2 FROM (SELECT UNNEST(hashes) AS hash FROM dl.cache_versions WHERE version IN (SELECT version FROM dl.cache_versions ORDER BY version DESC LIMIT 1) ) unnested")
	require.NoError(t, err)

	var hashes [][]byte

	for rows.Next() {
		var h1, h2 []byte
		err = rows.Scan(&h1, &h2)
		require.NoError(t, err)

		hash := make([]byte, 32)
		copy(hash[0:16], h1)
		copy(hash[16:32], h2)

		hashes = append(hashes, hash)
	}

	return hashes
}

func TestCreateCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, nil, "node_modules/a")

	firstVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules")
	firstVersionHashes := latestCacheVersionHashes(t, tc)
	require.NoError(t, err)
	assert.Equal(t, 1, len(firstVersionHashes))

	conn := tc.Connect()
	_, err = conn.Exec(tc.Context(), "UPDATE dl.objects SET stop_version = 2 WHERE project = 1 AND PATH = 'node_modules/a'")
	require.NoError(t, err)

	writePackedFiles(tc, 1, 2, nil, "node_modules/b")

	var newVersion int64
	newVersion, err = db.CreateCache(tc.Context(), tc.Connect(), "node_modules")
	require.NoError(t, err)

	newVersionHashes := latestCacheVersionHashes(t, tc)
	assert.Equal(t, firstVersion+1, newVersion)
	assert.Equal(t, 1, len(latestCacheVersionHashes(t, tc)))
	assert.NotEqual(t, firstVersionHashes, newVersionHashes)
}

func TestCreateCacheOnlyUsesPacksWithThePrefix(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, nil, "node_modules/a")

	writeProject(tc, 2, 2, "node_modules/")
	writePackedFiles(tc, 2, 1, nil, "node_modules/b")

	writePackedFiles(tc, 2, 1, nil, "private/")

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules")

	require.NoError(t, err)
	assert.Equal(t, 2, len(latestCacheVersionHashes(t, tc)))
}

func TestCreateCacheIgnoresModulesNoLongerUsed(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, i(2), "node_modules/a")
	writePackedFiles(tc, 2, 2, nil, "node_modules/b")

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules")
	require.NoError(t, err)
	assert.Equal(t, 1, len(latestCacheVersionHashes(t, tc)))
}
