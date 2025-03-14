package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestCombined(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")

	c, fs, close := createTestClient(tc)
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

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"a": {content: "a v2"},
		"c": {content: "c v2"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 2,
		count:   2,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a": {content: "a v2"},
		"b": {content: "b v1"},
		"c": {content: "c v2"},
	})
}

func TestCombinedWithIdenticalObjects(t *testing.T) {
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

	currentTime := time.Now().Local()
	err := os.Chtimes(filepath.Join(tmpDir, "a"), currentTime, currentTime)
	require.NoError(t, err, "touch file %v: %v", filepath.Join(tmpDir, "a"))

	err = os.Chtimes(filepath.Join(tmpDir, "b"), currentTime, currentTime)
	require.NoError(t, err, "touch file %v: %v", filepath.Join(tmpDir, "b"))

	writeFile(t, tmpDir, "c", "c v2")

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   3,
	}, nil)

	// Reset the tmpdir to remove all state and updates
	os.RemoveAll(tmpDir)
	err = os.Mkdir(tmpDir, 0775)
	require.NoError(t, err, "os.Mkdir")

	rebuild(tc, c, 1, i(1), tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   1, // Only one file should be updated since /a and /b were identical but had new mod times
	}, nil)
}

func TestCombinedWithEmptyDirectories(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeEmptyDir(tc, 1, 1, nil, "a/")
	writeEmptyDir(tc, 1, 1, nil, "b/")
	writeEmptyDir(tc, 1, 1, nil, "c/")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a/": {content: "", fileType: typeDirectory},
		"b/": {content: "", fileType: typeDirectory},
		"c/": {content: "", fileType: typeDirectory},
	})

	writeFile(t, tmpDir, "a/b", "a/b v2")
	writeFile(t, tmpDir, "b/c/d", "b/c/d v2")

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   2,
	}, nil)

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(prefixQuery(1, nil, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"a/b":   {content: "a/b v2"},
		"b/c/d": {content: "b/c/d v2"},
		"c/":    {content: ""},
	})
}

func TestCombinedWithChangingObjectTypes(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeEmptyDir(tc, 1, 1, nil, "b/")
	writeSymlink(tc, 1, 1, nil, "c", "a")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a":  {content: "a v1"},
		"b/": {fileType: typeDirectory},
		"c":  {content: "a", fileType: typeSymlink},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"a/": {mode: directoryMode},
		"b":  {content: "c", mode: symlinkMode},
		"c":  {content: "c v2"},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   3,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a/": {fileType: typeDirectory},
		"b":  {content: "c", fileType: typeSymlink},
		"c":  {content: "c v2"},
	})
}

func TestCombinedNonEmptyDirectoryIntoFile(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "foo/bar", "file contents")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"foo/bar": {content: "file contents"},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"foo": {content: "content"},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"foo": {content: "content"},
	})
}

func TestCombinedNonEmptyDirectoryIntoSymlink(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "foo/bar", "file contents")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"foo/bar": {content: "file contents"},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"target": {content: "content"},
		"foo":    {content: "target", mode: symlinkMode},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   2,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"target": {content: "content"},
		"foo":    {content: "target", fileType: typeSymlink},
	})
}

func TestCombinedFileIntoNonEmptyDirectory(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "foo", "file contents")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"foo": {content: "file contents"},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"foo/bar": {content: "content"},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"foo/bar": {content: "content"},
	})
}

func TestCombinedFileIntoEmptyDirectory(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "foo", "file contents")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"foo": {content: "file contents"},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"foo/": {mode: directoryMode},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"foo/": {fileType: typeDirectory},
	})
}

func TestCombinedWithPacked(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "a/")
	writePackedObjects(tc, 1, 1, nil, "a/", map[string]expectedObject{
		"a/c": {content: "a/c v1"},
		"a/d": {content: "a/d v1"},
		"a/e": {content: "a/e v1"},
	})
	writeObject(tc, 1, 1, nil, "b", "b v1")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   4,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a/c": {content: "a/c v1"},
		"a/d": {content: "a/d v1"},
		"a/e": {content: "a/e v1"},
		"b":   {content: "b v1"},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"a/c": {content: "a/c v2"},
		"a/e": {deleted: true},
		"b":   {content: "b v2"},
	})

	err := fs.Update(updateStream)
	if err != nil {
		t.Fatalf("fs.Update: %v", err)
	}

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   3, // We updated a pack so all of them were rebuilt
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a/c": {content: "a/c v2"},
		"a/d": {content: "a/d v1"},
		"b":   {content: "b v2"},
	})
}

