package test

import (
	"encoding/hex"
	"io"
	"regexp"
	"testing"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v5"

	"github.com/stretchr/testify/require"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func latestCacheVersionHashes(tc util.TestCtx) (int64, []db.Hash) {
	conn := tc.Connect()

	var version int64

	err := conn.QueryRow(tc.Context(), `
		SELECT version
		FROM dl.cache_versions
		ORDER BY version DESC
		LIMIT 1
	`).Scan(&version)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	require.NoError(tc.T(), err, "get latest cache version")

	rows, err := conn.Query(tc.Context(), `
		WITH cache_hashes AS (
			SELECT unnest(hashes) AS hash
			FROM dl.cache_versions
			WHERE version = $1
		)
		SELECT (hash).h1, (hash).h2
		FROM cache_hashes
	`, version)
	require.NoError(tc.T(), err, "get latest cache hashes")

	var hashes []db.Hash

	for rows.Next() {
		var hash db.Hash
		err = rows.Scan(&hash.H1, &hash.H2)
		require.NoError(tc.T(), err, "scan cache hash")

		hashes = append(hashes, hash)
	}

	return version, hashes
}

func TestCreateCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "pack/")
	writePackedFiles(tc, 1, 1, nil, "pack/a")

	firstVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err, "CreateCache")

	version, firstVersionHashes := latestCacheVersionHashes(tc)
	assert.Equal(t, firstVersion, version, "latest cache version matches newly created cache")
	assert.Equal(t, 1, len(firstVersionHashes), "cache hash count 1")

	conn := tc.Connect()
	_, err = conn.Exec(tc.Context(), `
		UPDATE dl.objects SET stop_version = 2
		WHERE project = 1
		  AND path = 'pack/a'
	`)
	require.NoError(t, err)

	writePackedFiles(tc, 1, 2, nil, "pack/b")

	secondVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)
	assert.Equal(t, firstVersion+1, secondVersion)

	version, secondVersionHashes := latestCacheVersionHashes(tc)
	assert.Equal(t, secondVersion, version, "latest cache version matches newly created cache")
	assert.Equal(t, 1, len(secondVersionHashes), "cache hash count 2")
	assert.NotEqual(t, firstVersionHashes, secondVersionHashes)
}

func TestCreateCacheOnlyUsesPacksWithThePrefix(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "pack/")
	writePackedFiles(tc, 1, 1, nil, "pack/a")

	writeProject(tc, 2, 2, "pack/")
	writePackedFiles(tc, 2, 1, nil, "pack/b")

	writePackedFiles(tc, 2, 1, nil, "private/")

	cacheVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	version, hashes := latestCacheVersionHashes(tc)
	assert.Equal(t, cacheVersion, version, "latest cache version matches newly created cache")
	assert.Equal(t, 2, len(hashes), "cache hash count")
}

func TestCreateCacheIgnoresModulesNoLongerUsed(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "pack/")
	writePackedFiles(tc, 1, 1, i(2), "pack/a")
	writePackedFiles(tc, 2, 2, nil, "pack/b")

	cacheVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "pack", 100)
	require.NoError(t, err)

	version, hashes := latestCacheVersionHashes(tc)
	assert.Equal(t, cacheVersion, version, "latest cache version matches newly created cache")
	assert.Equal(t, 1, len(hashes), "cache hash count")
}

func TestGetCacheWithMultipleVersions(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 3, "pack/")
	writePackedFiles(tc, 1, 1, nil, "pack/a")

	firstVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	version, hashes := latestCacheVersionHashes(tc)
	assert.Equal(t, firstVersion, version, "latest cache version matches newly created cache")
	assert.Equal(t, 1, len(hashes), "cache hash count")

	deleteObject(tc, 1, 2, "pack/a")
	writePackedFiles(tc, 1, 1, nil, "pack/b")

	secondVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	vrange, err := db.NewVersionRange(tc.Context(), tc.Connect(), 1, i(0), i(1))
	require.NoError(t, err)

	availableVersions := []int64{firstVersion, secondVersion}

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
