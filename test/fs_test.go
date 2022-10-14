package test

import (
	"testing"

	"github.com/gadget-inc/dateilager/internal/db"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	fs := tc.FsApi()

	_, err := fs.NewProject(tc.Context(), &pb.NewProjectRequest{Id: 1})
	require.NoError(t, err, "fs.NewProject")

	stream := &mockGetServer{ctx: tc.Context()}

	err = fs.Get(&pb.GetRequest{Project: 1}, stream)
	require.NoError(t, err, "fs.Get")

	require.Empty(t, stream.results, "stream results should be empty")
}

func TestNewProjectWithTemplate(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(3), "/a")
	writeObject(tc, 1, 2, i(3), "/b", "b v2")
	writeObject(tc, 1, 3, nil, "/b", "b v3")
	writeObject(tc, 1, 3, nil, "/c", "c v3")

	fs := tc.FsApi()

	_, err := fs.NewProject(tc.Context(), &pb.NewProjectRequest{Id: 2, Template: i(1)})
	require.NoError(t, err, "fs.NewProject")

	stream := &mockGetServer{ctx: tc.Context()}

	err = fs.Get(prefixQuery(2, nil, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/b": {content: "b v3"},
		"/c": {content: "c v3"},
	})
}

func TestGetEmpty(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)

	fs := tc.FsApi()
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(&pb.GetRequest{Project: 1}, stream)
	require.NoError(t, err, "fs.Get")

	require.Empty(t, stream.results, "stream results should be empty")
}

func TestGetExactlyOne(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a")
	writeObject(tc, 1, 1, nil, "/b")

	fs := tc.FsApi()
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(exactQuery(1, nil, "/a"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a": {content: ""},
	})
}

func TestGetPrefix(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a/a")
	writeObject(tc, 1, 1, nil, "/a/b")
	writeObject(tc, 1, 1, nil, "/b/a")
	writeObject(tc, 1, 1, nil, "/b/b")

	fs := tc.FsApi()
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(prefixQuery(1, nil, "/a"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/a": {content: ""},
		"/a/b": {content: ""},
	})
}

func TestGetWithIgnorePattern(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, nil, "/a/b/c")
	writeObject(tc, 1, 1, nil, "/a/b/d")
	writeObject(tc, 1, 1, nil, "/a/e/f")
	writeObject(tc, 1, 1, nil, "/a/e/g")
	writeObject(tc, 1, 2, nil, "/a/e/h")
	writeObject(tc, 1, 1, nil, "/a/i/j")
	writeObject(tc, 1, 1, i(2), "/a/i/k") // deleted at version 2
	writeObject(tc, 1, 1, nil, "/l/m")

	fs := tc.FsApi()

	testCases := []struct {
		name     string
		req      *pb.GetRequest
		expected map[string]expectedObject
	}{
		{
			name: "prefix query",
			req:  prefixQuery(1, nil, "/a", "/a/b", "/a/i"),
			expected: map[string]expectedObject{
				"/a/e/f": {content: ""},
				"/a/e/g": {content: ""},
				"/a/e/h": {content: ""},
			},
		},
		{
			name: "range query",
			req:  rangeQuery(1, i(1), nil, "/a", "/a/b", "/a/i"),
			expected: map[string]expectedObject{
				"/a/e/h": {content: ""},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stream := &mockGetServer{ctx: tc.Context()}
			err := fs.Get(testCase.req, stream)
			require.NoError(t, err, "fs.Get")

			verifyStreamResults(t, stream.results, testCase.expected)
		})
	}
}

func TestGetRange(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 4)
	writeObject(tc, 1, 1, i(3), "/a")
	writeObject(tc, 1, 2, i(3), "/b", "b v2")
	writeObject(tc, 1, 3, nil, "/b", "b v3")
	writeObject(tc, 1, 3, nil, "/c")
	writeObject(tc, 1, 4, nil, "/d")

	fs := tc.FsApi()

	testCases := []struct {
		name     string
		req      *pb.GetRequest
		expected map[string]expectedObject
	}{
		{
			name: "1 to 2",
			req:  rangeQuery(1, i(1), i(2), ""),
			expected: map[string]expectedObject{
				"/b": {
					content: "b v2",
					deleted: false,
				},
			},
		},
		{
			name: "1 to 3",
			req:  rangeQuery(1, i(1), i(3), ""),
			expected: map[string]expectedObject{
				"/a": {
					content: "",
					deleted: true,
				},
				"/b": {content: "b v3"},
				"/c": {content: ""},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stream := &mockGetServer{ctx: tc.Context()}
			err := fs.Get(testCase.req, stream)
			require.NoError(t, err, "fs.Get")

			verifyStreamResults(t, stream.results, testCase.expected)
		})
	}
}

