package cached

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charlievieth/fastwalk"
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

// ensurePhysicalVolume creates LVM physical volume if it doesn't exist
func (c *Cached) ensurePhysicalVolume(ctx context.Context) error {
	logger.Debug(ctx, "checking physical volume", zap.String("device", c.LVMDevice))

	err := exec.execLVM(ctx, "pvdisplay", c.LVMDevice)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find physical volume") {
			logger.Info(ctx, "creating physical volume", zap.String("device", c.LVMDevice))
			if err := exec.execLVM(ctx, "pvcreate", c.LVMDevice); err != nil && !strings.Contains(err.Error(), "signal: killed") {
				return fmt.Errorf("failed to create lvm physical volume %s: %w", c.LVMDevice, err)
			}
			return nil
		}
		return fmt.Errorf("failed to check lvm physical volume %s: %w", c.LVMDevice, err)
	}

	logger.Debug(ctx, "physical volume already exists", zap.String("device", c.LVMDevice))
	return nil
}

// ensureVolumeGroup creates LVM volume group if it doesn't exist
func (c *Cached) ensureVolumeGroup(ctx context.Context) error {
	c.lvmVolumeGroup = "vg_dateilager_cached" + strings.ReplaceAll(c.DriverNameSuffix, "-", "_")
	logger.Debug(ctx, "checking volume group", zap.String("vg", c.lvmVolumeGroup))

	err := exec.execLVM(ctx, "vgdisplay", c.lvmVolumeGroup)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			logger.Info(ctx, "creating volume group", zap.String("vg", c.lvmVolumeGroup), zap.String("device", c.LVMDevice))
			if err := exec.execLVM(ctx, "vgcreate", c.lvmVolumeGroup, c.LVMDevice); err != nil {
				return fmt.Errorf("failed to create lvm volume group %s: %w", c.lvmVolumeGroup, err)
			}
			return nil
		}
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVolumeGroup, err)
	}

	logger.Debug(ctx, "volume group already exists", zap.String("vg", c.lvmVolumeGroup))
	return nil
}

// ensureThinPool creates LVM thin pool if it doesn't exist
func (c *Cached) ensureThinPool(ctx context.Context) error {
	thinPoolPath := c.lvmVolumeGroup + "/thinpool"
	logger.Debug(ctx, "checking thin pool", zap.String("thinpool", thinPoolPath))

	err := exec.execLVM(ctx, "lvdisplay", thinPoolPath)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			logger.Info(ctx, "creating thin pool", zap.String("thinpool", thinPoolPath))
			if err := exec.execLVM(ctx, "lvcreate", c.lvmVolumeGroup, "--name=thinpool", "--extents=95%VG", "--thinpool=thinpool"); err != nil {
				return fmt.Errorf("failed to create lvm thin pool %s: %w", thinPoolPath, err)
			}
			return nil
		}
		return fmt.Errorf("failed to check lvm thin pool %s: %w", thinPoolPath, err)
	}

	logger.Debug(ctx, "thin pool already exists", zap.String("thinpool", thinPoolPath))
	return nil
}

// ensureBaseVolume creates base LVM volume if it doesn't exist
func (c *Cached) ensureBaseVolume(ctx context.Context) error {
	basePath := c.lvmVolumeGroup + "/base"
	logger.Debug(ctx, "checking base volume", zap.String("base", basePath))

	err := exec.execLVM(ctx, "lvdisplay", basePath)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			logger.Info(ctx, "creating base volume", zap.String("base", basePath), zap.String("size", c.LVMVirtualSize))
			if err := exec.execLVM(ctx, "lvcreate", "--name=base", "--virtualsize="+c.LVMVirtualSize, "--thinpool="+c.lvmVolumeGroup+"/thinpool"); err != nil {
				return fmt.Errorf("failed to create base volume %s: %w", basePath, err)
			}
			return nil
		}
		return fmt.Errorf("failed to check base volume %s: %w", basePath, err)
	}

	logger.Debug(ctx, "base volume already exists", zap.String("base", basePath))
	return nil
}

