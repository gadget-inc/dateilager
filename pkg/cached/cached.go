package cached

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
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
	Client                *client.Client
	DriverNameSuffix      string
	StagingPath           string
	CacheUid              int
	CacheGid              int
	LVMThinpoolDeviceGlob string
	LVMBaseDevice         string
	LVMBaseDeviceFormat   string
	lvmVg                 string
	lvmThinpoolDevices    []string
	prepared              atomic.Bool
}

func New(client *client.Client, driverNameSuffix string) *Cached {
	driverNameSuffixUnderscored := strings.ReplaceAll(driverNameSuffix, "-", "_")

	return &Cached{
		Client:                client,
		DriverNameSuffix:      driverNameSuffix,
		StagingPath:           "/var/lib/kubelet/dateilager_cached" + driverNameSuffixUnderscored,
		CacheUid:              NO_CHANGE_USER,
		CacheGid:              NO_CHANGE_USER,
		LVMBaseDevice:         os.Getenv("DL_LVM_BASE_DEVICE"),
		LVMBaseDeviceFormat:   os.Getenv("DL_LVM_BASE_DEVICE_FORMAT"),
		LVMThinpoolDeviceGlob: os.Getenv("DL_LVM_THINPOOL_DEVICE_GLOB"),
		lvmVg:                 "vg_dateilager_cached" + driverNameSuffixUnderscored,
	}
}

func (c *Cached) Prepare(ctx context.Context, cacheVersion int64) error {
	logger.Info(ctx, "preparing cached", key.CacheVersion.Field(cacheVersion))
	ctx, span := telemetry.Start(ctx, "cached.prepare", trace.WithAttributes(key.CacheVersion.Attribute(cacheVersion)))
	defer span.End()

	if err := c.findThinpoolDevices(ctx); err != nil {
		return err
	}

	if err := c.PrepareBaseDevice(ctx, cacheVersion); err != nil {
		return err
	}

	if err := c.importBaseDevice(ctx); err != nil {
		return err
	}

	if err := c.createThinPool(ctx); err != nil {
		return err
	}

	c.prepared.Store(true)

	return nil
}

// Unprepare removes the cached storage
func (c *Cached) Unprepare(ctx context.Context) error {
	logger.Info(ctx, "unpreparing cached storage", key.VolumeGroup.Field(c.lvmVg))

	if err := removeLV(ctx, c.lvmVg+"/thinpool"); err != nil {
		return err
	}

	if err := removeLV(ctx, c.lvmVg+"/base"); err != nil {
		return err
	}

	if err := removeVG(ctx, c.lvmVg); err != nil {
		return err
	}

	for _, device := range c.lvmThinpoolDevices {
		if err := removePV(ctx, device); err != nil {
			return err
		}
	}

	if err := removePV(ctx, c.LVMBaseDevice); err != nil {
		return err
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
	ready := c.prepared.Load()
	if !ready {
		logger.Warn(ctx, "csi probe failed as daemon hasn't prepared cache yet")
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

	lvName := c.lvmVg + "/" + volumeID
	lvDevice := "/dev/" + lvName

	ctx = logger.With(ctx, key.VolumeID.Field(volumeID), key.TargetPath.Field(targetPath), key.LogicalVolume.Field(lvName), key.Device.Field(lvDevice))
	trace.SpanFromContext(ctx).SetAttributes(key.VolumeID.Attribute(volumeID), key.TargetPath.Attribute(targetPath), key.LogicalVolume.Attribute(lvName), key.Device.Attribute(lvDevice))
	logger.Info(ctx, "publishing volume")

	if err := ensureLV(ctx, lvName, "--type", "thin", "--thinpool", c.lvmVg+"/thinpool", "--name", volumeID, "--setactivationskip=n", c.lvmVg+"/base"); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create logical volume: %v", err)
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

		logger.Info(ctx, "mounting logical volume")
		if err := mounter.Mount(lvDevice, targetPath, c.LVMBaseDeviceFormat, mountOptions); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to mount logical volume %s to %s: %v", lvDevice, targetPath, err)
		}
	}

	if err := os.Chmod(targetPath, 0o775); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to change permissions of target path %s: %v", targetPath, err)
	}

	logger.Info(ctx, "mounted logical volume")
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

	lvName := c.lvmVg + "/" + volumeID
	lvDevice := "/dev/" + lvName

	ctx = logger.With(ctx, key.VolumeID.Field(volumeID), key.TargetPath.Field(targetPath), key.LogicalVolume.Field(lvName), key.Device.Field(lvDevice))
	trace.SpanFromContext(ctx).SetAttributes(key.VolumeID.Attribute(volumeID), key.TargetPath.Attribute(targetPath), key.LogicalVolume.Attribute(lvName), key.Device.Attribute(lvDevice))
	logger.Info(ctx, "unpublishing volume")

	notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "failed to check if target path %s is mounted: %v", targetPath, err)
	}

	if !notMounted {
		logger.Info(ctx, "unmounting target path")
		if err := mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmount logical volume at %s: %v", targetPath, err)
		}
	}

	if err := removeLV(ctx, lvName); err != nil {
		logger.Warn(ctx, "failed to remove logical volume", zap.Error(err))
	}

	logger.Info(ctx, "removed logical volume")
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

