//go:build integration
// +build integration

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

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cached, endpoint, close := createTestCachedServer(tc, tmpDir)
	defer close()

	require.NoError(t, cached.Prepare(tc.Context(), -1))
	defer func() {
		assert.NoError(t, cached.Unprepare(tc.Context()))
	}()

	sanityPath := path.Join(tmpDir, "csi")
	require.NoError(t, os.MkdirAll(sanityPath, 0o755), "couldn't make staging path")

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

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cd, _, close := createTestCachedServer(tc, tmpDir)
	defer close()

	require.NoError(t, cd.Prepare(tc.Context(), -1))
	defer func() {
		assert.NoError(t, cd.Unprepare(tc.Context()))
	}()

	targetDir := path.Join(tmpDir, "vol-target")

	_, err = cd.NodePublishVolume(tc.Context(), &csi.NodePublishVolumeRequest{
		VolumeId:          "foobar",
		StagingTargetPath: path.Join(tmpDir, "vol-staging-target"),
		TargetPath:        targetDir,
		VolumeCapability:  &csi.VolumeCapability{},
	})
	require.NoError(t, err)

	defer func() {
		_, err = cd.NodeUnpublishVolume(tc.Context(), &csi.NodeUnpublishVolumeRequest{
			VolumeId:   "foobar",
			TargetPath: targetDir,
		})
		assert.NoError(t, err, "NodeUnpublishVolume should succeed")
	}()

	uid := os.Getuid()
	gid := os.Getgid()
	verifyDir(t, path.Join(targetDir, "dl_cache"), -1, map[string]expectedFile{
		fmt.Sprintf("objects/%v/pack/a/1", aHash): {content: "pack/a/1 v1", uid: uid, gid: gid},
		fmt.Sprintf("objects/%v/pack/a/2", aHash): {content: "pack/a/2 v1", uid: uid, gid: gid},
		fmt.Sprintf("objects/%v/pack/b/1", bHash): {content: "pack/b/1 v1", uid: uid, gid: gid},
		fmt.Sprintf("objects/%v/pack/b/2", bHash): {content: "pack/b/2 v1", uid: uid, gid: gid},
		"versions": {content: fmt.Sprintf("%v\n", version)},
	})

	fileInfo, err := os.Stat(targetDir)
	require.NoError(t, err)

	// the target dir should not be world writable -- only by the user the CSI driver is running as (which will be root)
	require.Equal(t, formatFileMode(os.FileMode(0o775)), formatFileMode(fileInfo.Mode()&os.ModePerm))

	// files inside cache dir should also *not* be writable -- it's managed by the CSI and must remain pristine
	cacheFileInfo, err := os.Stat(path.Join(targetDir, "dl_cache", fmt.Sprintf("objects/%v/pack/a/1", aHash)))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0o755)), formatFileMode(cacheFileInfo.Mode()&os.ModePerm))

	_, err = cd.NodeUnpublishVolume(tc.Context(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "foobar",
		TargetPath: targetDir,
	})
	require.NoError(t, err)
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

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cd, _, close := createTestCachedServer(tc, tmpDir)
	defer close()

	require.NoError(t, cd.Prepare(tc.Context(), -1))
	defer func() {
		assert.NoError(t, cd.Unprepare(tc.Context()))
	}()

	targetDir := path.Join(tmpDir, "vol-target")

	stagingTargetDir := path.Join(tmpDir, "vol-staging-target")
	_, err = cd.NodePublishVolume(tc.Context(), &csi.NodePublishVolumeRequest{
		VolumeId:          "foobar",
		StagingTargetPath: stagingTargetDir,
		TargetPath:        targetDir,
		VolumeCapability:  &csi.VolumeCapability{},
		VolumeContext:     map[string]string{},
	})
	require.NoError(t, err)

	defer func() {
		_, err = cd.NodeUnpublishVolume(tc.Context(), &csi.NodeUnpublishVolumeRequest{
			VolumeId:   "foobar",
			TargetPath: targetDir,
		})
		assert.NoError(t, err)
	}()

	verifyDir(t, path.Join(tmpDir, "vol-target"), -1, map[string]expectedFile{
		fmt.Sprintf("dl_cache/objects/%v/pack/a/1", aHash): {content: "pack/a/1 v1"},
		fmt.Sprintf("dl_cache/objects/%v/pack/a/2", aHash): {content: "pack/a/2 v1"},
		fmt.Sprintf("dl_cache/objects/%v/pack/b/1", bHash): {content: "pack/b/1 v1"},
		fmt.Sprintf("dl_cache/objects/%v/pack/b/2", bHash): {content: "pack/b/2 v1"},
		"dl_cache/versions": {content: fmt.Sprintf("%v\n", version)},
	})

	// the target dir *should* be writable -- we're going to use it as a scratch space to do useful stuff with the cache
	fileInfo, err := os.Stat(targetDir)
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0o775)), formatFileMode(fileInfo.Mode()&os.ModePerm))

	// the cache dir should *not* be writable -- it's managed by the CSI and must remain pristine
	cacheFileInfo, err := os.Stat(path.Join(targetDir, "dl_cache"))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0o755)), formatFileMode(cacheFileInfo.Mode()&os.ModePerm))

	// the versions file should *not* be writable -- it's managed by the CSI -- it *should* be world readable though so other users know which versions are available
	versionFileInfo, err := os.Stat(path.Join(targetDir, "dl_cache", "versions"))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0o644)), formatFileMode(versionFileInfo.Mode()&os.ModePerm))

	// files inside cache dir should *not* be writable -- it's managed by the CSI and must remain pristine
	cacheFileInfo, err = os.Stat(path.Join(targetDir, fmt.Sprintf("dl_cache/objects/%v/pack/a/1", aHash)))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0o755)), formatFileMode(cacheFileInfo.Mode()))
	require.Equal(t, targetDir, path.Join(tmpDir, "vol-target"))
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

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cd, _, close := createTestCachedServer(tc, tmpDir)
	defer close()

	response, err := cd.Probe(tc.Context(), &csi.ProbeRequest{})
	require.NoError(t, err)

	// not ready because we haven't Prepare-d yet
	assert.Equal(t, false, response.Ready.Value)

	require.NoError(t, cd.Prepare(tc.Context(), -1))
	defer func() {
		assert.NoError(t, cd.Unprepare(tc.Context()))
	}()

	response, err = cd.Probe(tc.Context(), &csi.ProbeRequest{})
	require.NoError(t, err)

	// ready because we Prepare-d
	assert.Equal(t, true, response.Ready.Value)
}

