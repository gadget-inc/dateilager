package cached

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	DRIVER_NAME       = "dev.gadget.dateilager.cached"
	CACHE_PATH_SUFFIX = "dl_cache"
	NO_CHANGE_USER    = -1
)

type Cached struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer

	Env environment.Env

	Client           *client.Client
	DriverNameSuffix string
	StagingPath      string
	CacheUid         int
	CacheGid         int

	LVMDevice      string
	LVMFormat      string
	LVMVirtualSize string

	// the current version of the cache on disk
	currentVersion int64
	reflinkSupport bool
	lvmVolumeGroup string
}

func (c *Cached) GetCachePath() string {
	return filepath.Join(c.StagingPath, CACHE_PATH_SUFFIX)
}

// Fetch the cache into the staging dir
func (c *Cached) Prepare(ctx context.Context, cacheVersion int64) error {
	ctx, span := telemetry.Start(ctx, "cached.prepare", trace.WithAttributes(key.CacheVersion.Attribute(cacheVersion)))
	defer span.End()

	start := time.Now()

	// check if the device is already a physical volume
	err := execCommand("pvdisplay", c.LVMDevice)
	switch {
	case err == nil:
		// the device is already a physical volume, so we can skip pvcreate
	case strings.Contains(err.Error(), "Failed to find physical volume"):
		// the device is not a physical volume, so we need to create it
		if err = execCommand("pvcreate", c.LVMDevice); err != nil {
			return fmt.Errorf("failed to create lvm physical volume %s: %w", c.LVMDevice, err)
		}
	default:
		// if the error is not about the physical volume not being found, then it's an unexpected error
		return fmt.Errorf("failed to check lvm physical volume %s: %w", c.LVMDevice, err)
	}

	c.lvmVolumeGroup = "vg_dateilager_cached" + strings.ReplaceAll(c.DriverNameSuffix, "-", "_")
	err = execCommand("vgdisplay", c.lvmVolumeGroup)
	switch {
	case err == nil:
		// the volume group already exists, so we can skip vgcreate
	case strings.Contains(err.Error(), "not found"):
		// the volume group does not exist, so we need to create it
		if err = execCommand("vgcreate", c.lvmVolumeGroup, c.LVMDevice); err != nil {
			return fmt.Errorf("failed to create lvm volume group %s: %w", c.LVMDevice, err)
		}
	default:
		// if the error is not about the volume group not being found, then it's an unexpected error
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVolumeGroup, err)
	}

	err = execCommand("lvdisplay", c.lvmVolumeGroup+"/thinpool")
	switch {
	case err == nil:
		// the thin pool already exists, so we can skip lvcreate
	case strings.Contains(err.Error(), "Failed to find logical volume"):
		if err = execCommand("lvcreate", c.lvmVolumeGroup, "--name=thinpool", "--extents=95%VG", "--thinpool=thinpool"); err != nil {
			return fmt.Errorf("failed to create lvm thin pool %s: %w", c.lvmVolumeGroup+"/thinpool", err)
		}
	default:
		// if the error is not about the thin pool not being found, then it's an unexpected error
		return fmt.Errorf("failed to check lvm thin pool %s: %w", c.lvmVolumeGroup+"/thinpool", err)
	}

	err = execCommand("lvdisplay", c.lvmVolumeGroup+"/base")
	switch {
	case err == nil:
		// the base volume already exists, so we can skip lvcreate
	case strings.Contains(err.Error(), "Failed to find logical volume"):
		// the base volume does not exist, so we need to create it
		if err = execCommand("lvcreate", "--name=base", "--virtualsize="+c.LVMVirtualSize, "--thinpool="+c.lvmVolumeGroup+"/thinpool"); err != nil {
			return fmt.Errorf("failed to create base volume %s: %w", c.lvmVolumeGroup+"/base", err)
		}
	default:
		// if the error is not about the base volume not being found, then it's an unexpected error
		return fmt.Errorf("failed to check base volume %s: %w", c.lvmVolumeGroup+"/base", err)
	}

	lvmBaseDir := "/dev/" + c.lvmVolumeGroup + "/base"
	err = execCommand("blkid", lvmBaseDir)
	switch {
	case err == nil:
		// the base volume is already formatted, so we can skip mkfs
	case strings.Contains(err.Error(), "exit status 2"):
		// the base volume is not formatted, so we need to format it
		if err = execCommand("mkfs."+c.LVMFormat, lvmBaseDir); err != nil {
			return fmt.Errorf("failed to format base volume %s: %w", lvmBaseDir, err)
		}
	default:
		// if the error is not about the base volume not exit status 2, then it's an unexpected error
		return fmt.Errorf("failed to check if base volume %s is formatted: %w", lvmBaseDir, err)
	}

	if err = execCommand("mount", fmt.Sprintf("--mkdir=%o", 0o777), lvmBaseDir, c.StagingPath); err != nil {
		return fmt.Errorf("failed to mount base volume %s to staging directory %s: %w", lvmBaseDir, c.StagingPath, err)
	}

	defer func() {
		if unmountErr := execCommand("umount", c.StagingPath); unmountErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to unmount staging directory %s: %w", c.StagingPath, unmountErr))
		}
	}()

	if c.CacheUid != NO_CHANGE_USER || c.CacheGid != NO_CHANGE_USER {
		// make the staging directory owned by the provided uid and gid
		if err = execCommand("chown", fmt.Sprintf("%d:%d", c.CacheUid, c.CacheGid), c.StagingPath); err != nil {
			return fmt.Errorf("failed to change permissions of cache directory %s: %w", c.StagingPath, err)
		}

		defer func() {
			if chownErr := execCommand("chown", "-R", fmt.Sprintf("%d:%d", c.CacheUid, c.CacheGid), c.StagingPath); chownErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to recursively change permissions of cache directory %s: %w", c.StagingPath, chownErr))
			}
		}()
	}

	cacheDir := c.GetCachePath()
	version, count, err := c.Client.GetCache(ctx, cacheDir, cacheVersion)
	if err != nil {
		return err
	}

	span.SetAttributes(key.Count.Attribute(int64(count)))

	c.currentVersion = version
	c.reflinkSupport = files.HasReflinkSupport(cacheDir)

	logger.Info(ctx, "downloaded golden copy", key.DurationMS.Field(time.Since(start)), key.Version.Field(version), key.Count.Field(int64(count)))
	return err
}

