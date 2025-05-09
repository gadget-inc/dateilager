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
	"github.com/gadget-inc/dateilager/internal/files"

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a": {content: "a v1"},
	})

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 4,
		count:   2,
	}, nil)

	verifyDir(t, tmpDir, 4, map[string]expectedFile{
		"a":   {content: "a v1"},
		"b/d": {content: "b/d v3"},
		"b/e": {content: "b/e v4"},
	})
}

func TestRebuildWithPackedObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writePackedFiles(tc, 1, 1, nil, "pack/a")
	writePackedFiles(tc, 1, 1, nil, "pack/b")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   4,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"pack/a/1": {content: "pack/a/1 v1"},
		"pack/a/2": {content: "pack/a/2 v1"},
		"pack/b/1": {content: "pack/b/1 v1"},
		"pack/b/2": {content: "pack/b/2 v1"},
	})
}

func TestRebuildWithCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	ha := writePackedFiles(tc, 1, 1, nil, "pack/a")
	hb := writePackedFiles(tc, 1, 1, nil, "pack/b")
	hc := writePackedFiles(tc, 1, 1, nil, "pack/", map[string]expectedObject{ // add packed files without a subdir and a symlink
		"1link": {content: "./1", mode: symlinkMode},
	})

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cacheDir := emptyTmpDir(t)
	defer os.RemoveAll(cacheDir)

	_, _, err = c.GetCache(tc.Context(), cacheDir, -1)
	require.NoError(t, err)

	rebuild(tc, c, 1, nil, tmpDir, &cacheDir, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	aCachePath := filepath.Join(client.CacheObjectsDir(cacheDir), ha, "pack/a")
	bCachePath := filepath.Join(client.CacheObjectsDir(cacheDir), hb, "pack/b")
	hcCachePath := filepath.Join(client.CacheObjectsDir(cacheDir), hc, "pack")

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"pack/a/1":   {content: "pack/a/1 v1"},
		"pack/a/2":   {content: "pack/a/2 v1"},
		"pack/b/1":   {content: "pack/b/1 v1"},
		"pack/b/2":   {content: "pack/b/2 v1"},
		"pack/1":     {content: "pack/1 v1"},
		"pack/2":     {content: "pack/2 v1"},
		"pack/1link": {content: "./1", fileType: typeSymlink},
	})

	assertFileContent := func(path string, expectedContent string) {
		content, err := os.ReadFile(path)
		require.NoError(t, err, "error reading file %v: %w", path, err)
		assert.Equal(t, expectedContent, string(content))
	}

	assertSymlinkTarget := func(path string, expectedTarget string) {
		target, err := os.Readlink(path)
		require.NoError(t, err, "error reading symlink %v: %w", path, err)
		assert.Equal(t, expectedTarget, target)
	}

	assertFileContent(filepath.Join(aCachePath, "1"), "pack/a/1 v1")
	assertFileContent(filepath.Join(aCachePath, "2"), "pack/a/2 v1")
	assertFileContent(filepath.Join(bCachePath, "1"), "pack/b/1 v1")
	assertFileContent(filepath.Join(bCachePath, "2"), "pack/b/2 v1")
	assertFileContent(filepath.Join(hcCachePath, "1"), "pack/1 v1")
	assertSymlinkTarget(filepath.Join(hcCachePath, "1link"), "./1")
}

func TestRebuildWithInexistantCacheDir(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writePackedFiles(tc, 1, 1, nil, "pack/a")

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	badPath := "/this/folder/does/not/exist"
	rebuild(tc, c, 1, nil, tmpDir, &badPath, expectedResponse{
		version: 1,
		count:   2,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"pack/a/1": {content: "pack/a/1 v1"},
		"pack/a/2": {content: "pack/a/2 v1"},
	})
}

