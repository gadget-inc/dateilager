package test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/stretchr/testify/assert"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestClientGetCacheWithEmptyCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	c, _, close := createTestClient(tc)
	defer close()

	tmpCacheDir, err := os.MkdirTemp("", "dl_cache_test_tmp")
	require.NoError(t, err)

	version, _, err := c.GetCache(tc.Context(), tmpCacheDir)
	assert.NoError(t, err, "no errors expected")
	assert.Equal(t, int64(-1), version)

	assert.Equal(t, []string{"objects"}, dirFileNames(t, tmpCacheDir))
	assert.Equal(t, []string{}, dirFileNames(t, filepath.Join(tmpCacheDir, "objects")))
}

func TestClientGetCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writePackedFiles(tc, 1, 1, nil, "node_modules/a")
	writePackedFiles(tc, 1, 1, nil, "node_modules/b")
	_, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules/", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpCacheDir, err := os.MkdirTemp("", "dl_cache_test_tmp")
	require.NoError(t, err)

	version, count, err := c.GetCache(tc.Context(), tmpCacheDir)
	require.NoError(t, err, "client.GetCache after GetCache")
	assert.Equal(t, []string{"objects", "versions"}, dirFileNames(t, tmpCacheDir))
	assert.Equal(t, uint32(4), count)

	names, err := filepath.Glob(filepath.Join(tmpCacheDir, "objects/*/node_modules/*"))
	require.NoError(t, err)

	var namesWithoutRoot []string
	for _, name := range names {
		namesWithoutRoot = append(namesWithoutRoot, name[len(filepath.Join(tmpCacheDir, "objects"))+64+2:])
	}

	sort.Strings(namesWithoutRoot)
	assert.Equal(t, []string{"node_modules/a", "node_modules/b"}, namesWithoutRoot)

	versionsFileContent, err := os.ReadFile(filepath.Join(tmpCacheDir, "versions"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d\n", version), string(versionsFileContent))
}

func TestClientGetCacheFailsIfLockCannotBeObtained(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	c, _, close := createTestClient(tc)
	defer close()

	tmpCacheDir, err := os.MkdirTemp("", "dl_cache_test_tmp")
	require.NoError(t, err)

	_, err = db.CreateCache(tc.Context(), tc.Connect(), "node_modules", 100)
	require.NoError(t, err)

	_, err = os.OpenFile(filepath.Join(tmpCacheDir, ".lock"), os.O_CREATE|os.O_EXCL, 0600)
	require.NoError(t, err)

	_, _, err = c.GetCache(tc.Context(), tmpCacheDir)
	assert.Error(t, err, "expected an error")
	assert.Contains(t, err.Error(), "unable to obtain cache lock file")
}

func TestClientCanHaveMultipleCacheVersions(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writePackedFiles(tc, 1, 1, nil, "pack/a")
	_, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpCacheDir, err := os.MkdirTemp("", "dl_cache_test_tmp")
	require.NoError(t, err)

	version1, count, err := c.GetCache(tc.Context(), tmpCacheDir)
	require.NoError(t, err, "client.GetCache after GetCache")
	assert.Equal(t, uint32(2), count)

	_, err = db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	version2, count, err := c.GetCache(tc.Context(), tmpCacheDir)
	require.NoError(t, err, "client.GetCache after GetCache")
	assert.Equal(t, uint32(0), count)

	versionsFileContent, err := os.ReadFile(filepath.Join(tmpCacheDir, "versions"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d\n%d\n", version1, version2), string(versionsFileContent))
}

func TestDownloadingTheSameVersionTwice(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writePackedFiles(tc, 1, 1, nil, "pack/a")
	_, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpCacheDir, err := os.MkdirTemp("", "dl_cache_test_tmp")
	require.NoError(t, err)

	version1, count, err := c.GetCache(tc.Context(), tmpCacheDir)
	require.NoError(t, err, "client.GetCache after GetCache")
	assert.Equal(t, uint32(2), count)

	_, count, err = c.GetCache(tc.Context(), tmpCacheDir)
	require.NoError(t, err, "client.GetCache after GetCache")
	assert.Equal(t, uint32(0), count)

	versionsFileContent, err := os.ReadFile(filepath.Join(tmpCacheDir, "versions"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d\n", version1), string(versionsFileContent))
}

func dirFileNames(t *testing.T, path string) []string {
	cacheRootFiles, err := os.ReadDir(path)
	require.NoError(t, err)
	filenames := make([]string, 0)
	for _, file := range cacheRootFiles {
		filenames = append(filenames, file.Name())
	}

	return filenames
}
