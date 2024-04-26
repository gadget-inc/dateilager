package test

import (
	"os"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagedUpdateObjectsShouldNotBeVisible(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := writeTmpFiles(t, 1, map[string]string{
		"a": "a v1",
		"b": "b v1",
		"c": "c v1",
	})
	defer os.RemoveAll(tmpDir)

	writeFile(t, tmpDir, "a", "a v2")
	writeFile(t, tmpDir, "c", "c v2")
	writeFile(t, tmpDir, "d", "d v2")

	stagedUpdate(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   3,
	})

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.GetLatest after update")

	verifyObjects(t, objects, map[string]string{
		"a": "a v1",
		"b": "b v1",
		"c": "c v1",
	})
}

func TestStagedUpdateWithCommitShouldBeVisible(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := writeTmpFiles(t, 1, map[string]string{
		"a": "a v1",
		"b": "b v1",
		"c": "c v1",
	})
	defer os.RemoveAll(tmpDir)

	writeFile(t, tmpDir, "a", "a v2")
	writeFile(t, tmpDir, "c", "c v2")
	writeFile(t, tmpDir, "d", "d v2")

	stagedUpdate(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   3,
	})

	err := c.CommitUpdate(tc.Context(), 1, 2)
	require.NoError(t, err, "client.CommitUpdate after staged update")

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.GetLatest after update")

	verifyObjects(t, objects, map[string]string{
		"a": "a v2",
		"b": "b v1",
		"c": "c v2",
		"d": "d v2",
	})
}

func TestStagedUpdateWithCommitAndDeletes(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := writeTmpFiles(t, 1, map[string]string{
		"a": "a v1",
		"b": "b v1",
		"c": "c v1",
	})
	defer os.RemoveAll(tmpDir)

	writeFile(t, tmpDir, "a", "a v2")
	writeFile(t, tmpDir, "d", "d v2")
	removeFile(t, tmpDir, "c")

	stagedUpdate(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   3,
	})

	err := c.CommitUpdate(tc.Context(), 1, 2)
	require.NoError(t, err, "client.CommitUpdate after staged update")

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.GetLatest after update")

	verifyObjects(t, objects, map[string]string{
		"a": "a v2",
		"b": "b v1",
		"d": "d v2",
	})
}

func TestCannotCommitAfterAnotherUpdate(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := writeTmpFiles(t, 1, map[string]string{
		"a": "a v1",
		"b": "b v1",
		"c": "c v1",
	})
	defer os.RemoveAll(tmpDir)

	writeFile(t, tmpDir, "a", "a v2")
	writeFile(t, tmpDir, "c", "c v2")
	writeFile(t, tmpDir, "d", "d v2")

	stagedUpdate(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   3,
	})

	writeFile(t, tmpDir, "e", "e v3")

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   1,
	})

	err := c.CommitUpdate(tc.Context(), 1, 2)
	assert.ErrorContains(t, err, "FS commit invalid, latest version 2 is greater than or equal to the commit version 2")
}