func (c *Cached) findThinpoolDevices(ctx context.Context) error {
	_, span := telemetry.Start(ctx, "cached.find-thinpool-devices")
	defer span.End()

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

//nolint:gocyclo // we should try to break this one up, but ignoring for now
func (c *Cached) PrepareBaseDevice(ctx context.Context, cacheVersion int64) error {
	ctx, span := telemetry.Start(ctx, "cached.prepare-base-device")
	defer span.End()

	if err := ensurePV(ctx, c.LVMBaseDevice); err != nil {
		return err
	}

	baseVg := "vg_dl_cache"
	baseLv := baseVg + "/base"
	baseLvDevice := "/dev/" + baseLv

	if err := ensureVG(ctx, baseVg, c.LVMBaseDevice); err != nil {
		if !strings.Contains(err.Error(), "already in volume group") {
			return err
		}

		// baseVg doesn't exist, but the base device is already in a
		// volume group, assume we already imported the base device and
		// the volume group is now named c.lvmVg
		baseVg = c.lvmVg
		baseLv = baseVg + "/base"
		baseLvDevice = "/dev/" + baseLv
	}

	if err := ensureLV(ctx, baseLv, "--type", "linear", "--extents", "100%FREE", "--name", "base", "-y", baseVg); err != nil {
		return err
	}

	out, err := exec.Output(ctx, "lvdisplay", baseLv)
	if err != nil {
		return fmt.Errorf("failed to display base volume %s: %w", baseLv, err)
	}

	if strings.Contains(out, "NOT available") {
		logger.Info(ctx, "base volume is NOT available, assuming the base volume has already been prepared")
		return nil
	}

	notMounted, err := mounter.IsLikelyNotMountPoint(c.StagingPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to check if staging directory %s is mounted: %w", c.StagingPath, err)
	}

	if notMounted {
		logger.Debug(ctx, "checking if base volume is formatted", key.Device.Field(baseLvDevice))
		var baseLvDeviceFormat string
		baseLvDeviceFormat, err = exec.Output(ctx, "blkid", "-o", "value", "-s", "TYPE", baseLvDevice)
		if err != nil && !strings.Contains(err.Error(), "exit status 2") {
			return fmt.Errorf("failed to get filesystem type of base device %s: %w", baseLvDevice, err)
		}

		if baseLvDeviceFormat != c.LVMBaseDeviceFormat {
			formatOptions := []string{baseLvDevice}
			if c.LVMBaseDeviceFormat == EXT4 {
				formatOptions = append(formatOptions, ext4FormatOptions()...)
			}

			logger.Info(ctx, "formatting base volume", key.Device.Field(baseLvDevice), zap.String("expected", c.LVMBaseDeviceFormat), zap.String("actual", baseLvDeviceFormat))
			if err := exec.Run(ctx, "mkfs."+c.LVMBaseDeviceFormat, formatOptions...); err != nil {
				return fmt.Errorf("failed to format base volume %s: %w", baseLvDevice, err)
			}
		}

		if err = os.MkdirAll(c.StagingPath, 0o775); err != nil {
			return fmt.Errorf("failed to create staging directory %s: %w", c.StagingPath, err)
		}

		var mountOptions []string
		if c.LVMBaseDeviceFormat == EXT4 {
			mountOptions = ext4MountOptions()
		}

		logger.Info(ctx, "mounting base volume", key.Device.Field(baseLvDevice), key.Path.Field(c.StagingPath))
		if err := mounter.Mount(baseLvDevice, c.StagingPath, c.LVMBaseDeviceFormat, mountOptions); err != nil {
			return fmt.Errorf("failed to mount base volume %s to staging directory %s: %w", baseLvDevice, c.StagingPath, err)
		}

		// ensure the base volume is unmounted when the function returns
		defer func() {
			if notMounted, _ := mounter.IsLikelyNotMountPoint(c.StagingPath); !notMounted {
				if err := mounter.Unmount(c.StagingPath); err != nil {
					logger.Error(ctx, "failed to unmount base volume", zap.Error(err))
				}
			}
		}()

		// Clean up lost+found directory, it's not needed and confusing.
		if err := os.RemoveAll(filepath.Join(c.StagingPath, "lost+found")); err != nil {
			return fmt.Errorf("failed to delete lost+found directory %s: %w", filepath.Join(c.StagingPath, "lost+found"), err)
		}
	}

	cacheRootDir := filepath.Join(c.StagingPath, CACHE_PATH_SUFFIX)
	cacheVersions := client.ReadCacheVersionFile(cacheRootDir)

	if (cacheVersion == -1 && len(cacheVersions) == 0) || !slices.Contains(cacheVersions, cacheVersion) {
		// if the cache version is -1 and there are no versions in the versions file, or the cache version is not in the versions file, then we need to download the cache
		var cachedCount uint32
		cacheVersion, cachedCount, err = c.Client.GetCache(ctx, cacheRootDir, cacheVersion)
		if err != nil {
			return err
		}

		logger.Info(ctx, "downloaded cache", key.CacheVersion.Field(cacheVersion), key.CachedCount.Field(cachedCount))

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

		if err := exec.Run(ctx, "fstrim", "-v", c.StagingPath); err != nil {
			logger.Warn(ctx, "failed to trim filesystem", zap.Error(err))
		}
	}

	if err := mounter.Unmount(c.StagingPath); err != nil {
		logger.Error(ctx, "failed to unmount base volume", zap.Error(err))
	}

	if err := exec.Run(ctx, "lvchange", "--permission", "r", baseLv); err != nil {
		return fmt.Errorf("failed to set permission on base volume %s: %w", baseLv, err)
	}

	if err := exec.Run(ctx, "lvchange", "--activate", "n", baseLv); err != nil {
		return fmt.Errorf("failed to deactivate base volume %s: %w", baseLv, err)
	}

	return nil
}

func (c *Cached) importBaseDevice(ctx context.Context) error {
	ctx, span := telemetry.Start(ctx, "cached.import-base-device")
	defer span.End()

	logger.Debug(ctx, "checking volume group", key.VolumeGroup.Field(c.lvmVg))

	err := exec.Run(ctx, "vgdisplay", c.lvmVg)
	if err == nil {
		logger.Info(ctx, "volume group already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", c.lvmVg, err)
	}

	logger.Info(ctx, "importing base device", key.VolumeGroup.Field(c.lvmVg), key.Device.Field(c.LVMBaseDevice))
	if err := exec.Run(ctx, "vgimportclone", "--basevgname", c.lvmVg, c.LVMBaseDevice); err != nil {
		return fmt.Errorf("failed to import base device %s: %w", c.LVMBaseDevice, err)
	}

	logger.Info(ctx, "extending volume group with thinpool devices", key.VolumeGroup.Field(c.lvmVg), key.DeviceGlob.Field(c.LVMThinpoolDeviceGlob))
	if err := exec.Run(ctx, "vgextend", append([]string{"--config=devices/allow_mixed_block_sizes=1", c.lvmVg}, c.lvmThinpoolDevices...)...); err != nil {
		return fmt.Errorf("failed to extend volume group with devices: %w", err)
	}

	return nil
}

// createThinPool creates the LVM thin pool if it doesn't exist
func (c *Cached) createThinPool(ctx context.Context) error {
	ctx, span := telemetry.Start(ctx, "cached.create-thin-pool")
	defer span.End()

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

		// Use one stripe per thinpool device to maximize performance
		"--stripes=" + strconv.Itoa(len(c.lvmThinpoolDevices)),

		// Use a small stripe size for better IO performance on small files
		"--stripesize=64k",

		// Skip zeroing for faster creation and better write performance
		// TODO: figure out data leakage risk
		"--zero=n",

		// Pass TRIM/discard commands through to underlying storage
		"--discards=passdown",

		// Pass the volume group the thin pool should be created in
		c.lvmVg,
	}

	// explicitly pass the devices to use for the thin pool
	lvCreateArgs = append(lvCreateArgs, c.lvmThinpoolDevices...)

	return ensureLV(ctx, thinPool, lvCreateArgs...)
}