func TestCachedCSIDriverMountCanDoAFullRebuild(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 4)
	writeObject(tc, 1, 1, nil, "a", "a v1")
	writeObject(tc, 1, 2, i(3), "b", "b v2")
	writeObject(tc, 1, 3, i(4), "b/c", "b/c v3")
	writeObject(tc, 1, 3, nil, "b/d", "b/d v3")
	writeObject(tc, 1, 4, nil, "b/e", "b/e v4")
	writePackedFiles(tc, 1, 4, nil, "pack/a")
	writePackedFiles(tc, 1, 4, nil, "pack/b")
	writePackedFiles(tc, 1, 4, nil, "pack/", map[string]expectedObject{ // add packed files without a subdir and a symlink
		"1link": {content: "./1", mode: symlinkMode},
	})
	_, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cd, _, close := createTestCachedServer(tc, tmpDir)
	defer close()

	require.NoError(t, cd.Prepare(tc.Context(), -1))
	defer func() {
		assert.NoError(t, cd.Unprepare(tc.Context()))
	}()

	targetDir := path.Join(tmpDir, "vol-target")

	stagingDir := path.Join(tmpDir, "vol-staging-target")
	_, err = cd.NodePublishVolume(tc.Context(), &csi.NodePublishVolumeRequest{
		VolumeId:          "foobar",
		StagingTargetPath: stagingDir,
		TargetPath:        targetDir,
		VolumeCapability:  &csi.VolumeCapability{},
		VolumeContext:     map[string]string{},
	})
	require.NoError(t, err)

	c, _, close := createTestClient(tc)
	defer close()

	appDir := path.Join(targetDir, "app")
	dlCacheDir := path.Join(targetDir, "dl_cache")

	// Do the initial build
	rebuild(tc, c, 1, i(1), appDir, &dlCacheDir, expectedResponse{
		version: 1,
		count:   1,
	}, nil)

	rebuild(tc, c, 1, nil, appDir, &dlCacheDir, expectedResponse{
		version: 4,
		count:   5,
	}, nil)

	verifyDir(t, appDir, 4, map[string]expectedFile{
		"a":          {content: "a v1"},
		"b/d":        {content: "b/d v3"},
		"b/e":        {content: "b/e v4"},
		"pack/a/1":   {content: "pack/a/1 v4"},
		"pack/a/2":   {content: "pack/a/2 v4"},
		"pack/b/1":   {content: "pack/b/1 v4"},
		"pack/b/2":   {content: "pack/b/2 v4"},
		"pack/1":     {content: "pack/1 v4"},
		"pack/2":     {content: "pack/2 v4"},
		"pack/1link": {content: "./1", fileType: typeSymlink},
	})

	_, err = cd.NodeUnpublishVolume(tc.Context(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "foobar",
		TargetPath: targetDir,
	})
	require.NoError(t, err)
}