func (c *Cached) Unprepare(ctx context.Context) error {
	err := execCommand("vgdisplay", c.lvmVolumeGroup)
	switch {
	case err == nil:
		// the volume group exists, so we need to remove it
		if err := execCommand("vgremove", "-y", c.lvmVolumeGroup); err != nil {
			return fmt.Errorf("failed to remove lvm volume group %s: %w", c.lvmVolumeGroup, err)
		}
	case strings.Contains(err.Error(), "not found"):
		// the volume group does not exist, so we can skip vgremove
	default:
		// if the error is not about the volume group not being found, then it's an unexpected error
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVolumeGroup, err)
	}

	err = execCommand("pvdisplay", c.LVMDevice)
	switch {
	case err == nil:
		// the device is a physical volume, so we need to remove it
		if err := execCommand("pvremove", c.LVMDevice); err != nil {
			return fmt.Errorf("failed to remove lvm physical volume %s: %w", c.LVMDevice, err)
		}
	case strings.Contains(err.Error(), "Failed to find physical volume"):
		// the device is not a physical volume, so we can skip pvremove
	default:
		// if the error is not about the physical volume not being found, then it's an unexpected error
		return fmt.Errorf("failed to check lvm physical volume %s: %w", c.LVMDevice, err)
	}

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
		NodeId:            first(os.Getenv("NODE_ID"), os.Getenv("NODE_NAME"), os.Getenv("K8S_NODE_NAME"), "dev"),
		MaxVolumesPerNode: 110,
	}, nil
}