func ensurePV(ctx context.Context, device string) error {
	ctx = logger.With(ctx, key.Device.Field(device))
	logger.Debug(ctx, "checking physical volume")

	err := exec.Run(ctx, "pvdisplay", device)
	if err == nil {
		logger.Info(ctx, "physical volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find physical volume") {
		return fmt.Errorf("failed to check lvm physical volume %s: %w", device, err)
	}

	logger.Info(ctx, "creating physical volume")
	if err := exec.Run(ctx, "pvcreate", device); err != nil && !strings.Contains(err.Error(), "signal: killed") {
		return fmt.Errorf("failed to create lvm physical volume %s: %w", device, err)
	}

	logger.Info(ctx, "physical volume created successfully")
	return nil
}

func removePV(ctx context.Context, device string) error {
	ctx = logger.With(ctx, key.Device.Field(device))
	logger.Debug(ctx, "checking physical volume for removal")

	err := exec.Run(ctx, "pvdisplay", device)
	if err != nil && !strings.Contains(err.Error(), "Failed to find physical volume") {
		return fmt.Errorf("failed to check lvm physical volume %s: %w", device, err)
	}

	if err == nil {
		logger.Info(ctx, "removing physical volume")
		if err := exec.Run(ctx, "pvremove", device); err != nil {
			return fmt.Errorf("failed to remove physical volume %s: %w", device, err)
		}
	}

	return nil
}

func ensureVG(ctx context.Context, vgName string, devices ...string) error {
	ctx = logger.With(ctx, key.VolumeGroup.Field(vgName), key.Device.Field(strings.Join(devices, ",")))
	logger.Debug(ctx, "checking volume group")

	err := exec.Run(ctx, "vgdisplay", vgName)
	if err == nil {
		logger.Debug(ctx, "volume group already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", vgName, err)
	}

	logger.Info(ctx, "creating volume group")
	if err := exec.Run(ctx, "vgcreate", append([]string{vgName}, devices...)...); err != nil {
		return fmt.Errorf("failed to create lvm volume group %s: %w", vgName, err)
	}

	return nil
}

func removeVG(ctx context.Context, vgName string) error {
	ctx = logger.With(ctx, key.VolumeGroup.Field(vgName))
	logger.Debug(ctx, "checking volume group for removal")

	err := exec.Run(ctx, "vgdisplay", vgName)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", vgName, err)
	}

	if err == nil {
		logger.Info(ctx, "removing volume group", key.VolumeGroup.Field(vgName))
		if err := exec.Run(ctx, "vgremove", "-y", vgName); err != nil {
			return fmt.Errorf("failed to remove lvm volume group %s: %w", vgName, err)
		}
	}

	return nil
}