func TestGetDeleteAll(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, i(2), "/a")
	writeObject(tc, 1, 1, i(3), "/b")
	writeObject(tc, 1, 1, i(3), "/c")

	fs := tc.FsApi()

	testCases := []struct {
		name     string
		req      *pb.GetRequest
		expected map[string]expectedObject
	}{
		{
			name: "1 to 2",
			req:  rangeQuery(1, i(1), i(2), ""),
			expected: map[string]expectedObject{
				"/a": {
					content: "",
					deleted: true,
				},
			},
		},
		{
			name: "1 to 3",
			req:  rangeQuery(1, i(1), i(3), ""),
			expected: map[string]expectedObject{
				"/a": {
					content: "",
					deleted: true,
				},
				"/b": {
					content: "",
					deleted: true,
				},
				"/c": {
					content: "",
					deleted: true,
				},
			},
		},
		{
			name: "2 to 3",
			req:  rangeQuery(1, i(2), i(3), ""),
			expected: map[string]expectedObject{
				"/b": {
					content: "",
					deleted: true,
				},
				"/c": {
					content: "",
					deleted: true,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stream := &mockGetServer{ctx: tc.Context()}
			err := fs.Get(testCase.req, stream)
			require.NoError(t, err, "fs.Get")

			verifyStreamResults(t, stream.results, testCase.expected)
		})
	}
}

func TestGetExactlyOneVersioned(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "/a", "v1")
	writeObject(tc, 1, 2, i(3), "/a", "v2")
	writeObject(tc, 1, 3, nil, "/a", "v3")

	fs := tc.FsApi()

	testCases := []struct {
		name     string
		req      *pb.GetRequest
		expected map[string]expectedObject
	}{
		{
			name: "version 1",
			req:  exactQuery(1, i(1), "/a"),
			expected: map[string]expectedObject{
				"/a": {content: "v1"},
			},
		},
		{
			name: "version 2",
			req:  exactQuery(1, i(2), "/a"),
			expected: map[string]expectedObject{
				"/a": {content: "v2"},
			},
		},
		{
			name: "version 3",
			req:  exactQuery(1, i(3), "/a"),
			expected: map[string]expectedObject{
				"/a": {content: "v3"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stream := &mockGetServer{ctx: tc.Context()}
			err := fs.Get(testCase.req, stream)
			require.NoError(t, err, "fs.Get")

			verifyStreamResults(t, stream.results, testCase.expected)
		})
	}
}

func TestGetCompress(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "/a", "v1")
	writeObject(tc, 1, 2, i(3), "/a", "v2")
	writeObject(tc, 1, 3, nil, "/a", "v3")

	fs := tc.FsApi()

	stream := &mockGetCompressServer{ctx: tc.Context()}
	err := fs.GetCompress(buildCompressRequest(1, nil, nil, ""), stream)
	require.NoError(t, err, "fs.GetCompress")

	assert.Equal(t, 1, len(stream.results), "expected 1 TAR files")

	verifyTarResults(t, stream.results, map[string]expectedObject{
		"/a": {content: "v3"},
	})
}

func TestGetCompressWithIgnorePattern(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a/b/c", "a/b/c v1")
	writeObject(tc, 1, 1, nil, "/a/b/d", "a/b/d v1")
	writeObject(tc, 1, 1, nil, "/a/e/f", "a/e/f v1")
	writeObject(tc, 1, 1, nil, "/a/e/g", "a/e/g v1")

	fs := tc.FsApi()

	stream := &mockGetCompressServer{ctx: tc.Context()}
	err := fs.GetCompress(buildCompressRequest(1, nil, nil, "", "/a/e"), stream)
	require.NoError(t, err, "fs.GetCompress")

	assert.Equal(t, 1, len(stream.results), "expected 1 TAR files")

	verifyTarResults(t, stream.results, map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v1"},
		"/a/b/d": {content: "a/b/d v1"},
	})
}

func TestGetPackedObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2, "/a/")

	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
		"/a/e": {content: "a/e v1"},
	})
	writeObject(tc, 1, 2, nil, "/b", "b v2")

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(prefixQuery(1, nil, "/a"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
		"/a/e": {content: "a/e v1"},
	})
}

