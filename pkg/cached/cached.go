package cached

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/exec"
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
	"k8s.io/utils/mount"
)

const (
	DRIVER_NAME       = "dev.gadget.dateilager.cached"
	CACHE_PATH_SUFFIX = "dl_cache"
	NO_CHANGE_USER    = -1
	EXT4              = "ext4"
)

type Cached struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer
	Client                     *client.Client
	DriverNameSuffix           string
	StagingPath                string
	CacheUid                   int
	CacheGid                   int
	LVMThinpoolDeviceGlob      string
	LVMBaseDevice              string
	LVMBaseDeviceFormat        string
	LVMVirtualSize             string
	LVMRAMWritebackCacheSizeKB int64
	lvmVg                      string
	lvmBaseLv                  string
	lvmThinpoolDevices         []string
	ramDevicePath              string
	currentVersion             atomic.Int64
}

func New(client *client.Client, driverNameSuffix string) *Cached {
	driverNameSuffixUnderscored := strings.ReplaceAll(driverNameSuffix, "-", "_")
	lvmVg := "vg_dateilager_cached" + driverNameSuffixUnderscored
	lvmBaseLv := lvmVg + "/base"

	lvmRamWritebackCacheSizeKB := int64(0)
	if envSize := os.Getenv("DL_LVM_RAM_WRITEBACK_CACHE_SIZE_KB"); envSize != "" {
		if size, err := strconv.ParseInt(envSize, 10, 64); err == nil {
			lvmRamWritebackCacheSizeKB = size
		}
	}

	return &Cached{
		Client:                     client,
		DriverNameSuffix:           driverNameSuffix,
		StagingPath:                "/var/lib/kubelet/dateilager_cached" + driverNameSuffixUnderscored,
		CacheUid:                   NO_CHANGE_USER,
		CacheGid:                   NO_CHANGE_USER,
		LVMThinpoolDeviceGlob:      os.Getenv("DL_LVM_THINPOOL_DEVICE_GLOB"),
		LVMBaseDevice:              os.Getenv("DL_LVM_BASE_DEVICE"),
		LVMBaseDeviceFormat:        os.Getenv("DL_LVM_BASE_DEVICE_FORMAT"),
		LVMVirtualSize:             os.Getenv("DL_LVM_VIRTUAL_SIZE"),
		LVMRAMWritebackCacheSizeKB: lvmRamWritebackCacheSizeKB,
		lvmVg:                      lvmVg,
		lvmBaseLv:                  lvmBaseLv,
	}
}

func (c *Cached) PrepareBaseVolume(ctx context.Context, cacheVersion int64) error {
	ctx, span := telemetry.Start(ctx, "cached.prepare-base-volume")
	defer span.End()

	start := time.Now()
	var err error
	if err = c.mountAndFormatBaseVolume(ctx); err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, c.unmountBaseVolume(ctx))
	}()

	var cachedCount uint32
	cacheVersion, cachedCount, err = c.Client.GetCache(ctx, filepath.Join(c.StagingPath, CACHE_PATH_SUFFIX), cacheVersion)
	if err != nil {
		return err
	}

	c.currentVersion.Store(cacheVersion)
	logger.Info(ctx, "prepared base volume", key.CacheVersion.Field(cacheVersion), key.CachedCount.Field(cachedCount), key.DurationMS.Field(time.Since(start)))
	span.SetAttributes(key.CacheVersion.Attribute(cacheVersion), key.CachedCount.Attribute(cachedCount))

	return err
}

