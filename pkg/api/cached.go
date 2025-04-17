package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	DRIVER_NAME       = "dev.gadget.dateilager.cached"
	CACHE_PATH_SUFFIX = "dl_cache"
	UPPER_DIR         = "upper"
	WORK_DIR          = "work"
	NO_CHANGE_USER    = -1
)

type Cached struct {
	pb.UnimplementedCachedServer
	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer

	Env environment.Env

	Client           *client.Client
	DriverNameSuffix string
	StagingPath      string
	CacheUid         int
	CacheGid         int

	// the current version of the cache on disk
	currentVersion int64
	reflinkSupport bool
}

func (c *Cached) PopulateDiskCache(ctx context.Context, req *pb.PopulateDiskCacheRequest) (*pb.PopulateDiskCacheResponse, error) {
	if c.Env != environment.Dev && c.Env != environment.Test {
		return nil, status.Errorf(codes.Unimplemented, "Cached populateDiskCache only implemented in dev and test environments")
	}

	destination := req.Path

	version, err := c.writeCache(destination)
	if err != nil {
		return nil, err
	}

	return &pb.PopulateDiskCacheResponse{Version: version}, nil
}

func (c *Cached) GetCachePath() string {
	return filepath.Join(c.StagingPath, CACHE_PATH_SUFFIX)
}

// Fetch the cache into the staging dir
func (c *Cached) Prepare(ctx context.Context, cacheVersion int64) error {
	start := time.Now()

	version, count, err := c.Client.GetCache(ctx, c.GetCachePath(), cacheVersion)
	if err != nil {
		return err
	}

	if c.CacheUid != NO_CHANGE_USER || c.CacheGid != NO_CHANGE_USER {
		// make the cache owned by the provided uid and gid
		err = fastwalk.Walk(nil, c.GetCachePath(), func(path string, de os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if de.Type() == os.ModeSymlink {
				return nil // symlinks are recreated so we don't need to chown them
			}
			if os.Getenv("RUN_WITH_SUDO") != "" {
				return execCommand("chown", fmt.Sprintf("%d:%d", c.CacheUid, c.CacheGid), path)
			} else {
				return os.Chown(path, c.CacheUid, c.CacheGid)
			}
		})
		if err != nil {
			return fmt.Errorf("failed to chown cache path %s: %v", c.GetCachePath(), err)
		}
	}

	c.currentVersion = version
	c.reflinkSupport = files.HasReflinkSupport(c.GetCachePath())

	logger.Info(ctx, "downloaded golden copy", key.DurationMS.Field(time.Since(start)), key.Version.Field(version), key.Count.Field(int64(count)))
	return nil
}

// GetPluginInfo returns metadata of the plugin
func (c *Cached) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	resp := &csi.GetPluginInfoResponse{
		Name:          DRIVER_NAME + c.DriverNameSuffix,
		VendorVersion: version.Version,
	}

	return resp, nil
}

// GetPluginCapabilities returns available capabilities of the plugin
func (c *Cached) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	resp := &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{},
	}

	return resp, nil
}

// Probe returns the health and readiness of the plugin
func (c *Cached) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	ready := true
	if c.currentVersion == 0 {
		ready = false
		logger.Warn(ctx, "csi probe failed as daemon hasn't prepared cache yet", key.Version.Field(c.currentVersion))
	}

	return &csi.ProbeResponse{
		Ready: &wrappers.BoolValue{
			Value: ready,
		},
	}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server
// this driver has no capabilities like expansion or staging, because we only use it for node local volumes
func (c *Cached) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	nscaps := []*csi.NodeServiceCapability{}

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: nscaps,
	}, nil
}

// NodeGetInfo returns the supported capabilities of the node server. This
// Usually, a CSI driver would return some interesting stuff about the node here for the controller to use to place volumes, but because we're only supporting node local volumes, we return something very basic
func (c *Cached) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId:            first(os.Getenv("NODE_NAME"), "dev"),
		MaxVolumesPerNode: 110,
	}, nil
}

func (c *Cached) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target Path must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume Capability must be provided")
	}

	targetPath := req.GetTargetPath()                    // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount
	volumePath := path.Join(targetPath, "..")            // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a
	upperDir := path.Join(volumePath, UPPER_DIR)         // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/upper
	workDir := path.Join(volumePath, WORK_DIR)           // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/work
	cacheDir := path.Join(targetPath, CACHE_PATH_SUFFIX) // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount/dl_cache

	if err := mkdirAll(upperDir, 0o777); err != nil {
		return nil, fmt.Errorf("failed to create overlay upper directory %s: %w", upperDir, err)
	}

	if err := mkdirAll(workDir, 0o777); err != nil {
		return nil, fmt.Errorf("failed to create overlay work directory %s: %w", workDir, err)
	}

	// Create the cache directory and make it writable by the pod
	if err := os.MkdirAll(targetPath, 0o777); err != nil {
		return nil, fmt.Errorf("failed to create target path directory %s: %w", targetPath, err)
	}

	mountArgs := []string{
		"-t",
		"overlay",
		"overlay",
		"-n",
		"--options",
		fmt.Sprintf("redirect_dir=on,volatile,lowerdir=%s,upperdir=%s,workdir=%s", c.StagingPath, upperDir, workDir),
		targetPath,
	}

	if err := execCommand("mount", mountArgs...); err != nil {
		return nil, fmt.Errorf("failed to mount overlay at %s: %w", targetPath, err)
	}

	if err := mkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache path %s: %w", cacheDir, err)
	}

	logger.Info(ctx, "mounted overlay", key.TargetPath.Field(targetPath), key.Version.Field(c.currentVersion))
	return &csi.NodePublishVolumeResponse{}, nil
}