func TestGetObjectWithinPack(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/")
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/b": {content: "a/b v1"},
		"/a/c": {content: "a/c v1"},
	})

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(exactQuery(1, nil, "/a/b"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/b": {content: "a/b v1"},
	})
}

func TestGetObjectWithinPatternPack(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/.*/")
	writePackedObjects(tc, 1, 1, nil, "/a/b/", map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v1"},
		"/a/b/d": {content: "a/b/d v1"},
	})
	writePackedObjects(tc, 1, 1, nil, "/a/e/", map[string]expectedObject{
		"/a/e/f": {content: "a/e/f v1"},
		"/a/e/g": {content: "a/e/g v1"},
	})

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(exactQuery(1, nil, "/a/b/c"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v1"},
	})
}

func TestGetCompressReturnsPackedObjectsWithoutRepacking(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2, "pack/")
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"pack/a": {content: "pack/a v1"},
		"pack/b": {content: "pack/b v1"},
	})
	writeObject(tc, 1, 2, nil, "c", "c v2")

	fs := tc.FsApi()

	stream := &mockGetCompressServer{ctx: tc.Context()}
	err := fs.GetCompress(buildCompressRequest(1, nil, nil, ""), stream)
	require.NoError(t, err, "fs.GetCompress")

	assert.Equal(t, 2, len(stream.results), "expected 2 TAR files")

	verifyTarResults(t, stream.results, map[string]expectedObject{
		"pack/a": {content: "pack/a v1"},
		"pack/b": {content: "pack/b v1"},
		"c":      {content: "c v2"},
	})
}

func TestGetCompressWithCacheVersions(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "pack/")
	writePackedFiles(tc, 1, 1, nil, "pack/a")

	cacheVersion, err := db.CreateCache(tc.Context(), tc.Connect(), "pack/", 100)
	require.NoError(t, err)

	version, hashes := latestCacheVersionHashes(tc)
	assert.Equal(t, cacheVersion, version, "latest cache version matches newly created cache")
	assert.Equal(t, 1, len(hashes), "cache hash count")

	writePackedFiles(tc, 1, 2, nil, "pack/b")

	fs := tc.FsApi()

	stream := &mockGetCompressServer{ctx: tc.Context()}
	request := buildCompressRequest(1, nil, nil, "")
	request.AvailableCacheVersions = []int64{cacheVersion}

	err = fs.GetCompress(request, stream)
	require.NoError(t, err, "fs.GetCompress")

	assert.Equal(t, 2, len(stream.results), "expected 2 TAR files")

	verifyTarResults(t, stream.results, map[string]expectedObject{
		"pack/a":   {content: string(hashes[0].Bytes())},
		"pack/b/1": {content: "pack/b/1 v2"},
		"pack/b/2": {content: "pack/b/2 v2"},
	})
}

func TestUpdate(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "v1")
	writeObject(tc, 1, 1, nil, "/b", "v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a": {content: "v2"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(2), updateStream.response.Version, "expected version 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(1, nil, "/"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a": {content: "v2"},
		"/b": {content: "v1"},
	})
}

func TestEmptyUpdate(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "v1")
	writeObject(tc, 1, 1, nil, "/b", "v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(-1), updateStream.response.Version, "expected version -1")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(1, nil, "/"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a": {content: "v1"},
		"/b": {content: "v1"},
	})
}

func TestIdenticalUpdate(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a": {content: "v1"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(1), updateStream.response.Version, "expected version 1")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(1, nil, "/"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a": {content: "v1"},
	})
}

func TestUpdatePackedObject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/")
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
	})
	writeObject(tc, 1, 1, nil, "/b", "b v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a/c": {content: "a/c v2"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(2), updateStream.response.Version, "expected version 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, nil, "/a/c"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v2"},
	})
}

func TestEmptyUpdatePackedObject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/")
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
	})
	writeObject(tc, 1, 1, nil, "/b", "b v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(-1), updateStream.response.Version, "expected version -1")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, nil, "/a/c"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
	})
}

func TestIdenticalUpdatePackedObject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/")
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/b": {content: "a/b v1"},
		"/a/c": {content: "a/c v1"},
	})

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a/b": {content: "a/b v1"},
		"/a/c": {content: "a/c v1"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(1), updateStream.response.Version, "expected version 1")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(1, nil, "/a"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/b": {content: "a/b v1"},
		"/a/c": {content: "a/c v1"},
	})
}

