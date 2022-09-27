package test

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/stretchr/testify/assert"

	"github.com/gadget-inc/dateilager/internal/db"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestRebuild(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	})

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a": {content: "a v1"},
		"b": {content: "b v1"},
		"c": {content: "c v1"},
	})
}

func TestRebuildWithOverwritesAndDeletes(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1 - long buffer of content")
	writeObject(tc, 1, 1, i(2), "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")
	writeObject(tc, 1, 1, i(2), "e", "e v1")
	writeObject(tc, 1, 2, nil, "a", "a v2")
	writeObject(tc, 1, 2, nil, "d", "d v2")
	writeSymlink(tc, 1, 2, nil, "e", "a")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := writeTmpFiles(t, 1, map[string]string{
		"a": "a v1 - long buffer of content",
		"b": "b v1",
		"c": "c v1",
		"e": "e v1",
	})
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 2,
		count:   4,
	})

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a": {content: "a v2"},
		"c": {content: "c v1"},
		"d": {content: "d v2"},
		"e": {content: "a v2"},
	})
}

func TestRebuildWithEmptyDirAndSymlink(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "d/e", "e v1")
	writeEmptyDir(tc, 1, 1, nil, "b/")
	writeSymlink(tc, 1, 2, nil, "c", "a")
	writeSymlink(tc, 1, 2, nil, "f/g/h", "d/e")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 2,
		count:   5,
	})

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a":     {content: "a v1"},
		"d/e":   {content: "e v1"},
		"b/":    {content: "", fileType: typeDirectory},
		"c":     {content: "a", fileType: typeSymlink},
		"f/g/h": {content: "d/e", fileType: typeSymlink},
	})
}

func TestRebuildWithUpdatedEmptyDirectories(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeEmptyDir(tc, 1, 1, nil, "a/")
	writeEmptyDir(tc, 1, 1, nil, "b/")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   2,
	})

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a/": {content: "", fileType: typeDirectory},
		"b/": {content: "", fileType: typeDirectory},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"a/c": {content: "a/c v2"},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 2,
		count:   1,
	})

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a/c": {content: "a/c v2"},
		"b/":  {content: "", fileType: typeDirectory},
	})
}

func TestRebuildWithManyObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	expectedFiles := make(map[string]expectedFile)

	writeProject(tc, 1, 1)
	for i := 0; i < 500; i++ {
		bytes := make([]byte, 50000)
		_, err := rand.Read(bytes)
		require.NoError(t, err, "could not generate random bytes")
		writeObject(tc, 1, 1, nil, fmt.Sprintf("/%d", i), string(bytes))
		expectedFiles[fmt.Sprintf("/%d", i)] = expectedFile{content: string(bytes)}
	}

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   500,
	})

	verifyDir(t, tmpDir, 1, expectedFiles)
}

func TestRebuildWithUpdatedObjectToDirectory(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 4)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 2, i(3), "b", "b v2")
	writeObject(tc, 1, 3, i(4), "b/c", "b/c v3")
	writeObject(tc, 1, 3, nil, "b/d", "b/d v3")
	writeObject(tc, 1, 4, nil, "b/e", "b/e v4")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, i(1), tmpDir, nil, expectedResponse{
		version: 1,
		count:   1,
	})

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a": {content: "a v1"},
	})

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 4,
		count:   2,
	})

	verifyDir(t, tmpDir, 4, map[string]expectedFile{
		"a":   {content: "a v1"},
		"b/d": {content: "b/d v3"},
		"b/e": {content: "b/e v4"},
	})
}

func TestRebuildWithCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	ha := writePackedFiles(tc, 1, 1, nil, "node_modules/a")
	hb := writePackedFiles(tc, 1, 1, nil, "node_modules/b")

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules")
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cacheDir := emptyTmpDir(t)
	defer os.RemoveAll(cacheDir)

	_, err = c.GetCache(tc.Context(), cacheDir)
	require.NoError(t, err)

	rebuild(tc, c, 1, nil, tmpDir, &cacheDir, expectedResponse{
		version: 1,
		count:   2,
	})

	aCachePath := filepath.Join(client.CacheObjectsDir(cacheDir), ha.Hex(), "node_modules/a")
	bCachePath := filepath.Join(client.CacheObjectsDir(cacheDir), hb.Hex(), "node_modules/b")

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"node_modules/a": {fileType: typeSymlink, content: aCachePath},
		"node_modules/b": {fileType: typeSymlink, content: bCachePath},
	})

	assertFileContent := func(path string, expectedContent string) {
		content, err := os.ReadFile(path)
		require.NoError(t, err, "error reading file %v: %w", path, err)
		assert.Equal(t, expectedContent, string(content))
	}

	assertFileContent(filepath.Join(aCachePath, "1"), "node_modules/a/1 v1")
	assertFileContent(filepath.Join(aCachePath, "2"), "node_modules/a/2 v1")
	assertFileContent(filepath.Join(bCachePath, "1"), "node_modules/b/1 v1")
	assertFileContent(filepath.Join(bCachePath, "2"), "node_modules/b/2 v1")
}
