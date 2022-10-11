package test

import (
	"encoding/hex"
	"io"
	"regexp"
	"testing"

	"github.com/gadget-inc/dateilager/internal/pb"

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
		var hash db.Hash
		err = rows.Scan(&hash.H1, &hash.H2)
		require.NoError(t, err)

		hashes = append(hashes, hash.Bytes())
	}

	return hashes
}

func TestCreateCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, nil, "node_modules/a")

	firstVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules", 100)
	firstVersionHashes := latestCacheVersionHashes(t, tc)
	require.NoError(t, err)
	assert.Equal(t, 1, len(firstVersionHashes))

	conn := tc.Connect()
	_, err = conn.Exec(tc.Context(), "UPDATE dl.objects SET stop_version = 2 WHERE project = 1 AND PATH = 'node_modules/a'")
	require.NoError(t, err)

	writePackedFiles(tc, 1, 2, nil, "node_modules/b")

	var newVersion int64
	newVersion, err = db.CreateCache(tc.Context(), tc.Connect(), "node_modules", 100)
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

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules", 100)

	require.NoError(t, err)
	assert.Equal(t, 2, len(latestCacheVersionHashes(t, tc)))
}

func TestCreateCacheIgnoresModulesNoLongerUsed(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedFiles(tc, 1, 1, i(2), "node_modules/a")
	writePackedFiles(tc, 2, 2, nil, "node_modules/b")

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules", 100)
	require.NoError(t, err)
	assert.Equal(t, 1, len(latestCacheVersionHashes(t, tc)))
}

func TestGetCacheWithMultipleVersions(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 3, "pack/")

	writePackedFiles(tc, 1, 1, nil, "pack/a")
	cache1, err := db.CreateCache(tc.Context(), tc.Connect(), "pack", 100)
	require.NoError(t, err)
	assert.Equal(t, 1, len(latestCacheVersionHashes(t, tc)))

	deleteObject(tc, 1, 2, "pack/a")
	writePackedFiles(tc, 1, 1, nil, "pack/b")
	cache2, err := db.CreateCache(tc.Context(), tc.Connect(), "pack", 100)
	require.NoError(t, err)

	vrange, err := db.NewVersionRange(tc.Context(), tc.Connect(), 1, i(0), i(1))
	require.NoError(t, err)

	availableVersions := []int64{cache1, cache2}

	query := &pb.ObjectQuery{
		Path:        "pack",
		IsPrefix:    true,
		WithContent: true,
	}
	tars, err := db.GetTars(tc.Context(), tc.Connect(), 1, availableVersions, vrange, query)
	require.NoError(t, err)

	var paths []string

	for {
		tar, _, err := tars()
		if err == io.EOF {
			break
		}
		if err == db.SKIP {
			continue
		}
		tarReader := db.NewTarReader(tar)

		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			content, err := tarReader.ReadContent()
			require.NoError(t, err)
			assert.Equal(t, pb.TarCached, int32(header.Typeflag))
			hexContent := hex.EncodeToString(content)
			assert.Regexp(t, regexp.MustCompile("[0-9a-f]{64}"), hexContent)
			paths = append(paths, header.Name)
		}
	}
	assert.Equal(t, []string{"pack/a", "pack/b"}, paths)
}