func (c *Cached) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID must be provided")
	}

	targetPath := req.GetTargetPath() // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target Path must be provided")
	}

	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume Capability must be provided")
	}

	trace.SpanFromContext(ctx).SetAttributes(
		key.VolumeID.Attribute(volumeID),
		key.TargetPath.Attribute(targetPath),
	)

	err := execCommand("lvdisplay", c.lvmVolumeGroup+"/"+volumeID)
	switch {
	case err == nil:
		// the snapshot already exists, so we can skip lvcreate
	case strings.Contains(err.Error(), "Failed to find logical volume"):
		// the snapshot does not exist, so we need to create it
		if err := execCommand("lvcreate", c.lvmVolumeGroup+"/base", "--name="+volumeID, "--snapshot", "--setactivationskip=n"); err != nil {
			return nil, fmt.Errorf("failed to create snapshot of base volume %s: %w", c.lvmVolumeGroup+"/base", err)
		}
	default:
		// if the error is not about the snapshot not being found, then it's an unexpected error
		return nil, fmt.Errorf("failed to check if snapshot %s exists: %w", c.lvmVolumeGroup+"/"+volumeID, err)
	}

	if err := mkdirAll(targetPath, 0o777); err != nil {
		return nil, fmt.Errorf("failed to create target path %s: %w", targetPath, err)
	}

	err = execCommand("mountpoint", "-q", targetPath)
	switch {
	case err == nil:
		// the target path is already mounted, so we can skip mount
	case strings.Contains(err.Error(), "exit status 32"):
		// the target path is not mounted, so we need to mount it
		snapshotPath := "/dev/" + c.lvmVolumeGroup + "/" + volumeID
		if err := execCommand("mount", snapshotPath, targetPath); err != nil {
			return nil, fmt.Errorf("failed to mount snapshot %s to %s: %w", snapshotPath, targetPath, err)
		}
	default:
		// if the error is not about the target path not being mounted, then it's an unexpected error
		return nil, fmt.Errorf("failed to check if target path %s is mounted: %w", targetPath, err)
	}

	if err := os.Chmod(targetPath, 0o777); err != nil {
		return nil, fmt.Errorf("failed to change permissions of target path %s: %w", targetPath, err)
	}

	logger.Info(ctx, "mounted snapshot", key.TargetPath.Field(targetPath), key.Version.Field(c.currentVersion))
	return &csi.NodePublishVolumeResponse{}, nil
}

func (c *Cached) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}

	targetPath := req.GetTargetPath() // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	trace.SpanFromContext(ctx).SetAttributes(
		key.VolumeID.Attribute(volumeID),
		key.TargetPath.Attribute(targetPath),
	)

	// Check if the target path exists
	_, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &csi.NodeUnpublishVolumeResponse{}, nil // Nothing for us to do
		}
		return nil, fmt.Errorf("failed to stat target path %s: %w", targetPath, err)
	}

	// Check if the snapshot is mounted
	err = execCommand("mountpoint", "-q", targetPath)
	if err == nil {
		// The snapshot is mounted, so we need to unmount it
		if err := execCommand("umount", targetPath); err != nil {
			return nil, fmt.Errorf("failed to unmount snapshot at %s: %w", targetPath, err)
		}
	} else if !strings.Contains(err.Error(), "exit status 32") {
		// exit status 32 means not mounted, so if it's not 32, then it's an unexpected error
		// See: https://man7.org/linux/man-pages/man1/mountpoint.1.html
		return nil, fmt.Errorf("failed to check if snapshot is mounted at %s: %w", targetPath, err)
	}

	// Check if the snapshot exists
	snapshotPath := "/dev/" + c.lvmVolumeGroup + "/" + volumeID
	err = execCommand("lvdisplay", "-q", snapshotPath)
	if err == nil {
		// The snapshot exists, so we need to remove it
		if err := execCommand("lvremove", "-y", snapshotPath); err != nil {
			return nil, fmt.Errorf("failed to remove snapshot %s: %w", snapshotPath, err)
		}
	} else if !strings.Contains(err.Error(), "exit status 5") {
		// exit status 5 means not found, so if it's not 5, then it's an unexpected error
		return nil, fmt.Errorf("failed to check if snapshot %s exists: %w", snapshotPath, err)
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

func first(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
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

func execCommand(command string, args ...string) error {
	var cmd *exec.Cmd
	if os.Getenv("RUN_WITH_SUDO") != "" {
		cmd = exec.Command("sudo", append([]string{command}, args...)...)
	} else {
		cmd = exec.Command(command, args...)
	}

	logger.Debug(context.TODO(), "executing command", zap.String("command", cmd.String()))

	bs, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command %s: %w: %s", cmd.String(), err, string(bs))
	}

	return nil
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
