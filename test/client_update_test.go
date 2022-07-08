package test

import (
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestUpdateObjects(t *testing.T) {
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

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 2,
		count:   3,
	})

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.GetLatest after update")

	verifyObjects(t, objects, map[string]string{
		"a": "a v2",
		"b": "b v1",
		"c": "c v2",
		"d": "d v2",
	})
}

func TestUpdateWithManyObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 0)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	fixtureFiles := make(map[string]string)

	for i := 0; i < 500; i++ {
		bytes := make([]byte, 50000)
		_, err := rand.Read(bytes)
		require.NoError(t, err, "could not generate random bytes")

		content := string(bytes)

		path := fmt.Sprintf("%d", i)
		writeFile(t, tmpDir, path, content)
		fixtureFiles[path] = content
	}

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 1,
		count:   500,
	})

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.GetLatest after update")

	verifyObjects(t, objects, fixtureFiles)
}

func TestConcurrentUpdatesSetsCorrectMetadata(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 1, nil, "c", "c v1")

	c, fs, close := createTestClient(tc)
	defer close()

	tmpDir := writeTmpFiles(t, 1, map[string]string{
		"a": "a v1",
		"b": "b v1",
		"c": "c v1",
	})
	defer os.RemoveAll(tmpDir)

	// Concurrent update not visible on disk
	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"c": {content: "c v2"},
		"d": {content: "d v2"},
	})
	err := fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	writeFile(t, tmpDir, "a", "a v3")
	writeFile(t, tmpDir, "d", "d v3")

	update(tc, c, 1, tmpDir, expectedResponse{
		version: 3,
		count:   2,
	})

	verifyDir(t, tmpDir, 3, map[string]expectedFile{
		"a": {content: "a v3"},
		"b": {content: "b v1"},
		"c": {content: "c v2"},
		"d": {content: "d v3"},
	})
}
