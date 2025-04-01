package test

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopulateCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	aHash := writePackedFiles(tc, 1, 1, nil, "pack/a")
	bHash := writePackedFiles(tc, 1, 1, nil, "pack/b")
	version, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	c, cached, close := createTestCachedClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	require.NoError(t, cached.Prepare(tc.Context(), -1), "cached.Prepare must succeed")

	_, err = c.PopulateDiskCache(tc.Context(), path.Join(tmpDir, "test"))
	require.NoError(t, err, "Cached.PopulateDiskCache")

	verifyDir(t, path.Join(tmpDir, "test"), -1, map[string]expectedFile{
		fmt.Sprintf("objects/%v/pack/a/1", aHash): {content: "pack/a/1 v1"},
		fmt.Sprintf("objects/%v/pack/a/2", aHash): {content: "pack/a/2 v1"},
		fmt.Sprintf("objects/%v/pack/b/1", bHash): {content: "pack/b/1 v1"},
		fmt.Sprintf("objects/%v/pack/b/2", bHash): {content: "pack/b/2 v1"},
		"versions": {content: fmt.Sprintf("%v\n", version)},
	})
}

func TestPopulateEmptyCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	// no packed files, so no cache
	version, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)
	assert.NotEqual(t, int64(-1), version)

	c, cached, close := createTestCachedClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	require.NoError(t, cached.Prepare(tc.Context(), -1), "cached.Prepare must succeed")

	_, err = c.PopulateDiskCache(tc.Context(), path.Join(tmpDir, "test"))
	require.NoError(t, err, "PopulateDiskCache must succeed")

	verifyDir(t, path.Join(tmpDir, "test"), -1, map[string]expectedFile{
		"objects/": {content: "", fileType: typeDirectory},
	})
}

func TestPopulateCacheToPathWithNoWritePermissions(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	writePackedFiles(tc, 1, 1, nil, "pack/a")
	writePackedFiles(tc, 1, 1, nil, "pack/b")
	_, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	c, cached, close := createTestCachedClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	require.NoError(t, cached.Prepare(tc.Context(), -1), "cached.Prepare must succeed")

	// Create a directory with no write permissions
	err = os.Mkdir(path.Join(tmpDir, "test"), 0o000)
	require.NoError(t, err)

	_, err = c.PopulateDiskCache(tc.Context(), path.Join(tmpDir, "test"))
	require.Error(t, err, "populating cache to a path with no write permissions must fail")
}