func (s *Cached) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	targetPath := req.GetTargetPath()            // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount
	volumePath := path.Join(targetPath, "..")    // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a
	upperDir := path.Join(volumePath, UPPER_DIR) // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/upper
	workDir := path.Join(volumePath, WORK_DIR)   // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/work

	// Check if the volume path exists
	_, err := os.Stat(volumePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &csi.NodeUnpublishVolumeResponse{}, nil // Nothing for us to do
		}
		return nil, fmt.Errorf("failed to stat volume path %s: %w", volumePath, err)
	}

	// Unmount the overlay and use -l to make it not throw if it's not mounted (i.e. make it idempotent)
	if err := execCommand("umount", "-l", targetPath); err != nil {
		return nil, fmt.Errorf("failed to unmount overlay at %s: %w", targetPath, err)
	}

	// Clean up upper directory from the overlay
	if err := removeAll(upperDir); err != nil {
		return nil, fmt.Errorf("failed to remove upper directory %s: %w", upperDir, err)
	}

	// Clean up work directory from the overlay
	if err := removeAll(workDir); err != nil {
		return nil, fmt.Errorf("failed to remove work directory %s: %w", workDir, err)
	}

	logger.Info(ctx, "volume unpublished and data removed", key.TargetPath.Field(targetPath))
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetVolumeStats returns the volume capacity statistics available for the given volume.
func (c *Cached) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume ID must be provided")
	}

	volumePath := req.VolumePath
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume Path must be provided")
	}

	usedBytes, err := getFolderSize(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve used size statistics for volume path %s: %v", volumePath, err)
	}

	var stat syscall.Statfs_t
	err = syscall.Statfs(volumePath, &stat)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve total size statistics for volume path %s: %v", volumePath, err)
	}

	// Calculate free space in bytes
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	if freeBytes > math.MaxInt64 {
		return nil, status.Errorf(codes.Internal, "total size statistics for volume path too big for int64: %d", freeBytes)
	}
	signedFreeBytes := int64(freeBytes)

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: signedFreeBytes,
				Total:     signedFreeBytes + usedBytes,
				Used:      usedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
		},
	}, nil
}

// check if the destination exists, and if so, if its writable
// hardlink the golden copy into this downstream's destination, creating it if need be
func (c *Cached) writeCache(destination string) (int64, error) {
	if c.currentVersion == 0 {
		return -1, errors.New("no cache prepared, currentDir is nil")
	}

	stat, err := os.Stat(destination)
	if !os.IsNotExist(err) {
		if err != nil {
			return -1, fmt.Errorf("failed to stat cache destination %s: %v", destination, err)
		}

		if !stat.IsDir() {
			return -1, fmt.Errorf("failed to open cache destination %s for writing -- it is already a file", destination)
		}

		if unix.Access(destination, unix.W_OK) != nil {
			return -1, fmt.Errorf("failed to open cache destination %s for writing -- write permission denied", destination)
		}
	}

	if c.reflinkSupport {
		err = files.Reflink(c.GetCachePath(), destination)
	} else {
		err = files.Hardlink(c.GetCachePath(), destination)
	}

	if err != nil {
		return -1, fmt.Errorf("failed to hardlink cache to destination %s: %v", destination, err)
	}
	return c.currentVersion, nil
}

func first(one, two string) string {
	if one == "" {
		return two
	}
	return one
}

func getFolderSize(path string) (int64, error) {
	var totalSize int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	return totalSize, err
}

func execCommand(cmdName string, args ...string) error {
	if os.Getenv("RUN_WITH_SUDO") != "" {
		cmd := exec.Command("sudo", append([]string{cmdName}, args...)...)
		return cmd.Run()
	}

	cmd := exec.Command(cmdName, args...)
	return cmd.Run()
}

func mkdirAll(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat directory %s: %w", path, err)
	}

	if info.Mode()&os.ModePerm != mode {
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("failed to change permissions of directory %s: %w", path, err)
		}
	}

	return nil
}

func removeAll(path string) error {
	if os.Getenv("RUN_WITH_SUDO") != "" {
		return execCommand("sudo", "rm", "-rf", path)
	}
	return os.RemoveAll(path)
}
