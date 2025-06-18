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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/container-storage-interface/spec/lib/go/csi"
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
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

const (
	DRIVER_NAME       = "dev.gadget.dateilager.cached"
	CACHE_PATH_SUFFIX = "dl_cache"
	NO_CHANGE_USER    = -1
)

type Cached struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer
	Client           *client.Client
	DriverNameSuffix string
	StagingPath      string
	CacheUid         int
	CacheGid         int
	LVMDevice        string
	LVMFormat        string
	LVMVirtualSize   string
	lvmVg            string
	lvmBaseLv        string
	currentVersion   atomic.Int64
}

func New(client *client.Client, driverNameSuffix string) *Cached {
	driverNameSuffixUnderscored := strings.ReplaceAll(driverNameSuffix, "-", "_")
	lvmVg := "vg_dateilager_cached" + driverNameSuffixUnderscored
	lvmBaseLv := lvmVg + "/base"

	return &Cached{
		Client:           client,
		DriverNameSuffix: driverNameSuffix,
		StagingPath:      "/var/lib/kubelet/dateilager_cached" + driverNameSuffixUnderscored,
		CacheUid:         NO_CHANGE_USER,
		CacheGid:         NO_CHANGE_USER,
		LVMDevice:        os.Getenv("DL_LVM_DEVICE"),
		LVMFormat:        os.Getenv("DL_LVM_FORMAT"),
		LVMVirtualSize:   os.Getenv("DL_LVM_VIRTUAL_SIZE"),
		lvmVg:            lvmVg,
		lvmBaseLv:        lvmBaseLv,
	}
}

// Fetch the cache into the staging dir
func (c *Cached) Prepare(ctx context.Context, cacheVersion int64) error {
	logger.Info(ctx, "preparing cache", key.CacheVersion.Field(cacheVersion))
	ctx, span := telemetry.Start(ctx, "cached.prepare", trace.WithAttributes(key.CacheVersion.Attribute(cacheVersion)))
	defer span.End()

	start := time.Now()

	var err error
	if err = c.ensurePhysicalVolume(ctx); err != nil {
		return err
	}

	if err = c.ensureVolumeGroup(ctx); err != nil {
		return err
	}

	if err = c.ensureThinPool(ctx); err != nil {
		return err
	}

	if err = c.ensureBaseVolume(ctx); err != nil {
		return err
	}

	if err = c.mountAndFormatBaseVolume(ctx); err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, c.unmountBaseVolume(ctx))
	}()

	var version int64
	var count uint32
	version, count, err = c.Client.GetCache(ctx, filepath.Join(c.StagingPath, CACHE_PATH_SUFFIX), cacheVersion)
	if err != nil {
		return err
	}

	c.currentVersion.Store(version)
	logger.Info(ctx, "cache prepared", key.DurationMS.Field(time.Since(start)), key.Version.Field(version), key.Count.Field(int64(count)))
	span.SetAttributes(key.Count.Attribute(int64(count)))

	return err
}

// Unprepare removes the cached storage
func (c *Cached) Unprepare(ctx context.Context) error {
	logger.Info(ctx, "unpreparing cached storage", key.VolumeGroup.Field(c.lvmVg))

	// Remove volume group if it exists
	err := exec(ctx, "vgdisplay", c.lvmVg)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVg, err)
	}

	if err == nil {
		logger.Info(ctx, "removing volume group", key.VolumeGroup.Field(c.lvmVg))
		if err := exec(ctx, "vgremove", "-y", c.lvmVg); err != nil {
			return fmt.Errorf("failed to remove lvm volume group %s: %w", c.lvmVg, err)
		}
	}

	// Remove physical volume if it exists
	err = exec(ctx, "pvdisplay", c.LVMDevice)
	if err != nil && !strings.Contains(err.Error(), "Failed to find physical volume") {
		return fmt.Errorf("failed to check lvm physical volume %s: %w", c.LVMDevice, err)
	}

	if err == nil {
		logger.Info(ctx, "removing physical volume", key.Device.Field(c.LVMDevice))
		if err := exec(ctx, "pvremove", c.LVMDevice); err != nil {
			return fmt.Errorf("failed to remove lvm physical volume %s: %w", c.LVMDevice, err)
		}
	}

	return nil
}

