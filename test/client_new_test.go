package test

import (
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientNewProjectEmptyPackPattern(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	c, fs, close := createTestClient(tc)
	defer close()

	err := c.NewProject(tc.Context(), 1, nil, nil)
	require.NoError(t, err, "NewProject")

	updateStream := newMockUpdateServer(tc.Context(), 1, map[string]expectedObject{
		"a": {content: "a v1"},
		"b": {content: "b v1"},
		"c": {content: "c v1"},
	})
	err = fs.Update(updateStream)
	require.NoError(t, err, "fs.Update")

	stream := &mockGetCompressServer{ctx: tc.Context()}
	err = fs.GetCompress(buildCompressRequest(1, nil, nil, ""), stream)
	require.NoError(t, err, "fs.GetCompress")

	// If the objects were marked as packed they would be returned as more than 1 TAR
	assert.Equal(t, 1, len(stream.results), "expected 1 TAR files")

	verifyTarResults(t, stream.results, map[string]expectedObject{
		"a": {content: "a v1"},
		"b": {content: "b v1"},
		"c": {content: "c v1"},
	})
}