func TestCombinedWithPackedSymlinks(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "a/")
	writePackedObjects(tc, 1, 1, nil, "a/", map[string]expectedObject{
		"a/c":      {content: "a/c v1"},
		"a/d":      {content: "a/d v1"},
		"a/link-c": {content: "a/c", mode: symlinkMode},
		"a/link-d": {content: "a/d", mode: symlinkMode},
		"a/link-e": {content: "a/e", mode: symlinkMode}, // Purposefully broken symlink
	})
	writeObject(tc, 1, 1, nil, "b", "b v1")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   6,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a/c":      {content: "a/c v1"},
		"a/d":      {content: "a/d v1"},
		"a/link-c": {content: "a/c", fileType: typeSymlink},
		"a/link-d": {content: "a/d", fileType: typeSymlink},
		"a/link-e": {content: "a/e", fileType: typeSymlink},
		"b":        {content: "b v1"},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"a/c":      {content: "a/c v2"},
		"a/link-d": {content: "a/link-d v2"},
		"a/link-e": {content: "a/d", mode: symlinkMode},
		"b":        {content: "b v2"},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 2,
		count:   6, // We updated one file in a pack so all of them were rebuilt
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a/c":      {content: "a/c v2"},
		"a/d":      {content: "a/d v1"},
		"a/link-c": {content: "a/c", fileType: typeSymlink},
		"a/link-d": {content: "a/link-d v2"},
		"a/link-e": {content: "a/d", fileType: typeSymlink},
		"b":        {content: "b v2"},
	})
}

func TestCombinedWithPackAsASymlink(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "[b|e]/")
	writeObject(tc, 1, 1, nil, "a/c", "a/c v1")
	writeObject(tc, 1, 1, nil, "a/d", "a/d v1")
	writePackedObjects(tc, 1, 1, nil, "b/", map[string]expectedObject{
		"b/": {content: "a/", mode: symlinkMode},
	})

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a/c": {content: "a/c v1"},
		"a/d": {content: "a/d v1"},
		"b":   {content: "a/", fileType: typeSymlink},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"a/c": {content: "a/c v2"},
		"b/":  {content: "f/", mode: symlinkMode},
		"e/":  {content: "a/", mode: symlinkMode},
		"f/g": {content: "f/g v2"},
	})

	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 2,
		count:   4,
	}, nil)

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"a/c": {content: "a/c v2"},
		"a/d": {content: "a/d v1"},
		"b":   {content: "f/", fileType: typeSymlink},
		"e":   {content: "a/", fileType: typeSymlink},
		"f/g": {content: "f/g v2"},
	})
}

func TestCombinedWithIdenticalPackedObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "a/")
	writePackedObjects(tc, 1, 1, nil, "a/", map[string]expectedObject{
		"a/c": {content: "a/c v1"},
		"a/d": {content: "a/d v1"},
	})
	writeObject(tc, 1, 1, nil, "b", "b v1")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a/c": {content: "a/c v1"},
		"a/d": {content: "a/d v1"},
		"b":   {content: "b v1"},
	})

	currentTime := time.Now().Local()
	err := os.Chtimes(filepath.Join(tmpDir, "a/c"), currentTime, currentTime)
	require.NoError(t, err, "touch file %v: %v", filepath.Join(tmpDir, "a/c"))

	err = os.Chtimes(filepath.Join(tmpDir, "a/d"), currentTime, currentTime)
	require.NoError(t, err, "touch file %v: %v", filepath.Join(tmpDir, "a/d"))

	writeFile(t, tmpDir, "b", "b v2")

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   3,
	}, nil)

	os.RemoveAll(tmpDir)
	err = os.Mkdir(tmpDir, 0775)
	require.NoError(t, err, "os.Mkdir")

	rebuild(tc, c, 1, i(1), tmpDir, nil, expectedResponse{
		version: 1,
		count:   3,
	}, nil)

	rebuild(tc, c, 1, i(2), tmpDir, nil, expectedResponse{
		version: 2,
		count:   1, // Only one file should be updated since /a and /b were identical but with a new mod times
	}, nil)
}

func TestCombinedWithPrefixDirectoryBug(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeEmptyDir(tc, 1, 1, nil, "a/")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	rebuild(tc, c, 1, nil, tmpDir, nil, expectedResponse{
		version: 1,
		count:   1,
	}, nil)

	verifyDir(t, tmpDir, 1, map[string]expectedFile{
		"a/": {content: "", fileType: typeDirectory},
	})

	writeFile(t, tmpDir, "abc", "abc v2")

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   1,
	}, nil)

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(prefixQuery(1, nil, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"a/":  {content: ""},
		"abc": {content: "abc v2"},
	})
}

func TestRebuildUpdateWithSubpaths(t *testing.T) {
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

	writeFile(t, tmpDir, "pack/sub/c/1", "pack/sub/c/1 v2")
	writeFile(t, tmpDir, "pack/a/1", "pack/a/1 v2")

	// Should only update pack/sub/c which is part of the subpaths
	update(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   1,
	}, []string{"pack/sub"})

	verifyDir(t, tmpDir, 2, map[string]expectedFile{
		"pack/a/1":     {content: "pack/a/1 v2"},
		"pack/sub/b/1": {content: "pack/sub/b/1 v1"},
		"pack/sub/b/2": {content: "pack/sub/b/2 v1"},
		"pack/sub/c/1": {content: "pack/sub/c/1 v2"},
		"pack/sub/c/2": {content: "pack/sub/c/2 v1"},
	})
}