// GetPluginInfo returns metadata of the plugin
func (c *Cached) GetPluginInfo(ctx context.Context, _ *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{Name: DRIVER_NAME + c.DriverNameSuffix, VendorVersion: version.Version}, nil
}

// GetPluginCapabilities returns available capabilities of the plugin
func (c *Cached) GetPluginCapabilities(ctx context.Context, _ *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{Capabilities: []*csi.PluginCapability{}}, nil
}

// Probe returns the health and readiness of the plugin
func (c *Cached) Probe(ctx context.Context, _ *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	ready := c.currentVersion.Load() != 0
	if !ready {
		logger.Warn(ctx, "csi probe failed as daemon hasn't prepared cache yet", key.Version.Field(c.currentVersion.Load()))
	}
	return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: ready}}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
// This driver has no capabilities like expansion or staging, because we only use it for node local volumes.
func (c *Cached) NodeGetCapabilities(ctx context.Context, _ *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{Capabilities: []*csi.NodeServiceCapability{}}, nil
}

// NodeGetInfo returns the supported capabilities of the node server.
// Usually, a CSI driver would return some interesting stuff about the node here for the controller to use to place volumes, but because we're only supporting node local volumes, we return something very basic
func (c *Cached) NodeGetInfo(ctx context.Context, _ *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId:            first(os.Getenv("NODE_ID"), os.Getenv("NODE_NAME"), os.Getenv("K8S_NODE_NAME"), "dev"),
		MaxVolumesPerNode: 110,
	}, nil
}

// NodePublishVolume publishes a volume to a target path
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

	ctx = logger.With(ctx, key.VolumeID.Field(volumeID), key.TargetPath.Field(targetPath), key.Version.Field(c.currentVersion.Load()))
	trace.SpanFromContext(ctx).SetAttributes(key.VolumeID.Attribute(volumeID), key.TargetPath.Attribute(targetPath), key.Version.Attribute(c.currentVersion.Load()))
	logger.Info(ctx, "publishing volume")

	if err := c.createSnapshot(ctx, volumeID); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create snapshot: %v", err)
	}

	notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "failed to check if target path %s is mounted: %v", targetPath, err)
	}

	if notMounted {
		if err := os.MkdirAll(targetPath, 0o775); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create target path %s: %v", targetPath, err)
		}

		var mountOptions []string
		if c.LVMFormat == "ext4" {
			mountOptions = ext4MountOptions()
		}

		device := "/dev/" + c.lvmVg + "/" + volumeID
		logger.Info(ctx, "mounting snapshot", key.Device.Field(device))
		if err := mounter.Mount(device, targetPath, c.LVMFormat, mountOptions); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to mount snapshot %s to %s: %v", device, targetPath, err)
		}
	}

	if err := os.Chmod(targetPath, 0o775); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to change permissions of target path %s: %v", targetPath, err)
	}

	logger.Info(ctx, "mounted snapshot")
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unpublishes a volume from a target path
func (c *Cached) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}

	targetPath := req.GetTargetPath() // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	ctx = logger.With(ctx, key.VolumeID.Field(volumeID), key.TargetPath.Field(targetPath), key.Version.Field(c.currentVersion.Load()))
	trace.SpanFromContext(ctx).SetAttributes(key.VolumeID.Attribute(volumeID), key.TargetPath.Attribute(targetPath), key.Version.Attribute(c.currentVersion.Load()))
	logger.Info(ctx, "unpublishing volume")

	notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "failed to check if target path %s is mounted: %v", targetPath, err)
	}

	if !notMounted {
		logger.Info(ctx, "unmounting target path")
		if err := mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmount lvm snapshot at %s: %v", targetPath, err)
		}
	}

	if err := c.removeSnapshot(ctx, volumeID); err != nil {
		logger.Warn(ctx, "failed to remove snapshot", zap.Error(err))
	}

	logger.Info(ctx, "volume unpublished and data removed")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetVolumeStats returns the volume capacity statistics available for the given volume
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

