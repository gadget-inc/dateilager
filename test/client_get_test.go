package test

import (
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/stretchr/testify/require"
)

func TestGetLatestEmpty(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	c, _, close := createTestClient(tc)
	defer close()

	writeProject(tc, 1, 1)

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	require.NoError(t, err, "client.GetLatest empty")

	require.Empty(t, objects, "object list should be empty")
}

func TestGet(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	writeObject(tc, 1, 1, nil, "b", "b v1")
	writeObject(tc, 1, 2, nil, "c", "c v2")
	writeObject(tc, 1, 3, nil, "d", "d v3")

	c, _, close := createTestClient(tc)
	defer close()

	testCases := []struct {
		name     string
		project  int64
		prefix   string
		ignores  []string
		vrange   client.VersionRange
		expected map[string]string
	}{
		{
			name:    "get version 1",
			project: 1,
			vrange:  toVersion(1),
			expected: map[string]string{
				"a": "a v1",
				"b": "b v1",
			},
		},
		{
			name:    "get version 2",
			project: 1,
			vrange:  toVersion(2),
			expected: map[string]string{
				"b": "b v1",
				"c": "c v2",
			},
		},
		{
			name:    "get version with prefix",
			project: 1,
			prefix:  "b",
			vrange:  toVersion(2),
			expected: map[string]string{
				"b": "b v1",
			},
		},
		{
			name:    "get latest version",
			project: 1,
			vrange:  emptyVersionRange,
			expected: map[string]string{
				"b": "b v1",
				"c": "c v2",
				"d": "d v3",
			},
		},
		{
			name:    "get latest version with prefix",
			project: 1,
			prefix:  "c",
			vrange:  emptyVersionRange,
			expected: map[string]string{
				"c": "c v2",
			},
		},
		{
			name:    "get latest version with ignores",
			project: 1,
			prefix:  "",
			ignores: []string{"b"},
			vrange:  emptyVersionRange,
			expected: map[string]string{
				"c": "c v2",
				"d": "d v3",
			},
		},
		{
			name:    "get latest version with ignores and deleted files",
			project: 1,
			prefix:  "",
			ignores: []string{"a"},
			vrange:  fromVersion(1), // makes sure the query includes deleted files
			expected: map[string]string{
				"c": "c v2",
				"d": "d v3",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			objects, err := c.Get(tc.Context(), testCase.project, testCase.prefix, testCase.ignores, testCase.vrange)
			require.NoError(t, err, "client.Get")

			verifyObjects(t, objects, testCase.expected)
		})
	}
}

func TestGetVersionMissingProject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	c, _, close := createTestClient(tc)
	defer close()

	objects, err := c.Get(tc.Context(), 1, "", nil, toVersion(1))
	require.Error(t, err, "client.GetLatest didn't error accessing objects: %v", objects)
}
