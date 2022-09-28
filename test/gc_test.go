package test

import (
	"fmt"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGcProjectRemovesObjectsAndContent(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 4)
	writeObject(tc, 1, 1, i(2), "/a", "a v1")
	writeObject(tc, 1, 2, i(3), "/b", "b v2")
	writeObject(tc, 1, 3, i(4), "/b", "b v3")
	writeObject(tc, 1, 3, nil, "/c", "c v3")
	writeObject(tc, 1, 4, nil, "/d", "d v4")

	fs := tc.FsApi()

	response, err := fs.GcProject(tc.Context(), &pb.GcProjectRequest{
		Project:      1,
		KeepVersions: 1,
	})
	require.NoError(t, err, "fs.GcProject")

	assert.Equal(t, int64(1), response.Project, "Gc result project")
	assert.Equal(t, int64(2), response.Count, "Gc result count")

	stream := &mockGetServer{ctx: tc.Context()}

	err = fs.Get(prefixQuery(1, i(2), ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{})
}

func TestGcProjectWithoutDeletes(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 2, nil, "/b", "b v2")
	writeObject(tc, 1, 3, nil, "/c", "c v3")

	fs := tc.FsApi()

	response, err := fs.GcProject(tc.Context(), &pb.GcProjectRequest{
		Project:      1,
		KeepVersions: 3,
	})
	require.NoError(t, err, "fs.GcProject")

	assert.Equal(t, int64(1), response.Project, "Gc result project")
	assert.Equal(t, int64(0), response.Count, "Gc result count")

	stream := &mockGetServer{ctx: tc.Context()}

	err = fs.Get(prefixQuery(1, i(3), ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a": {content: "a v1"},
		"/b": {content: "b v2"},
		"/c": {content: "c v3"},
	})
}

func TestGcProjectFromVersion(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	writeProject(tc, 1, 4)
	writeObject(tc, 1, 1, i(3), "/a", "a v1")
	writeObject(tc, 1, 2, i(3), "/b", "b v2")
	writeObject(tc, 1, 3, i(4), "/b", "b v3")
	writeObject(tc, 1, 3, nil, "/c", "c v3")
	writeObject(tc, 1, 4, nil, "/d", "d v4")

	fs := tc.FsApi()

	response, err := fs.GcProject(tc.Context(), &pb.GcProjectRequest{
		Project:      1,
		KeepVersions: 1,
		FromVersion:  i(1),
	})
	require.NoError(t, err, "fs.GcProject")

	assert.Equal(t, int64(1), response.Project, "Gc result project")
	assert.Equal(t, int64(1), response.Count, "Gc result count")

	stream := &mockGetServer{ctx: tc.Context()}

	err = fs.Get(prefixQuery(1, i(2), ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a": {content: "a v1"},
	})
}

func TestGcRandomProjectsRemovesObjectsAndContent(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	for project := int64(1); project <= 20; project++ {
		writeProject(tc, project, 4)
		writeObject(tc, project, 1, i(2), "/a", fmt.Sprintf("%d: a v1", project))
		writeObject(tc, project, 2, i(3), "/b", fmt.Sprintf("%d: b v2", project))
		writeObject(tc, project, 3, i(4), "/b", fmt.Sprintf("%d: b v3", project))
		writeObject(tc, project, 3, nil, "/c", fmt.Sprintf("%d: c v3", project))
		writeObject(tc, project, 4, nil, "/d", fmt.Sprintf("%d: d v4", project))
	}

	objectsCount := countObjects(tc)
	contentsCount := countContents(tc)

	fs := tc.FsApi()

	response, err := fs.GcRandomProjects(tc.Context(), &pb.GcRandomProjectsRequest{
		Sample:       25.0,
		KeepVersions: 1,
	})
	require.NoError(t, err, "fs.GcRandomProjects")

	assert.GreaterOrEqual(t, response.Count, int64(1), "Gc result count")
	assert.GreaterOrEqual(t, len(response.Projects), 1, "Gc result projects")

	assert.Less(t, countObjects(tc), objectsCount, "Gc fewer objects")
	assert.Less(t, countContents(tc), contentsCount, "Gc fewer contents")
}

func TestGcContentsRemovesNoContentWhenEverythingIsReferenced(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	for project := int64(1); project <= 20; project++ {
		writeProject(tc, project, 4)
		writeObject(tc, project, 1, i(2), "/a", fmt.Sprintf("%d: a v1", project))
		writeObject(tc, project, 2, i(3), "/b", fmt.Sprintf("%d: b v2", project))
		writeObject(tc, project, 3, i(4), "/b", fmt.Sprintf("%d: b v3", project))
		writeObject(tc, project, 3, nil, "/c", fmt.Sprintf("%d: c v3", project))
		writeObject(tc, project, 4, nil, "/d", fmt.Sprintf("%d: d v4", project))
	}

	objectsCount := countObjects(tc)
	contentsCount := countContents(tc)

	fs := tc.FsApi()

	response, err := fs.GcContents(tc.Context(), &pb.GcContentsRequest{
		Sample: 25.0,
	})
	require.NoError(t, err, "fs.GcRandomProjects")

	assert.Equal(t, int64(0), response.Count, "Gc result count")

	assert.Equal(t, objectsCount, countObjects(tc), "Gc same objects")
	assert.Equal(t, contentsCount, countContents(tc), "Gc same contents")
}

func TestGcContentsRemovesContent(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	for project := int64(1); project <= 20; project++ {
		writeProject(tc, project, 4)
		writeObject(tc, project, 1, i(2), "/a", fmt.Sprintf("%d: a v1", project))
		writeObject(tc, project, 2, i(3), "/b", fmt.Sprintf("%d: b v2", project))
		writeObject(tc, project, 3, i(4), "/b", fmt.Sprintf("%d: b v3", project))
		writeObject(tc, project, 3, nil, "/c", fmt.Sprintf("%d: c v3", project))
		writeObject(tc, project, 4, nil, "/d", fmt.Sprintf("%d: d v4", project))

		// unreference some content
		deleteObject(tc, project, 2, "/b")
		deleteObject(tc, project, 4, "/d")
	}

	objectsCount := countObjects(tc)
	contentsCount := countContents(tc)

	fs := tc.FsApi()
	var response *pb.GcContentsResponse

	// SYSTEM sampling sometimes returns no results for small datasets
	// attempt this a few times
	for i := 0; i < 10; i++ {
		var err error
		response, err = fs.GcContents(tc.Context(), &pb.GcContentsRequest{
			Sample: 25.0,
		})
		require.NoError(t, err, "fs.GcRandomProjects")

		if response.Count > 0 {
			break
		}
	}

	assert.GreaterOrEqual(t, response.Count, int64(1), "Gc result count")

	assert.Equal(t, objectsCount, countObjects(tc), "Gc same objects")
	assert.Less(t, countContents(tc), contentsCount, "Gc fewer contents")
}