var executor = utilexec.New()

var mounter = &mount.SafeFormatAndMount{
	Interface: mount.New(""),
	Exec:      executor,
}

// exec executes a command
func exec(ctx context.Context, command string, args ...string) error {
	_, err := execOutput(ctx, command, args...)
	if err != nil {
		return err
	}
	return nil
}

// execOutput executes a command and returns the output
func execOutput(ctx context.Context, command string, args ...string) (string, error) {
	logger.Debug(ctx, "executing command", key.Command.Field(command), key.Args.Field(args))
	cmd := executor.CommandContext(ctx, command, args...)
	bs, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command %s %s: %w: %s", command, strings.Join(args, " "), err, string(bs))
	}
	return strings.TrimSpace(string(bs)), nil
}

// ensurePhysicalVolume creates LVM physical volume if it doesn't exist
func (c *Cached) ensurePhysicalVolume(ctx context.Context) error {
	ctx = logger.With(ctx, key.Device.Field(c.LVMDevice))
	logger.Debug(ctx, "checking physical volume")

	err := exec(ctx, "pvdisplay", c.LVMDevice)
	if err == nil {
		logger.Debug(ctx, "physical volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find physical volume") {
		return fmt.Errorf("failed to check lvm physical volume %s: %w", c.LVMDevice, err)
	}

	logger.Info(ctx, "creating physical volume")
	if err := exec(ctx, "pvcreate", c.LVMDevice); err != nil && !strings.Contains(err.Error(), "signal: killed") {
		return fmt.Errorf("failed to create lvm physical volume %s: %w", c.LVMDevice, err)
	}

	return nil
}

// ensureVolumeGroup creates LVM volume group if it doesn't exist
func (c *Cached) ensureVolumeGroup(ctx context.Context) error {
	ctx = logger.With(ctx, key.VolumeGroup.Field(c.lvmVg), key.Device.Field(c.LVMDevice))
	logger.Debug(ctx, "checking volume group")

	err := exec(ctx, "vgdisplay", c.lvmVg)
	if err == nil {
		logger.Debug(ctx, "volume group already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVg, err)
	}

	logger.Info(ctx, "creating volume group")
	if err := exec(ctx, "vgcreate", c.lvmVg, c.LVMDevice); err != nil {
		return fmt.Errorf("failed to create lvm volume group %s: %w", c.lvmVg, err)
	}

	return nil
}

// ensureThinPool creates LVM thin pool if it doesn't exist
func (c *Cached) ensureThinPool(ctx context.Context) error {
	thinPool := c.lvmVg + "/thinpool"
	ctx = logger.With(ctx, key.ThinPool.Field(thinPool))
	logger.Debug(ctx, "checking thin pool")

	err := exec(ctx, "lvdisplay", thinPool)
	if err == nil {
		logger.Debug(ctx, "thin pool already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check lvm thin pool %s: %w", thinPool, err)
	}

	logger.Info(ctx, "creating thin pool")
	if err := exec(ctx, "lvcreate", c.lvmVg, "--name=thinpool", "--extents=95%VG", "--thinpool=thinpool", "--chunksize=64k"); err != nil {
		return fmt.Errorf("failed to create lvm thin pool %s: %w", thinPool, err)
	}

	return nil
}

// ensureBaseVolume creates base LVM volume if it doesn't exist
func (c *Cached) ensureBaseVolume(ctx context.Context) error {
	ctx = logger.With(ctx, key.LogicalVolume.Field(c.lvmBaseLv), key.VirtualSize.Field(c.LVMVirtualSize))
	logger.Debug(ctx, "checking base volume")

	err := exec(ctx, "lvdisplay", c.lvmBaseLv)
	if err == nil {
		logger.Debug(ctx, "base volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check base volume %s: %w", c.lvmBaseLv, err)
	}

	logger.Info(ctx, "creating base volume")
	if err := exec(ctx, "lvcreate", "--name=base", "--virtualsize="+c.LVMVirtualSize, "--thinpool="+c.lvmVg+"/thinpool"); err != nil {
		return fmt.Errorf("failed to create base volume %s: %w", c.lvmBaseLv, err)
	}

	if err := c.udevSettle(ctx, "/dev/"+c.lvmBaseLv); err != nil {
		logger.Warn(ctx, "udev settle failed for base volume", zap.Error(err))
	}

	return nil
}

// mountAndFormatBaseVolume mounts and formats the base volume
func (c *Cached) mountAndFormatBaseVolume(ctx context.Context) error {
	notMounted, err := mounter.IsLikelyNotMountPoint(c.StagingPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to check if staging directory %s is mounted: %w", c.StagingPath, err)
	}

	if !notMounted {
		logger.Debug(ctx, "staging directory is already mounted", key.Path.Field(c.StagingPath))
		return nil
	}

	logger.Info(ctx, "mounting base volume", key.LogicalVolume.Field(c.lvmBaseLv), key.Path.Field(c.StagingPath))
	if err := os.MkdirAll(c.StagingPath, 0o775); err != nil {
		return fmt.Errorf("failed to create staging directory %s: %w", c.StagingPath, err)
	}

	var mountOptions []string
	if c.LVMFormat == "ext4" {
		mountOptions = ext4MountOptions()

		// ext4 is created with `lazy_itable_init=1` by default. That means the inode tables are not fully zero-initialized at mkfs time.
		// Instead, the kernel's `ext4lazyinit` thread clears the remaining blocks later while the filesystem is mounted.
		//
		// On ordinary block devices that "pay-as-you-go" strategy is fine, but on our LVM thin snapshots it is disastrous: every background
		// write touches a previously shared metadata block, so the thin-pool must allocate a fresh block for the snapshot (copy-on-write). For
		// example, a 350 GiB filesystem can burn ~3-4 GiB of pool space per snapshot before the inode tables are finally clean.
		//
		// Mounting with `init_itable=0` tells the kernel to finish that zeroing in one burst now on the base volume. We take the hit once,
		// up-front, and future snapshots stay space-neutral until real user data is written.
		mountOptions = append(mountOptions, "init_itable=0")
	}

	if err := mounter.FormatAndMount("/dev/"+c.lvmBaseLv, c.StagingPath, c.LVMFormat, mountOptions); err != nil {
		return fmt.Errorf("failed to mount base volume %s to staging directory %s: %w", "/dev/"+c.lvmBaseLv, c.StagingPath, err)
	}

	// Clean up lost+found directory, it's not needed and confusing.
	if err := os.RemoveAll(filepath.Join(c.StagingPath, "lost+found")); err != nil {
		return fmt.Errorf("failed to remove lost+found directory %s: %w", filepath.Join(c.StagingPath, "lost+found"), err)
	}

	return nil
}

// unmountBaseVolume unmounts the staging directory and resizes the base volume to the required size
func (c *Cached) unmountBaseVolume(ctx context.Context) error {
	if c.CacheUid != NO_CHANGE_USER || c.CacheGid != NO_CHANGE_USER {
		logger.Debug(ctx, "setting ownership", zap.Int("uid", c.CacheUid), zap.Int("gid", c.CacheGid))
		if err := fastwalk.Walk(nil, c.StagingPath, func(walkPath string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Lchown(walkPath, c.CacheUid, c.CacheGid)
		}); err != nil {
			return fmt.Errorf("failed to set ownership of staging directory %s: %w", c.StagingPath, err)
		}
	}

	// Trim filesystem before unmounting
	if err := exec(ctx, "fstrim", "-v", c.StagingPath); err != nil {
		logger.Warn(ctx, "failed to trim filesystem", zap.Error(err))
	}

	if err := mounter.Unmount(c.StagingPath); err != nil {
		return fmt.Errorf("failed to unmount staging directory %s: %w", c.StagingPath, err)
	}

	return nil
}

// createSnapshot creates an LVM snapshot from the base volume
func (c *Cached) createSnapshot(ctx context.Context, volumeID string) error {
	snapshotLv := c.lvmVg + "/" + volumeID
	ctx = logger.With(ctx, key.LogicalVolume.Field(snapshotLv))
	logger.Debug(ctx, "checking snapshot")

	err := exec(ctx, "lvdisplay", snapshotLv)
	if err == nil {
		logger.Info(ctx, "snapshot already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check if snapshot %s exists: %w", snapshotLv, err)
	}

	logger.Info(ctx, "creating snapshot")
	if err := exec(ctx, "lvcreate", c.lvmBaseLv, "--name="+volumeID, "--snapshot", "--setactivationskip=n"); err != nil {
		return fmt.Errorf("failed to create snapshot of base volume %s: %w", c.lvmBaseLv, err)
	}

	// Wait for device to appear and settle udev
	if err := c.udevSettle(ctx, "/dev/"+snapshotLv); err != nil {
		// keep going, the device might still be available
		logger.Warn(ctx, "udev settle failed for snapshot", zap.Error(err))
	}

	return nil
}

// removeSnapshot removes an LVM snapshot
func (c *Cached) removeSnapshot(ctx context.Context, volumeID string) error {
	snapshotLv := c.lvmVg + "/" + volumeID
	ctx = logger.With(ctx, key.LogicalVolume.Field(snapshotLv))
	logger.Debug(ctx, "checking snapshot for removal")

	err := exec(ctx, "lvdisplay", snapshotLv)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			logger.Info(ctx, "snapshot already removed")
			return nil
		}
		return fmt.Errorf("failed to check if snapshot %s exists: %w", snapshotLv, err)
	}

	logger.Info(ctx, "removing snapshot")
	if err := exec(ctx, "lvremove", "-y", snapshotLv); err != nil {
		return fmt.Errorf("failed to remove snapshot %s: %w", snapshotLv, err)
	}

	return nil
}