func TestUpdateDeletePackedObject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/")
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
	})
	writeObject(tc, 1, 1, nil, "/b", "b v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a/c": {deleted: true},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(2), updateStream.response.Version, "expected version 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, nil, "/a/c"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{})
}

func TestUpdateWithNewPatternPackedObject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/.*/")
	writeObject(tc, 1, 1, nil, "/b", "b v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v2"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(2), updateStream.response.Version, "expected version 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, nil, "/a/b/c"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v2"},
	})
}

func TestUpdateWithExistingPatternPackedObject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/.*/")
	writePackedObjects(tc, 1, 1, nil, "/a/b/", map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v1"},
		"/a/b/d": {content: "a/b/d v1"},
	})
	writeObject(tc, 1, 2, nil, "/b", "b v1")

	fs := tc.FsApi()

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v2"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	assert.Equal(t, int64(2), updateStream.response.Version, "expected version 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, nil, "/a/b/c"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/b/c": {content: "a/b/c v2"},
	})
}

func TestSnapshotAndReset(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a/c", "a/c v1")
	writeObject(tc, 1, 1, nil, "/a/d", "a/d v1")

	fs := tc.FsApi()

	snapshotResponse, err := fs.Snapshot(tc.Context(), &pb.SnapshotRequest{})
	require.NoError(t, err, "fs.Snapshot")

	project := snapshotResponse.Projects[0]
	assert.True(t, project.Id == 1 && project.Version == 1, "expected snaptshotted project (1, 1) got (%v, %v)", project.Id, project.Version)

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a/c": {content: "a/c v2"},
	})
	err = fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(1, nil, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v2"},
		"/a/d": {content: "a/d v1"},
	})

	writeProject(tc, 2, 1)

	_, err = fs.Reset(tc.Context(), &pb.ResetRequest{
		Projects: snapshotResponse.Projects,
	})
	require.NoError(t, err, "fs.Reset")

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(1, nil, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
	})

	writeProject(tc, 2, 1)
}

func TestResetAll(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 1)

	fs := tc.FsApi()

	_, err := fs.Reset(tc.Context(), &pb.ResetRequest{})
	require.NoError(t, err, "fs.Reset")

	writeProject(tc, 1, 1)
}

func TestCloneTo(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeProject(tc, 2, 1)

	writeObject(tc, 1, 1, nil, "/a/c", "a/c v1")
	writeObject(tc, 1, 1, nil, "/a/d", "a/d v1")
	writeObject(tc, 1, 1, nil, "/a/f", "a/f v1")

	fs := tc.FsApi()

	response, err := fs.CloneToProject(tc.Context(), &pb.CloneToProjectRequest{
		Source:      1,
		FromVersion: 0,
		ToVersion:   1,
		Target:      2,
	})
	require.NoError(t, err, "fs.CloneToProject")

	assert.True(t, response.LatestVersion == 2, "expected new version to be 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(2, &response.LatestVersion, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
		"/a/f": {content: "a/f v1"},
	})

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"/a/c": {deleted: true},
		"/a/d": {content: "a/d v2"},
	})
	err = fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	writeObject(tc, 1, 2, nil, "/a/b", "a/b v1")

	response, err = fs.CloneToProject(tc.Context(), &pb.CloneToProjectRequest{
		Source:      1,
		FromVersion: 1,
		ToVersion:   2,
		Target:      2,
	})
	require.NoError(t, err, "fs.CloneToProject")

	assert.True(t, response.LatestVersion == 3, "expected new version to be 3")

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(2, &response.LatestVersion, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/d": {content: "a/d v2"},
		"/a/b": {content: "a/b v1"},
		"/a/f": {content: "a/f v1"},
	})
}

func TestCloneToVersionGreaterThanCurrent(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeProject(tc, 2, 1)

	writeObject(tc, 1, 1, nil, "/a/c", "a/c v1")
	writeObject(tc, 1, 1, nil, "/a/d", "a/d v1")

	fs := tc.FsApi()

	response, err := fs.CloneToProject(tc.Context(), &pb.CloneToProjectRequest{
		Source:      1,
		FromVersion: 0,
		ToVersion:   20,
		Target:      2,
	})
	require.NoError(t, err, "fs.CloneToProject")

	assert.True(t, response.LatestVersion == 2, "expected new version to be 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(2, i(2), ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
	})
}