func TestRebuildWithFileMatcherInclude(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "ab", "ab v1")
	writeObject(tc, 1, 1, nil, "bb", "bb v1")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	matcher, err := files.NewFileMatcher("[ab]*", "")
	require.NoError(t, err, "invalid file pattern: %w", err)

	rebuildWithMatcher(tc, c, 1, nil, tmpDir, matcher, true, expectedResponse{
		version: 1,
		count:   2,
	})

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"ab": {content: "ab v1"},
		"bb": {content: "bb v1"},
	})

	otherDir := emptyTmpDir(t)
	defer os.RemoveAll(otherDir)

	matcher, err = files.NewFileMatcher("a*", "")
	require.NoError(t, err, "invalid file pattern: %w", err)

	rebuildWithMatcher(tc, c, 1, nil, otherDir, matcher, false, expectedResponse{
		version: 1,
		count:   2,
	})

	verifyDir(t, otherDir, 1, map[string]expectedFile{
		"ab": {content: "ab v1"},
		"bb": {content: "bb v1"},
	})
}

func TestRebuildWithFileMatcherExclude(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, nil, "ab", "ab v1")
	writeObject(tc, 1, 1, nil, "bb", "bb v1")
	writeObject(tc, 1, 1, nil, "cb", "cb v1")
	writeObject(tc, 1, 2, nil, "ac", "ac v2")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	matcher, err := files.NewFileMatcher("", "ac")
	require.NoError(t, err, "invalid file pattern: %w", err)

	rebuildWithMatcher(tc, c, 1, i(1), tmpDir, matcher, true, expectedResponse{
		version: 1,
		count:   3,
	})

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"ab": {content: "ab v1"},
		"bb": {content: "bb v1"},
		"cb": {content: "cb v1"},
	})

	rebuildWithMatcher(tc, c, 1, nil, tmpDir, matcher, false, expectedResponse{
		version: 2,
		count:   1,
	})

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"ab": {content: "ab v1"},
		"bb": {content: "bb v1"},
		"cb": {content: "cb v1"},
		"ac": {content: "ac v2"},
	})
}

func TestRebuildWithFileMatcherIncludeAndExclude(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, nil, "ab", "ab v1")
	writeObject(tc, 1, 1, nil, "bb", "bb v1")
	writeObject(tc, 1, 1, nil, "cb", "cb v1")
	writeObject(tc, 1, 2, nil, "ac", "ac v2")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	matcher, err := files.NewFileMatcher("[a|b|c]*", "ac")
	require.NoError(t, err, "invalid file pattern: %w", err)

	rebuildWithMatcher(tc, c, 1, i(1), tmpDir, matcher, true, expectedResponse{
		version: 1,
		count:   3,
	})

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"ab": {content: "ab v1"},
		"bb": {content: "bb v1"},
		"cb": {content: "cb v1"},
	})

	rebuildWithMatcher(tc, c, 1, nil, tmpDir, matcher, false, expectedResponse{
		version: 2,
		count:   1,
	})

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"ab": {content: "ab v1"},
		"bb": {content: "bb v1"},
		"cb": {content: "cb v1"},
		"ac": {content: "ac v2"},
	})
}

func TestRebuildWithMissingMetadataDir(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "ab", "ab v1")
	writePackedFiles(tc, 1, 1, nil, "pack/a")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"ab":       {content: "ab v1"},
		"pack/a/1": {content: "pack/a/1 v1"},
		"pack/a/2": {content: "pack/a/2 v1"},
	})

	os.RemoveAll(filepath.Join(tmpDir, ".dl"))

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"ab":       {content: "ab v1"},
		"pack/a/1": {content: "pack/a/1 v1"},
		"pack/a/2": {content: "pack/a/2 v1"},
	})
}

func TestRebuildFileBecomesADir(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a.html", "a v1")
	writeObject(tc, 1, 2, nil, "a.html/foo", "a v2")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, i(1), tmpDir, nil, expectedResponse{
		version: 1,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a.html": {content: "a v1"},
	})

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a.html/foo": {content: "a v2"},
	})
}

func TestRebuildWithSubpaths(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "ab", "ab v1")
	writePackedFiles(tc, 1, 1, nil, "pack/a")
	writePackedFiles(tc, 1, 1, nil, "pack/sub/b")
	writePackedFiles(tc, 1, 1, nil, "pack/sub/c")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   4,
	}, []string{"pack/sub"})

	// Should only have the objects under the subpath
	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"pack/sub/b/1": {content: "pack/sub/b/1 v1"},
		"pack/sub/b/2": {content: "pack/sub/b/2 v1"},
		"pack/sub/c/1": {content: "pack/sub/c/1 v1"},
		"pack/sub/c/2": {content: "pack/sub/c/2 v1"},
	})
}