// udevSettle triggers udev events and waits for device to appear
func (c *Cached) udevSettle(ctx context.Context, devPath string) error {
	// Trigger udev events for the device
	if err := exec(ctx, "udevadm", "trigger", "--action=add", devPath); err != nil {
		logger.Warn(ctx, "udevadm trigger failed", zap.Error(err))
		// Continue anyway, the device might still be available
	}

	// Wait for udev to settle with timeout
	settleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := exec(settleCtx, "udevadm", "settle", "--exit-if-exists="+devPath); err != nil {
		logger.Warn(ctx, "udev settle failed", zap.Error(err))
		// Fallback to polling
		return waitForDevice(ctx, devPath, 5*time.Second)
	}

	return nil
}

// waitForDevice polls for device availability with timeout
func waitForDevice(ctx context.Context, devicePath string, timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(devicePath); err == nil {
				logger.Debug(ctx, "device is available")
				return nil
			}
		case <-timeoutCtx.Done():
			return fmt.Errorf("device node did not appear in time: %s (timeout: %v)", devicePath, timeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// first returns the first non-empty string from the given slice
func first(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func getFolderSize(path string) (int64, error) {
	var totalBytes atomic.Int64
	err := fastwalk.Walk(nil, path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		totalBytes.Add(info.Size())
		return nil
	})
	return totalBytes.Load(), err
}

// ext4MountOptions returns the mount options that improve write performance for ext4 filesystems
func ext4MountOptions() []string {
	return []string{
		// Eliminates most time-stamp writes (noatime/nodiratime) and defers the rest (lazytime) so they piggy-back on larger I/O.
		// Safe enough for most apps that don’t rely on atime, but you give up the ability to audit “last accessed” precisely.
		"noatime", "nodiratime", "lazytime",

		// Journals only metadata; user-data blocks may hit disk long after their metadata, so post-crash files can contain stale garbage.
		// Best-case throughput boost for random-write workloads.
		"data=writeback",

		// Skips the flush/write-barrier that normally forces the drive cache to commit data in-order. If the device isn’t battery-backed, a
		// sudden power loss can wipe everything written since the last barrier.
		"nobarrier",

		// Lets kjournald2 ship the journal’s commit block without waiting for descriptor blocks, cutting one synchronous flush per commit.
		// Pairs well with long commit= intervals.
		"journal_async_commit",

		// Metadata is flushed only every two minutes (default is 5 s). You’ll lose up to the last 120 s of metadata on a crash—sometimes more
		// thanks to delayed allocation.
		"commit=120",
	}
}