// More complicated test involving a project with multiple versions already and cloning versions with increment > 1
func TestCloneToMultipleFromVersionToVersion(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 5)
	writeProject(tc, 2, 1)

	writeObject(tc, 1, 1, i(2), "/a/c", "a/c v1")
	writeObject(tc, 1, 1, i(6), "/a/d", "a/d v1")
	writeObject(tc, 1, 2, i(4), "/a/b", "a/b v1")
	writeObject(tc, 1, 5, nil, "/a/f", "a/f v1")

	fs := tc.FsApi()

	response, err := fs.CloneToProject(tc.Context(), &pb.CloneToProjectRequest{
		Source:      1,
		FromVersion: 0,
		ToVersion:   2,
		Target:      2,
	})
	require.NoError(t, err, "fs.CloneToProject")

	assert.True(t, response.LatestVersion == 2, "expected new version to be 2")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(2, &response.LatestVersion, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/d": {content: "a/d v1"},
		"/a/b": {content: "a/b v1"},
	})

	response, err = fs.CloneToProject(tc.Context(), &pb.CloneToProjectRequest{
		Source:      1,
		FromVersion: 2,
		ToVersion:   5,
		Target:      2,
	})
	require.NoError(t, err, "fs.CloneToProject")

	assert.True(t, response.LatestVersion == 3, "expected new version to be 3")

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(2, &response.LatestVersion, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a/f": {content: "a/f v1"},
		"/a/d": {content: "a/d v1"},
	})
}

func TestGetCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2, "node_modules/")
	writePackedObjects(tc, 1, 1, nil, "node_modules/", map[string]expectedObject{
		"node_modules/a/a": {content: "node_modules/a/a v1"},
		"node_modules/a/b": {content: "node_modules/a/b v1"},
	})

	writeProject(tc, 2, 2, "node_modules/")
	writePackedObjects(tc, 2, 1, nil, "node_modules/", map[string]expectedObject{
		"node_modules/b/a": {content: "node_modules/b/a v1"},
		"node_modules/b/b": {content: "node_modules/b/b v1"},
	})

	writePackedObjects(tc, 2, 1, nil, "private/", map[string]expectedObject{
		"private/a": {content: "private/a v1"},
		"private/b": {content: "private/b v1"},
	})

	_, err := db.CreateCache(tc.Context(), tc.Connect(), "node_modules", 100)

	require.NoError(t, err, "db.CreateCache")

	fs := tc.FsApi()

	stream := &mockGetCacheServer{ctx: tc.Context()}
	err = fs.GetCache(&pb.GetCacheRequest{}, stream)
	require.NoError(t, err, "fs.GetCache")

	assert.Equal(t, 2, len(stream.results), "expected 2 TAR files")
	verifyTarResults(t, stream.results, map[string]expectedObject{
		"node_modules/a/a": {content: "node_modules/a/a v1"},
		"node_modules/a/b": {content: "node_modules/a/b v1"},
		"node_modules/b/a": {content: "node_modules/b/a v1"},
		"node_modules/b/b": {content: "node_modules/b/b v1"},
	})
}

func TestGetCacheWithoutAvailableCacheVersion(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	fs := tc.FsApi()

	stream := &mockGetCacheServer{ctx: tc.Context()}
	err := fs.GetCache(&pb.GetCacheRequest{}, stream)
	require.NoError(t, err, "fs.GetCache")

	assert.Equal(t, 0, len(stream.results), "expected 0 TAR files")
	verifyTarResults(t, stream.results, map[string]expectedObject{})
}

func TestFsCreateCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 2, "pack/")
	hash1 := writePackedObjects(tc, 1, 1, nil, "pack/a", map[string]expectedObject{
		"pack/a/a": {content: "pack/a/a v1"},
		"pack/a/b": {content: "pack/a/b v1"},
	})
	hash2 := writePackedObjects(tc, 1, 2, nil, "pack/b", map[string]expectedObject{
		"pack/b/a": {content: "pack/b/a v2"},
		"pack/b/b": {content: "pack/b/b v2"},
	})

	fs := tc.FsApi()

	_, err := fs.CreateCache(tc.Context(), &pb.CreateCacheRequest{Count: 2, Prefix: "pack/"})
	require.NoError(t, err, "fs.CreateCache")

	_, cachedHashes := latestCacheVersionHashes(tc)
	assert.Equal(t, 2, len(cachedHashes))
	assert.Contains(t, cachedHashes, hash1)
	assert.Contains(t, cachedHashes, hash2)
}
