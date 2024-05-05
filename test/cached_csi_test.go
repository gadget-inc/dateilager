package test

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/gadget-inc/dateilager/pkg/cached"
	"github.com/kubernetes-csi/csi-test/pkg/sanity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedCSIDriver(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	writePackedFiles(tc, 1, 1, nil, "pack/a")
	writePackedFiles(tc, 1, 1, nil, "pack/b")
	_, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	s := cached.NewCacheCSIServer(tc.Context(), c, path.Join(tmpDir, "cached", "staging"))
	require.NoError(t, s.Cached.Prepare(tc.Context()), "cached.Prepare must succeed")
	defer s.Grpc.GracefulStop()

	socket := path.Join(tmpDir, "csi.sock")
	endpoint := "unix://" + socket
	go (func() {
		require.NoError(t, s.ServeCSI(tc.Context(), endpoint), "ServeCSI must succeed")
	})()

	sanityPath := path.Join(tmpDir, "csi")
	require.NoError(t, os.MkdirAll(sanityPath, 0755), "couldn't make staging path")

	cfg := &sanity.Config{
		StagingPath: path.Join(tmpDir, "staging"),
		TargetPath:  path.Join(tmpDir, "target"),
		Address:     endpoint,
	}

	sanity.Test(t, cfg)
}

func TestCachedCSIDriverMountsCache(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	aHash := writePackedFiles(tc, 1, 1, nil, "pack/a")
	bHash := writePackedFiles(tc, 1, 1, nil, "pack/b")
	version, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	s := cached.NewCacheCSIServer(tc.Context(), c, path.Join(tmpDir, "cached", "staging"))
	require.NoError(t, s.Cached.Prepare(tc.Context()), "cached.Prepare must succeed")
	defer s.Grpc.GracefulStop()

	targetDir := path.Join(tmpDir, "vol-target")

	_, err = s.Cached.NodePublishVolume(tc.Context(), &csi.NodePublishVolumeRequest{
		VolumeId:          "foobar",
		StagingTargetPath: path.Join(tmpDir, "vol-staging-target"),
		TargetPath:        targetDir,
		VolumeCapability:  &csi.VolumeCapability{},
	})
	require.NoError(t, err)

	verifyDir(t, targetDir, -1, map[string]expectedFile{
		fmt.Sprintf("objects/%v/pack/a/1", aHash): {content: "pack/a/1 v1"},
		fmt.Sprintf("objects/%v/pack/a/2", aHash): {content: "pack/a/2 v1"},
		fmt.Sprintf("objects/%v/pack/b/1", bHash): {content: "pack/b/1 v1"},
		fmt.Sprintf("objects/%v/pack/b/2", bHash): {content: "pack/b/2 v1"},
		"versions": {content: fmt.Sprintf("%v\n", version)},
	})

	fileInfo, err := os.Stat(targetDir)
	require.NoError(t, err)

	// the target dir should not be world writable -- only by the user the CSI driver is running as (which will be root)
	require.Equal(t, formatFileMode(os.FileMode(0755)), formatFileMode(fileInfo.Mode()&os.ModePerm))

	// files inside cache dir should also *not* be writable -- it's managed by the CSI and must remain pristine
	cacheFileInfo, err := os.Stat(path.Join(targetDir, fmt.Sprintf("objects/%v/pack/a/1", aHash)))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0755)), formatFileMode(cacheFileInfo.Mode()&os.ModePerm))
}

func TestCachedCSIDriverMountsCacheAtSuffix(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	aHash := writePackedFiles(tc, 1, 1, nil, "pack/a")
	bHash := writePackedFiles(tc, 1, 1, nil, "pack/b")
	version, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	// defer os.RemoveAll(tmpDir)

	s := cached.NewCacheCSIServer(tc.Context(), c, path.Join(tmpDir, "cached", "staging"))
	require.NoError(t, s.Cached.Prepare(tc.Context()), "cached.Prepare must succeed")
	defer s.Grpc.GracefulStop()

	targetDir := path.Join(tmpDir, "vol-target")
	_, err = s.Cached.NodePublishVolume(tc.Context(), &csi.NodePublishVolumeRequest{
		VolumeId:          "foobar",
		StagingTargetPath: path.Join(tmpDir, "vol-staging-target"),
		TargetPath:        targetDir,
		VolumeCapability:  &csi.VolumeCapability{},
		VolumeContext:     map[string]string{"placeCacheAtPath": "inner_mount"},
	})
	require.NoError(t, err)

	verifyDir(t, path.Join(tmpDir, "vol-target"), -1, map[string]expectedFile{
		fmt.Sprintf("inner_mount/objects/%v/pack/a/1", aHash): {content: "pack/a/1 v1"},
		fmt.Sprintf("inner_mount/objects/%v/pack/a/2", aHash): {content: "pack/a/2 v1"},
		fmt.Sprintf("inner_mount/objects/%v/pack/b/1", bHash): {content: "pack/b/1 v1"},
		fmt.Sprintf("inner_mount/objects/%v/pack/b/2", bHash): {content: "pack/b/2 v1"},
		"inner_mount/versions":                                {content: fmt.Sprintf("%v\n", version)},
	})

	fileInfo, err := os.Stat(targetDir)
	require.NoError(t, err)

	// the target dir *should* be world writable -- we're going to use it as a scratch space to do useful stuff with the cache
	require.Equal(t, formatFileMode(os.FileMode(0777)), formatFileMode(fileInfo.Mode()&os.ModePerm))

	// the cache dir should *not* be writable -- it's managed by the CSI and must remain pristine
	cacheFileInfo, err := os.Stat(path.Join(targetDir, "inner_mount"))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0755)), formatFileMode(cacheFileInfo.Mode()&os.ModePerm))

	// files inside cache dir should *not* be writable -- it's managed by the CSI and must remain pristine
	cacheFileInfo, err = os.Stat(path.Join(targetDir, fmt.Sprintf("inner_mount/objects/%v/pack/a/1", aHash)))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0755)), formatFileMode(cacheFileInfo.Mode()))
}

func TestCachedCSIDriverProbeFailsUntilPrepared(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	writePackedFiles(tc, 1, 1, nil, "pack/a")
	writePackedFiles(tc, 1, 1, nil, "pack/b")
	_, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	s := cached.NewCacheCSIServer(tc.Context(), c, path.Join(tmpDir, "cached", "staging"))
	defer s.Grpc.GracefulStop()

	response, err := s.Cached.Probe(tc.Context(), &csi.ProbeRequest{})
	require.NoError(t, err)

	// not ready because we haven't Prepare-d yet
	assert.Equal(t, false, response.Ready.Value)

	require.NoError(t, s.Cached.Prepare(tc.Context()), "cached.Prepare must succeed")

	response, err = s.Cached.Probe(tc.Context(), &csi.ProbeRequest{})
	require.NoError(t, err)

	//  ready because we Prepare-d
	assert.Equal(t, true, response.Ready.Value)
}

func formatFileMode(mode os.FileMode) string {
	return fmt.Sprintf("%#o", mode)
}