// createSnapshot creates an LVM snapshot from the base volume
func (c *Cached) createSnapshot(ctx context.Context, volumeID string) error {
	snapshotPath := c.lvmVolumeGroup + "/" + volumeID
	logger.Debug(ctx, "checking snapshot", zap.String("snapshot", snapshotPath))

	err := exec.execLVM(ctx, "lvdisplay", snapshotPath)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			logger.Info(ctx, "creating snapshot", zap.String("snapshot", snapshotPath))
			if err := exec.execLVM(ctx, "lvcreate", c.lvmVolumeGroup+"/base", "--name="+volumeID, "--snapshot", "--setactivationskip=n"); err != nil {
				return fmt.Errorf("failed to create snapshot of base volume %s: %w", c.lvmVolumeGroup+"/base", err)
			}

			// Wait for device to appear and settle udev
			devicePath := "/dev/" + c.lvmVolumeGroup + "/" + volumeID
			if err := exec.udevSettle(ctx, devicePath); err != nil {
				logger.Warn(ctx, "udev settle failed for snapshot", zap.String("device", devicePath), zap.Error(err))
			}

			return nil
		}
		return fmt.Errorf("failed to check if snapshot %s exists: %w", snapshotPath, err)
	}

	logger.Debug(ctx, "snapshot already exists", zap.String("snapshot", snapshotPath))
	return nil
}

// removeSnapshot removes an LVM snapshot
func (c *Cached) removeSnapshot(ctx context.Context, volumeID string) error {
	snapshotDevice := "/dev/" + c.lvmVolumeGroup + "/" + volumeID
	logger.Debug(ctx, "checking snapshot for removal", zap.String("snapshot", snapshotDevice))

	err := exec.execLVM(ctx, "lvdisplay", "-q", snapshotDevice)
	if err != nil {
		if !strings.Contains(err.Error(), "exit status 5") {
			// exit status 5 means not found, any other error is unexpected
			return fmt.Errorf("failed to check if snapshot %s exists: %w", snapshotDevice, err)
		}
		return nil
	}

	logger.Info(ctx, "removing snapshot", zap.String("snapshot", snapshotDevice))
	if err := exec.execLVM(ctx, "lvremove", "-y", snapshotDevice); err != nil {
		return fmt.Errorf("failed to remove snapshot %s: %w", snapshotDevice, err)
	}
	return nil
}

// setOwnership sets the ownership of a path using os.Chown
func (c *Cached) setOwnership(ctx context.Context, path string) error {
	if c.CacheUid == NO_CHANGE_USER && c.CacheGid == NO_CHANGE_USER {
		return nil
	}

	logger.Debug(ctx, "setting ownership", zap.Int("uid", c.CacheUid), zap.Int("gid", c.CacheGid))
	return fastwalk.Walk(nil, path, func(walkPath string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(walkPath, c.CacheUid, c.CacheGid)
	})
}

// Fetch the cache into the staging dir
func (c *Cached) Prepare(ctx context.Context, cacheVersion int64) error {
	ctx, span := telemetry.Start(ctx, "cached.prepare", trace.WithAttributes(key.CacheVersion.Attribute(cacheVersion)))
	defer span.End()

	start := time.Now()
	logger.Info(ctx, "preparing cached storage", zap.Int64("cacheVersion", cacheVersion))

	// Ensure LVM infrastructure exists
	if err := c.ensurePhysicalVolume(ctx); err != nil {
		return err
	}

	if err := c.ensureVolumeGroup(ctx); err != nil {
		return err
	}

	if err := c.ensureThinPool(ctx); err != nil {
		return err
	}

	if err := c.ensureBaseVolume(ctx); err != nil {
		return err
	}

	// Check if staging path is mounted
	notMounted, err := mounter.IsLikelyNotMountPoint(c.StagingPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to check if staging directory %s is mounted: %w", c.StagingPath, err)
	}

	lvmBaseDevice := "/dev/" + c.lvmVolumeGroup + "/base"
	if notMounted {
		logger.Info(ctx, "mounting base volume", zap.String("device", lvmBaseDevice), zap.String("target", c.StagingPath))

		// Ensure device is available
		if err := exec.udevSettle(ctx, lvmBaseDevice); err != nil {
			logger.Warn(ctx, "udev settle failed for base volume", zap.String("device", lvmBaseDevice), zap.Error(err))
		}

		if err := os.MkdirAll(c.StagingPath, 0o775); err != nil {
			return fmt.Errorf("failed to create staging directory %s: %w", c.StagingPath, err)
		}

		if err := mounter.FormatAndMount(lvmBaseDevice, c.StagingPath, c.LVMFormat, nil); err != nil {
			return fmt.Errorf("failed to mount base volume %s to staging directory %s: %w", lvmBaseDevice, c.StagingPath, err)
		}
	}

	defer func() {
		if unmountErr := mounter.Unmount(c.StagingPath); unmountErr != nil && !errors.Is(unmountErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("failed to unmount staging directory %s: %w", c.StagingPath, unmountErr))
		}
	}()

	// Download cache
	cacheDir := filepath.Join(c.StagingPath, CACHE_PATH_SUFFIX)
	version, count, err := c.Client.GetCache(ctx, cacheDir, cacheVersion)
	if err != nil {
		return err
	}

	span.SetAttributes(key.Count.Attribute(int64(count)))

	// Set ownership if specified
	if err = c.setOwnership(ctx, c.StagingPath); err != nil {
		return fmt.Errorf("failed to change permissions of staging directory %s: %w", c.StagingPath, err)
	}

	c.currentVersion = version
	c.reflinkSupport = files.HasReflinkSupport(cacheDir)

	logger.Info(ctx, "downloaded golden copy", key.DurationMS.Field(time.Since(start)), key.Version.Field(version), key.Count.Field(int64(count)))
	return err
}

