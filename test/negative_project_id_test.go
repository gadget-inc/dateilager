package test

import (
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProjectWithNegativeID(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	fs := tc.FsApi()

	_, err := fs.NewProject(tc.Context(), &pb.NewProjectRequest{Id: -100})
	require.NoError(t, err, "fs.NewProject with negative ID")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(&pb.GetRequest{Project: -100}, stream)
	require.NoError(t, err, "fs.Get with negative project ID")

	require.Empty(t, stream.results, "new project should have no objects")
}

func TestNegativeProjectIDCreateReadUpdateDelete(t *testing.T) {
	projectID := int64(-42)

	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	fs := tc.FsApi()

	// Create project
	_, err := fs.NewProject(tc.Context(), &pb.NewProjectRequest{Id: projectID})
	require.NoError(t, err, "fs.NewProject")

	// Update: add objects
	updateStream := newMockUpdateServer(tc.Context(), projectID, map[string]expectedObject{
		"/a": {content: "a v1"},
		"/b": {content: "b v1"},
	})
	err = fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")
	assert.Equal(t, int64(1), updateStream.response.Version)

	// Read: verify objects
	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(projectID, nil, "/"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/a": {content: "a v1"},
		"/b": {content: "b v1"},
	})

	// Update again: modify an object
	updateStream2 := newMockUpdateServer(tc.Context(), projectID, map[string]expectedObject{
		"/a": {content: "a v2"},
	})
	err = fs.Update(updateStream2)
	require.NoError(t, err, "fs.Update v2")
	assert.Equal(t, int64(2), updateStream2.response.Version)

	// Read: verify updated state
	stream2 := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(projectID, nil, "/"), stream2)
	require.NoError(t, err, "fs.Get after update")

	verifyStreamResults(t, stream2.results, map[string]expectedObject{
		"/a": {content: "a v2"},
		"/b": {content: "b v1"},
	})

	// Delete project
	_, err = fs.DeleteProject(tc.Context(), &pb.DeleteProjectRequest{Project: projectID})
	require.NoError(t, err, "fs.DeleteProject")

	// Verify project is gone
	stream3 := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(projectID, nil, "/"), stream3)
	require.Error(t, err, "fs.Get after delete should fail")
}

func TestNegativeProjectIDWithTemplate(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin)
	defer tc.Close()

	// Set up template project with negative ID
	writeProject(tc, -10, 2)
	writeObject(tc, -10, 1, i(2), "/old", "old content")
	writeObject(tc, -10, 2, nil, "/current", "current content")

	fs := tc.FsApi()

	// Create new negative-ID project from negative-ID template
	_, err := fs.NewProject(tc.Context(), &pb.NewProjectRequest{Id: -20, Template: i(-10)})
	require.NoError(t, err, "fs.NewProject from template")

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(-20, nil, ""), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/current": {content: "current content"},
	})

	// Only live objects should be copied
	assert.Equal(t, 2, countObjectsByProject(tc, -10))
	assert.Equal(t, 1, countObjectsByProject(tc, -20))
}

func TestNegativeProjectIDWithDirectDBInsert(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, -50)
	defer tc.Close()

	writeProject(tc, -50, 1)
	writeObject(tc, -50, 1, nil, "/file.txt", "hello from negative land")

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(prefixQuery(-50, nil, "/"), stream)
	require.NoError(t, err, "fs.Get")

	verifyStreamResults(t, stream.results, map[string]expectedObject{
		"/file.txt": {content: "hello from negative land"},
	})
}

func TestNegativeProjectIDClientRoundTrip(t *testing.T) {
	projectID := int64(-77)

	tc := util.NewTestCtx(t, auth.Admin, projectID)
	defer tc.Close()

	c, _, close := createTestClient(tc)
	defer close()

	// Create via client
	err := c.NewProject(tc.Context(), projectID, nil, nil)
	require.NoError(t, err, "client.NewProject")

	// Get via client (empty project)
	objects, err := c.Get(tc.Context(), projectID, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.Get empty")
	assert.Empty(t, objects)

	// Delete via client
	err = c.DeleteProject(tc.Context(), projectID)
	require.NoError(t, err, "client.DeleteProject")

	// Verify gone
	_, err = c.Get(tc.Context(), projectID, "", nil, emptyVersionRange)
	require.Error(t, err, "client.Get after delete should fail")
}
