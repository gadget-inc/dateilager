package test

import (
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestDeleteProject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 2, nil, "c", "c v2")

	c, _, close := createTestClient(tc)
	defer close()

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.GetLatest with results")

	verifyObjects(t, objects, map[string]string{
		"b": "b v1",
		"c": "c v2",
	})

	err = c.DeleteProject(tc.Context(), 1)
	require.NoError(t, err, "client.DeleteProject with results")

	objects, err = c.Get(tc.Context(), 1, "", nil, toVersion(1))
	require.Error(t, err, "client.GetLatest didn't error accessing objects: %v", objects)
}