func TestCachedCSIDriverIdempotency(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "a", "a v1")
	aHash := writePackedFiles(tc, 1, 1, nil, "pack/a")
	bHash := writePackedFiles(tc, 1, 1, nil, "pack/b")
	version, err := db.CreateCache(tc.Context(), tc.Connect(), "", 100)
	require.NoError(t, err)

	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	cd, _, close := createTestCachedServer(tc, tmpDir)
	defer close()

	require.NoError(t, cd.Prepare(tc.Context(), -1))
	defer func() {
		assert.NoError(t, cd.Unprepare(tc.Context()))
	}()

	require.NoError(t, cd.Prepare(tc.Context(), -1))
	defer func() {
		assert.NoError(t, cd.Unprepare(tc.Context()))
	}()

	targetDir := path.Join(tmpDir, "vol-target")

	_, err = cd.NodePublishVolume(tc.Context(), &csi.NodePublishVolumeRequest{
		VolumeId:          "foobar",
		StagingTargetPath: path.Join(tmpDir, "vol-staging-target"),
		TargetPath:        targetDir,
		VolumeCapability:  &csi.VolumeCapability{},
	})
	require.NoError(t, err, "NodePublishVolume 1 must succeed")

	_, err = cd.NodePublishVolume(tc.Context(), &csi.NodePublishVolumeRequest{
		VolumeId:          "foobar",
		StagingTargetPath: path.Join(tmpDir, "vol-staging-target"),
		TargetPath:        targetDir,
		VolumeCapability:  &csi.VolumeCapability{},
	})
	require.NoError(t, err, "NodePublishVolume 2 must succeed")

	uid := os.Getuid()
	gid := os.Getgid()
	verifyDir(t, path.Join(targetDir, "dl_cache"), -1, map[string]expectedFile{
		fmt.Sprintf("objects/%v/pack/a/1", aHash): {content: "pack/a/1 v1", uid: uid, gid: gid},
		fmt.Sprintf("objects/%v/pack/a/2", aHash): {content: "pack/a/2 v1", uid: uid, gid: gid},
		fmt.Sprintf("objects/%v/pack/b/1", bHash): {content: "pack/b/1 v1", uid: uid, gid: gid},
		fmt.Sprintf("objects/%v/pack/b/2", bHash): {content: "pack/b/2 v1", uid: uid, gid: gid},
		"versions": {content: fmt.Sprintf("%v\n", version)},
	})

	fileInfo, err := os.Stat(targetDir)
	require.NoError(t, err)

	// the target dir should not be world writable -- only by the user the CSI driver is running as (which will be root)
	require.Equal(t, formatFileMode(os.FileMode(0o775)), formatFileMode(fileInfo.Mode()&os.ModePerm))

	// files inside cache dir should also *not* be writable -- it's managed by the CSI and must remain pristine
	cacheFileInfo, err := os.Stat(path.Join(targetDir, "dl_cache", fmt.Sprintf("objects/%v/pack/a/1", aHash)))
	require.NoError(t, err)
	require.Equal(t, formatFileMode(os.FileMode(0o755)), formatFileMode(cacheFileInfo.Mode()&os.ModePerm))

	_, err = cd.NodeUnpublishVolume(tc.Context(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "foobar",
		TargetPath: targetDir,
	})
	require.NoError(t, err, "NodeUnpublishVolume 1 must succeed")

	_, err = cd.NodeUnpublishVolume(tc.Context(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "foobar",
		TargetPath: targetDir,
	})
	require.NoError(t, err, "NodeUnpublishVolume 2 must succeed")
}

func formatFileMode(mode os.FileMode) string {
	return fmt.Sprintf("%#o", mode)
}

func createTestCachedServer(tc util.TestCtx, tmpDir string) (*cached.Cached, string, func()) {
	cl, _, closeClient := createTestClient(tc)
	_, grpcServer, _ := createTestGRPCServer(tc)

	s := cached.CachedServer{
		Grpc: grpcServer,
	}

	cd := cached.New(cl, "test")
	cd.BaseLVMountPoint = path.Join(tmpDir, "mnt/base")
	s.RegisterCSI(cd)

	socket := path.Join(tmpDir, "csi.sock")
	endpoint := "unix://" + socket

	go func() {
		err := s.Serve(endpoint)
		require.NoError(tc.T(), err, "CSI Server exited")
	}()

	return cd, endpoint, func() { closeClient(); s.Grpc.Stop() }
}