func (c *Cached) Unprepare(ctx context.Context) error {
	logger.Info(ctx, "unpreparing cached storage", zap.String("vg", c.lvmVolumeGroup))

	// Remove volume group if it exists
	err := exec.execLVM(ctx, "vgdisplay", c.lvmVolumeGroup)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVolumeGroup, err)
		} else {
			logger.Debug(ctx, "volume group does not exist", zap.String("vg", c.lvmVolumeGroup))
		}
	} else {
		logger.Info(ctx, "removing volume group", zap.String("vg", c.lvmVolumeGroup))
		if err := exec.execLVM(ctx, "vgremove", "-y", c.lvmVolumeGroup); err != nil {
			return fmt.Errorf("failed to remove lvm volume group %s: %w", c.lvmVolumeGroup, err)
		}
	}

	// Remove physical volume if it exists
	err = exec.execLVM(ctx, "pvdisplay", c.LVMDevice)
	if err != nil {
		if !strings.Contains(err.Error(), "Failed to find physical volume") {
			return fmt.Errorf("failed to check lvm physical volume %s: %w", c.LVMDevice, err)
		} else {
			logger.Debug(ctx, "physical volume does not exist", zap.String("device", c.LVMDevice))
		}
	} else {
		logger.Info(ctx, "removing physical volume", zap.String("device", c.LVMDevice))
		if err := exec.execLVM(ctx, "pvremove", c.LVMDevice); err != nil {
			return fmt.Errorf("failed to remove lvm physical volume %s: %w", c.LVMDevice, err)
		}
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

	logger.Info(ctx, "publishing volume", zap.String("volumeID", volumeID), zap.String("targetPath", targetPath))

	// Create LVM snapshot
	if err := c.createSnapshot(ctx, volumeID); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create snapshot: %v", err)
	}

	// Check if already mounted to avoid double mounting
	notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "failed to check if target path %s is mounted: %v", targetPath, err)
	}

	if notMounted {
		// Create target directory
		if err := os.MkdirAll(targetPath, 0o775); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create target path %s: %v", targetPath, err)
		}

		// Mount the snapshot
		lvmSnapshotDevice := "/dev/" + c.lvmVolumeGroup + "/" + volumeID

		// Ensure device is available before mounting
		if err := exec.udevSettle(ctx, lvmSnapshotDevice); err != nil {
			logger.Warn(ctx, "udev settle failed for snapshot mount", zap.String("device", lvmSnapshotDevice), zap.Error(err))
		}

		logger.Info(ctx, "mounting snapshot", zap.String("device", lvmSnapshotDevice), zap.String("target", targetPath))
		if err := mounter.Mount(lvmSnapshotDevice, targetPath, c.LVMFormat, nil); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to mount snapshot %s to %s: %v", lvmSnapshotDevice, targetPath, err)
		}
	} else {
		logger.Debug(ctx, "target path already mounted", zap.String("targetPath", targetPath))
	}

	// Set permissions on target path
	if err := os.Chmod(targetPath, 0o775); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to change permissions of target path %s: %v", targetPath, err)
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

	logger.Info(ctx, "unpublishing volume", zap.String("volumeID", volumeID), zap.String("targetPath", targetPath))

	// Check if mounted before attempting unmount
	notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "failed to check if target path %s is mounted: %v", targetPath, err)
	}

	// Unmount if mounted
	if !notMounted {
		logger.Info(ctx, "unmounting target path", zap.String("targetPath", targetPath))
		if err := mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmount lvm snapshot at %s: %v", targetPath, err)
		}
	} else {
		logger.Debug(ctx, "target path not mounted", zap.String("targetPath", targetPath))
	}

	// Remove snapshot
	if err := c.removeSnapshot(ctx, volumeID); err != nil {
		logger.Warn(ctx, "failed to remove snapshot", zap.String("volumeID", volumeID), zap.Error(err))
		// Don't return error here as the unmount succeeded - log and continue
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