func ensureLV(ctx context.Context, lvName string, lvCreateArgs ...string) error {
	ctx = logger.With(ctx, key.LogicalVolume.Field(lvName))
	logger.Debug(ctx, "checking logical volume")

	err := exec.Run(ctx, "lvdisplay", lvName)
	if err == nil {
		logger.Info(ctx, "logical volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check if logical volume %s exists: %w", lvName, err)
	}

	logger.Info(ctx, "creating logical volume")
	if err := exec.Run(ctx, "lvcreate", lvCreateArgs...); err != nil {
		return fmt.Errorf("failed to create logical volume %s: %w", lvName, err)
	}

	// Wait for device to appear and settle udev
	if err := udevSettle(ctx, "/dev/"+lvName); err != nil {
		// keep going, the device might still be available
		logger.Warn(ctx, "udev settle failed for logical volume", zap.Error(err))
	}

	return nil
}

func removeLV(ctx context.Context, lvName string) error {
	ctx = logger.With(ctx, key.LogicalVolume.Field(lvName))
	logger.Debug(ctx, "checking logical volume for removal")

	err := exec.Run(ctx, "lvdisplay", lvName)
	if err != nil && !strings.Contains(err.Error(), "Failed to find logical volume") && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check if logical volume %s exists: %w", lvName, err)
	}

	if err == nil {
		logger.Info(ctx, "removing logical volume", key.LogicalVolume.Field(lvName))
		if err := exec.Run(ctx, "lvremove", "-y", lvName); err != nil {
			return fmt.Errorf("failed to remove logical volume %s: %w", lvName, err)
		}
	}

	return nil
}

// udevSettle triggers udev events and waits for device to appear
func udevSettle(ctx context.Context, device string) error {
	// Trigger udev events for the device
	if err := exec.Run(ctx, "udevadm", "trigger", "--action=add", device); err != nil {
		logger.Warn(ctx, "udevadm trigger failed", zap.Error(err))
		// Continue anyway, the device might still be available
	}

	// Wait for udev to settle with timeout
	settleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := exec.Run(settleCtx, "udevadm", "settle", "--exit-if-exists="+device); err != nil {
		logger.Warn(ctx, "udev settle failed", zap.Error(err))
		// Fallback to polling
		return waitForDevice(ctx, device, 5*time.Second)
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
		"-O", "extent,dir_index,sparse_super2,filetype,flex_bg,64bit,inline_data,^has_journal,^metadata_csum",

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

		// Disable delayed allocation - can help with small file workloads
		// Forces immediate allocation which can reduce fragmentation for node_modules
		"nodelalloc",

		// Enable discard for SSD/NVMe TRIM support
		"discard",

		// Note: data=writeback and commit options are journal-related
		// Since we disabled journaling (^has_journal), these are not needed

		// Continue on errors rather than remounting read-only
		"errors=continue",
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