// Fetch the cache into the staging dir
func (c *Cached) Prepare(ctx context.Context, cacheVersion int64) error {
	logger.Info(ctx, "preparing cached", key.CacheVersion.Field(cacheVersion))
	ctx, span := telemetry.Start(ctx, "cached.prepare", trace.WithAttributes(key.CacheVersion.Attribute(cacheVersion)))
	defer span.End()

	if err := c.findThinpoolDevices(ctx); err != nil {
		return err
	}

	if err := c.ensureVolumeGroup(ctx); err != nil {
		return err
	}

	if c.LVMRAMWritebackCacheSizeKB > 0 {
		if err := c.ensureRamPool(ctx); err != nil {
			return err
		}
	}

	if err := c.ensureThinPool(ctx); err != nil {
		return err
	}

	if err := c.ensureBaseVolume(ctx, cacheVersion); err != nil {
		return err
	}

	return nil
}

// Unprepare removes the cached storage
func (c *Cached) Unprepare(ctx context.Context) error {
	logger.Info(ctx, "unpreparing cached storage", key.VolumeGroup.Field(c.lvmVg))

	// Remove volume group if it exists
	err := exec.Run(ctx, "vgdisplay", c.lvmVg)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVg, err)
	}

	if err == nil {
		logger.Info(ctx, "removing volume group", key.VolumeGroup.Field(c.lvmVg))
		if err := exec.Run(ctx, "vgremove", "-y", c.lvmVg); err != nil {
			return fmt.Errorf("failed to remove lvm volume group %s: %w", c.lvmVg, err)
		}
	}

	// Remove each physical volume if it exists
	for _, device := range c.lvmThinpoolDevices {
		err = exec.Run(ctx, "pvdisplay", device)
		if err != nil && !strings.Contains(err.Error(), "Failed to find physical volume") {
			return fmt.Errorf("failed to check lvm physical volume %s: %w", device, err)
		}

		if err == nil {
			logger.Info(ctx, "removing physical volume", key.Device.Field(device))
			if err := exec.Run(ctx, "pvremove", device); err != nil {
				return fmt.Errorf("failed to remove lvm physical volume %s: %w", device, err)
			}
		}
	}

	// Remove RAM cache physical volume if it exists
	if c.ramDevicePath != "" {
		err = exec.Run(ctx, "pvdisplay", c.ramDevicePath)
		if err != nil && !strings.Contains(err.Error(), "Failed to find physical volume") {
			return fmt.Errorf("failed to check lvm physical volume %s: %w", c.ramDevicePath, err)
		}

		if err == nil {
			logger.Info(ctx, "removing physical volume", key.Device.Field(c.ramDevicePath))
			if err := exec.Run(ctx, "pvremove", c.ramDevicePath); err != nil {
				return fmt.Errorf("failed to remove lvm physical volume %s: %w", c.ramDevicePath, err)
			}
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
		if c.LVMBaseDeviceFormat == EXT4 {
			mountOptions = ext4MountOptions()
		}

		device := "/dev/" + c.lvmVg + "/" + volumeID
		logger.Info(ctx, "mounting snapshot", key.Device.Field(device))
		if err := mounter.Mount(device, targetPath, c.LVMBaseDeviceFormat, mountOptions); err != nil {
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

var mounter = mount.New("")

func (c *Cached) findThinpoolDevices(_ context.Context) error {
	globs := strings.Split(c.LVMThinpoolDeviceGlob, ",")
	var allDevices []string

	for _, glob := range globs {
		glob = strings.TrimSpace(glob)
		devices, err := filepath.Glob(glob)
		if err != nil {
			return fmt.Errorf("failed to find lvm devices for glob %s: %w", glob, err)
		}
		allDevices = append(allDevices, devices...)
	}

	if len(allDevices) == 0 {
		return fmt.Errorf("no lvm devices found for globs %s", c.LVMThinpoolDeviceGlob)
	}
	c.lvmThinpoolDevices = allDevices
	return nil
}

// ensureVolumeGroup creates LVM volume group if it doesn't exist
func (c *Cached) ensureVolumeGroup(ctx context.Context) error {
	ctx = logger.With(ctx, key.VolumeGroup.Field(c.lvmVg), key.DeviceGlob.Field(c.LVMThinpoolDeviceGlob))
	logger.Debug(ctx, "checking volume group")

	err := exec.Run(ctx, "vgdisplay", c.lvmVg)
	if err == nil {
		logger.Info(ctx, "volume group already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVg, err)
	}

	if c.LVMBaseDevice != "" {
		logger.Info(ctx, "importing base device")
		if err := exec.Run(ctx, "vgimportclone", "--basevgname="+c.lvmVg, c.LVMBaseDevice); err != nil {
			return fmt.Errorf("failed to import base device %s: %w", c.LVMBaseDevice, err)
		}

		if err := exec.Run(ctx, "vgextend", append([]string{"--config=devices/allow_mixed_block_sizes=1", c.lvmVg}, c.lvmThinpoolDevices...)...); err != nil {
			return fmt.Errorf("failed to extend volume group with devices: %w", err)
		}
	} else {
		logger.Info(ctx, "creating volume group")
		if err := exec.Run(ctx, "vgcreate", append([]string{c.lvmVg}, c.lvmThinpoolDevices...)...); err != nil {
			return fmt.Errorf("failed to create lvm volume group %s: %w", c.lvmVg, err)
		}
	}

	return nil
}

// ensureThinPool creates the LVM thin pool if it doesn't exist
func (c *Cached) ensureThinPool(ctx context.Context) error {
	thinPool := c.lvmVg + "/thinpool"
	ctx = logger.With(ctx, key.ThinPool.Field(thinPool))
	logger.Debug(ctx, "checking thin pool")

	err := exec.Run(ctx, "lvdisplay", thinPool)
	if err == nil {
		logger.Info(ctx, "thin pool already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check lvm thin pool %s: %w", thinPool, err)
	}

	lvCreateArgs := []string{
		// Create a thin pool
		"--type", "thin-pool",

		// Name the thin pool vg_dateilager_cached/thinpool
		"--name", "thinpool",

		// Make the thin pool take up all the space on the provided PVs
		"--extents=100%PVS",

		// Use minimum allowed chunk size for better small file efficiency
		"--chunksize=64k",

		// Use one stripe per thin pool device to maximize performance
		"--stripes=" + strconv.Itoa(len(c.lvmThinpoolDevices)),

		// Use a small stripe size for better IO performance on small files
		"--stripesize=64k",

		// Skip zeroing for faster creation and better write performance
		// TODO: figure out data leakage risk
		"--zero=n",

		// Pass TRIM/discard commands through to underlying storage
		"--discards=passdown",

		// Don't activate the thinpool yet so we can muck with it for writeback caching if needed
		"--activate=n",

		// Pass the volume group the thin pool should be created in
		c.lvmVg,
	}

	// explicitly pass the devices to use for the thin pool
	lvCreateArgs = append(lvCreateArgs, c.lvmThinpoolDevices...)

	logger.Info(ctx, "creating thin pool")
	if err := exec.Run(ctx, "lvcreate", lvCreateArgs...); err != nil {
		return fmt.Errorf("failed to create lvm thin pool %s: %w", thinPool, err)
	}

	if c.LVMRAMWritebackCacheSizeKB > 0 {
		if err := exec.Run(ctx, "lvconvert", "--yes", "--type", "writecache", "--cachesettings", "high_watermark=75 low_watermark=60 writeback_jobs=4 block_size=4096", "--cachevol", c.lvmVg+"/cache", c.lvmVg+"/thinpool"); err != nil {
			return fmt.Errorf("failed to create writeback cache for data lv %s: %w", c.lvmBaseLv, err)
		}

		// invoke lvconvert to move the thinpool metadata to the ram device cache_meta LV
		// TODO: get working, creates transaction id mismatch errors if enabled
		// if err := exec.Do(ctx, "lvconvert", "--yes", "--thinpool", c.lvmVg+"/thinpool", "--poolmetadata", c.lvmVg+"/cache_meta"); err != nil {
		// 	return fmt.Errorf("failed to move thinpool metadata to ram device cache_meta LV %s to %s: %w", c.lvmVg+"/thinpool", c.lvmVg+"/cache_meta", err)
		// }

		logger.Info(ctx, "writeback cache created successfully")
	}

	// activate the thinpool
	if err := exec.Run(ctx, "lvchange", "--activate", "y", c.lvmVg+"/thinpool"); err != nil {
		return fmt.Errorf("failed to activate thinpool %s: %w", c.lvmVg+"/thinpool", err)
	}

	if err := c.udevSettle(ctx, "/dev/"+c.lvmVg+"/thinpool"); err != nil {
		logger.Warn(ctx, "udev settle failed for thinpool", zap.Error(err))
	}

	// Refresh the thinpool to fix mismatched transaction ID issues
	if err := exec.Run(ctx, "pvscan", "--cache", "--activate", "ay"); err != nil {
		return fmt.Errorf("failed to rescan pv cache: %w", err)
	}
	if err := exec.Run(ctx, "lvchange", "--refresh", c.lvmVg); err != nil {
		return fmt.Errorf("failed to refresh vg %s: %w", c.lvmVg, err)
	}
	if err := exec.Run(ctx, "lvchange", "--refresh", c.lvmVg+"/thinpool"); err != nil {
		return fmt.Errorf("failed to refresh thinpool %s: %w", c.lvmVg+"/thinpool", err)
	}

	return nil
}

// ensureBaseVolume creates base LVM volume if it doesn't exist
func (c *Cached) ensureBaseVolume(ctx context.Context, cacheVersion int64) error {
	ctx = logger.With(ctx, key.LogicalVolume.Field(c.lvmBaseLv), key.VirtualSize.Field(c.LVMVirtualSize), key.CacheVersion.Field(cacheVersion))
	logger.Debug(ctx, "checking base volume")

	err := exec.Run(ctx, "lvdisplay", c.lvmBaseLv)
	if err == nil {
		logger.Info(ctx, "base volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check base volume %s: %w", c.lvmBaseLv, err)
	}

	if c.LVMBaseDevice != "" {
		return fmt.Errorf("LVMBaseDevice is set, but base volume %s does not exist", c.lvmBaseLv)
	}

	logger.Info(ctx, "creating base volume")
	if err := exec.Run(ctx, "lvcreate", "--name=base", "--virtualsize="+c.LVMVirtualSize, "--thinpool="+c.lvmVg+"/thinpool"); err != nil {
		return fmt.Errorf("failed to create base volume %s: %w", c.lvmBaseLv, err)
	}

	if err := c.udevSettle(ctx, "/dev/"+c.lvmBaseLv); err != nil {
		logger.Warn(ctx, "udev settle failed for base volume", zap.Error(err))
	}

	return c.PrepareBaseVolume(ctx, cacheVersion)
}

// mountAndFormatBaseVolume mounts and formats the base volume
func (c *Cached) mountAndFormatBaseVolume(ctx context.Context) error {
	notMounted, err := mounter.IsLikelyNotMountPoint(c.StagingPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to check if staging directory %s is mounted: %w", c.StagingPath, err)
	}

	if !notMounted {
		logger.Info(ctx, "staging directory is already mounted", key.Path.Field(c.StagingPath))
		return nil
	}

	baseDevice := "/dev/" + c.lvmBaseLv
	if c.LVMBaseDevice != "" {
		baseDevice = c.LVMBaseDevice
	}

	ctx = logger.With(ctx, key.Device.Field(baseDevice), key.Path.Field(c.StagingPath))

	fsFormat, err := exec.Output(ctx, "blkid", "-o", "value", "-s", "TYPE", baseDevice)
	if err != nil && !strings.Contains(err.Error(), "exit status 2") {
		return fmt.Errorf("failed to get filesystem type of base volume %s: %w", baseDevice, err)
	}

	if fsFormat != c.LVMBaseDeviceFormat {
		formatOptions := []string{baseDevice}
		if c.LVMBaseDeviceFormat == EXT4 {
			formatOptions = append(formatOptions, ext4FormatOptions()...)
		}

		logger.Info(ctx, "base volume is not formatted as expected, formatting", zap.String("expected", c.LVMBaseDeviceFormat), zap.String("actual", fsFormat))
		if err := exec.Run(ctx, "mkfs."+c.LVMBaseDeviceFormat, formatOptions...); err != nil {
			return fmt.Errorf("failed to format base volume %s: %w", baseDevice, err)
		}
	}

	logger.Info(ctx, "mounting base volume")
	if err := os.MkdirAll(c.StagingPath, 0o775); err != nil {
		return fmt.Errorf("failed to create staging directory %s: %w", c.StagingPath, err)
	}

	var mountOptions []string
	if c.LVMBaseDeviceFormat == EXT4 {
		mountOptions = ext4MountOptions()
	}

	if err := mounter.Mount(baseDevice, c.StagingPath, c.LVMBaseDeviceFormat, mountOptions); err != nil {
		return fmt.Errorf("failed to mount base volume %s to staging directory %s: %w", baseDevice, c.StagingPath, err)
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
		logger.Info(ctx, "setting ownership", zap.Int("uid", c.CacheUid), zap.Int("gid", c.CacheGid))
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
	if err := exec.Run(ctx, "fstrim", "-v", c.StagingPath); err != nil {
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

	err := exec.Run(ctx, "lvdisplay", snapshotLv)
	if err == nil {
		logger.Info(ctx, "snapshot already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check if snapshot %s exists: %w", snapshotLv, err)
	}

	logger.Info(ctx, "creating snapshot")
	// Use the original LVM base volume for creating snapshots, not the cached device
	if err := exec.Run(ctx, "lvcreate", c.lvmBaseLv, "--name="+volumeID, "--snapshot", "--setactivationskip=n"); err != nil {
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

	err := exec.Run(ctx, "lvdisplay", snapshotLv)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			logger.Info(ctx, "snapshot already removed")
			return nil
		}
		return fmt.Errorf("failed to check if snapshot %s exists: %w", snapshotLv, err)
	}

	logger.Info(ctx, "removing snapshot")
	if err := exec.Run(ctx, "lvremove", "-y", snapshotLv); err != nil {
		return fmt.Errorf("failed to remove snapshot %s: %w", snapshotLv, err)
	}

	return nil
}

// udevSettle triggers udev events and waits for device to appear
func (c *Cached) udevSettle(ctx context.Context, devPath string) error {
	// Trigger udev events for the device
	if err := exec.Run(ctx, "udevadm", "trigger", "--action=add", devPath); err != nil {
		logger.Warn(ctx, "udevadm trigger failed", zap.Error(err))
		// Continue anyway, the device might still be available
	}

	// Wait for udev to settle with timeout
	settleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := exec.Run(settleCtx, "udevadm", "settle", "--exit-if-exists="+devPath); err != nil {
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

// ext4FormatOptions returns the format options for ext4 filesystems optimized for node_modules
func ext4FormatOptions() []string {
	return []string{
		// Force creation even if the target looks mounted or already formatted
		"-F",

		// 4 KiB logical blocks - better balance for small files on modern storage
		"-b", "4096",

		// One inode per 16 KiB of projected data. 256-byte inodes instead of 128, required for inline_data.
		"-i", "16384", "-I", "256",

		// Zero per-FS reserve since this is a dedicated node_modules volume
		"-m", "0",

		// Larger flex_bg groups for better allocation - 64 block groups
		"-G", "64",

		// Optimized feature flags for write performance and small files
		"-O", "extent,dir_index,sparse_super2,filetype,flex_bg,64bit,inline_data,^metadata_csum",

		// Extended parameters optimized for NVMe and small files:
		//   No stride/stripe-width for better flexibility
		//   lazy_*_init=0 for predictable performance
		//   nodiscard during format for faster creation
		//   packed_meta_blocks for better metadata locality
		"-E", "lazy_itable_init=0,lazy_journal_init=0,nodiscard,packed_meta_blocks=1,num_backup_sb=0",
	}
}

// ext4MountOptions returns mount options optimized for maximum write performance
func ext4MountOptions() []string {
	return []string{
		// Eliminate all timestamp updates for maximum performance
		"noatime", "nodiratime", "lazytime",

		// Disable write barriers - assumes battery-backed storage or acceptable data loss risk
		"nobarrier",

		// Enable discard for SSD/NVMe TRIM support
		"discard",

		// Enable writeback for better performance
		"data=writeback",

		// Set commit interval to 60 seconds for better performance
		"commit=60",

		// Continue on errors rather than remounting read-only
		"errors=continue",
	}
}

// ensureRamPool creates a RAM device using dmsetup for use as a writeback cache
func (c *Cached) ensureRamPool(ctx context.Context) error {
	c.ramDevicePath = "/dev/ram0"
	ctx = logger.With(ctx, key.Device.Field(c.ramDevicePath), key.Count.Field(c.LVMRAMWritebackCacheSizeKB))
	logger.Info(ctx, "creating RAM device for writeback cache")

	if _, err := os.Stat(c.ramDevicePath); err == nil {
		logger.Info(ctx, "RAM device already exists")
	} else {
		if err := exec.Run(ctx, "modprobe", "brd", "rd_nr=1", "rd_size="+strconv.Itoa(int(c.LVMRAMWritebackCacheSizeKB))); err != nil {
			return fmt.Errorf("failed to create RAM device %s: %w", c.ramDevicePath, err)
		}
	}

	// Initialize as PV and add to VG
	if err := exec.Run(ctx, "pvdisplay", c.ramDevicePath); err == nil {
		logger.Info(ctx, "RAM device already a PV")
	} else {
		if err := exec.Run(ctx, "pvcreate", c.ramDevicePath); err != nil {
			return fmt.Errorf("failed to pvcreate RAM device %s: %w", c.ramDevicePath, err)
		}
	}
	if err := exec.Run(ctx, "vgdisplay", c.lvmVg); err == nil {
		logger.Info(ctx, "RAM volume group already exists")
	} else {
		if err := exec.Run(ctx, "vgextend", c.lvmVg, c.ramDevicePath); err != nil {
			return fmt.Errorf("failed to vgextend RAM device to VG %s: %w", c.lvmVg, err)
		}
	}

	metaCacheSize := int64(float64(c.LVMRAMWritebackCacheSizeKB) * 0.2)
	cacheSize := int64(float64(c.LVMRAMWritebackCacheSizeKB) * 0.79)

	// Create an LV that will act as the writeback cache for the metadata, will create an LV like vg_dateilager_cached_ram_<driver_name>/cache_meta
	if err := exec.Run(ctx, "lvcreate", "--size", strconv.FormatInt(metaCacheSize, 10)+"kb", "--name", "cache_meta", "--setactivationskip=y", c.lvmVg); err != nil {
		if strings.Contains(err.Error(), "already exists in volume group") {
			logger.Info(ctx, "metadata cache LV already exists")
		} else {
			return fmt.Errorf("failed to create metadata cache LV on RAM VG %s: %w", c.lvmVg, err)
		}
	}

	// Create an LV that will act as the writeback cache for the data, will create an LV like vg_dateilager_cached_ram_<driver_name>/cache
	if err := exec.Run(ctx, "lvcreate", "--size", strconv.FormatInt(cacheSize, 10)+"kb", "--name", "cache", "--setactivationskip=y", c.lvmVg); err != nil {
		if strings.Contains(err.Error(), "already exists in volume group") {
			logger.Info(ctx, "data cache LV already exists")
		} else {
			return fmt.Errorf("failed to create data cache LV on RAM VG %s: %w", c.lvmVg, err)
		}
	}

	logger.Info(ctx, "RAM device created, initialized as PV, added to VG, and created data and metadata cache LV")
	return nil
}
