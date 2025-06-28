package cached

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/exec"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/lvm"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/client"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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
	BaseLV           string
	BaseLVFormat     string
	BaseLVMountPoint string
	BasePV           string
	CacheGid         int
	CacheUid         int
	Client           *client.Client
	NameSuffix       string
	ThinpoolLV       string
	ThinpoolPVGlobs  string
	ThinpoolPVs      []string
	VG               string
	prepared         atomic.Bool
}

func New(client *client.Client, nameSuffix string) *Cached {
	vg := "vg_dateilager_cached" + strings.ReplaceAll(nameSuffix, "-", "_")
	baseLV := vg + "/base"
	thinpoolLV := vg + "/thinpool"

	return &Cached{
		BaseLV:           first(os.Getenv("DL_BASE_LV"), baseLV),
		BaseLVFormat:     os.Getenv("DL_BASE_LV_FORMAT"),
		BaseLVMountPoint: path.Join("/mnt", baseLV),
		BasePV:           os.Getenv("DL_BASE_PV"),
		CacheGid:         NO_CHANGE_USER,
		CacheUid:         NO_CHANGE_USER,
		Client:           client,
		NameSuffix:       nameSuffix,
		ThinpoolLV:       thinpoolLV,
		ThinpoolPVGlobs:  os.Getenv("DL_THINPOOL_PV_GLOBS"),
		VG:               vg,
	}
}

func (c *Cached) Prepare(ctx context.Context, cacheVersion int64) error {
	logger.Info(ctx, "preparing cached", key.CacheVersion.Field(cacheVersion))
	ctx, span := telemetry.Start(ctx, "cached.prepare", trace.WithAttributes(key.CacheVersion.Attribute(cacheVersion)))
	defer span.End()

	if err := c.findThinpoolPVs(ctx); err != nil {
		return err
	}

	if err := c.PrepareBasePV(ctx, cacheVersion); err != nil {
		return err
	}

	if err := c.importBasePV(ctx); err != nil {
		return err
	}

	if err := c.createThinpool(ctx); err != nil {
		return err
	}

	c.prepared.Store(true)

	return nil
}

// Unprepare removes the cached storage
func (c *Cached) Unprepare(ctx context.Context) error {
	logger.Info(ctx, "unpreparing cached",
		key.LV.Field(c.ThinpoolLV),
		key.LV.Field(c.BaseLV),
		key.VG.Field(c.VG),
		key.PVs.Field(c.ThinpoolPVs),
		key.PV.Field(c.BasePV),
	)

	if err := lvm.RemoveLV(ctx, c.ThinpoolLV); err != nil {
		return err
	}

	if err := lvm.RemoveLV(ctx, c.BaseLV); err != nil {
		return err
	}

	if err := lvm.RemoveVG(ctx, c.VG); err != nil {
		return err
	}

	for _, pv := range c.ThinpoolPVs {
		if err := lvm.RemovePV(ctx, pv); err != nil {
			return err
		}
	}

	if err := lvm.RemovePV(ctx, c.BasePV); err != nil {
		return err
	}

	return nil
}

func (c *Cached) findThinpoolPVs(ctx context.Context) error {
	_, span := telemetry.Start(ctx, "cached.find-thinpool-pvs")
	defer span.End()

	var pvs []string
	for glob := range strings.SplitSeq(c.ThinpoolPVGlobs, ",") {
		glob = strings.TrimSpace(glob)
		devices, err := filepath.Glob(glob)
		if err != nil {
			return fmt.Errorf("failed to find lvm devices for glob %s: %w", glob, err)
		}
		pvs = append(pvs, devices...)
	}

	if len(pvs) == 0 {
		return fmt.Errorf("no devices found for globs %s", c.ThinpoolPVGlobs)
	}

	c.ThinpoolPVs = pvs

	return nil
}

var mounter = mount.New("")

//nolint:gocyclo // we should try to break this one up, but ignoring for now
func (c *Cached) PrepareBasePV(ctx context.Context, cacheVersion int64) error {
	ctx, span := telemetry.Start(ctx, "cached.prepare-base-pv")
	defer span.End()

	if err := lvm.EnsurePV(ctx, c.BasePV); err != nil {
		return err
	}

	vg := first(os.Getenv("DL_CACHE_VG"), "vg_dl_cache")

	if err := lvm.EnsureVG(ctx, vg, c.BasePV); err != nil {
		if !strings.Contains(err.Error(), "already in volume group") {
			return err
		}

		// base physical volume is already in a volume group. Assume we
		// vgimportclone'd the base physical volume and renamed the
		// hardcoded cache volume group to c.VG
		vg = c.VG
	}

	lv := vg + "/base"
	lvDevice := "/dev/" + lv

	if err := lvm.EnsureLV(ctx, lv, "--type", "linear", "--extents", "100%FREE", "--name", "base", "-y", vg); err != nil {
		return err
	}

	out, err := exec.Output(ctx, "lvdisplay", lv)
	if err != nil {
		return fmt.Errorf("failed to display base volume %s: %w", lv, err)
	}

	if strings.Contains(out, "read only") {
		logger.Info(ctx, "base volume is read only, assuming the base volume has already been prepared")

		// ensure the base volume is deactivated so that we can vgimportclone it later
		if err := exec.Run(ctx, "lvchange", "--activate", "n", lv); err != nil {
			return fmt.Errorf("failed to deactivate base volume %s: %w", lv, err)
		}
		return nil
	}

	notMounted, err := mounter.IsLikelyNotMountPoint(c.BaseLVMountPoint)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to check if %s is mounted: %w", c.BaseLVMountPoint, err)
	}

	if notMounted {
		logger.Debug(ctx, "checking if base lv is formatted", key.LV.Field(lv))
		var baseLvDeviceFormat string
		baseLvDeviceFormat, err = exec.Output(ctx, "blkid", "-o", "value", "-s", "TYPE", lvDevice)
		if err != nil && !strings.Contains(err.Error(), "exit status 2") {
			return fmt.Errorf("failed to get filesystem type of base device %s: %w", lvDevice, err)
		}

		if baseLvDeviceFormat != c.BaseLVFormat {
			formatOptions := []string{lvDevice}
			if c.BaseLVFormat == EXT4 {
				formatOptions = append(formatOptions, EXT4FormatOptions()...)
			}

			logger.Info(ctx, "formatting base lv", key.LV.Field(lv), zap.String("expected", c.BaseLVFormat), zap.String("actual", baseLvDeviceFormat))
			if err := exec.Run(ctx, "mkfs."+c.BaseLVFormat, formatOptions...); err != nil {
				return fmt.Errorf("failed to format base lv %s: %w", lvDevice, err)
			}
		}

		if err = os.MkdirAll(c.BaseLVMountPoint, 0o775); err != nil {
			return fmt.Errorf("failed to mkdir %s: %w", c.BaseLVMountPoint, err)
		}

		var mountOptions []string
		if c.BaseLVFormat == EXT4 {
			mountOptions = EXT4MountOptions()
		}

		logger.Info(ctx, "mounting base lv", key.LV.Field(lv), key.Path.Field(c.BaseLVMountPoint))
		if err := mounter.Mount(lvDevice, c.BaseLVMountPoint, c.BaseLVFormat, mountOptions); err != nil {
			return fmt.Errorf("failed to mount %s to %s: %w", lvDevice, c.BaseLVMountPoint, err)
		}

		// ensure the base lv is unmounted when the function returns
		defer func() {
			if notMounted, _ := mounter.IsLikelyNotMountPoint(c.BaseLVMountPoint); !notMounted {
				if err := mounter.Unmount(c.BaseLVMountPoint); err != nil {
					logger.Error(ctx, "failed to unmount base lv", zap.Error(err))
				}
			}
		}()

		// Clean up lost+found directory, it's not needed and confusing.
		if err := os.RemoveAll(filepath.Join(c.BaseLVMountPoint, "lost+found")); err != nil {
			return fmt.Errorf("failed to delete %s: %w", filepath.Join(c.BaseLVMountPoint, "lost+found"), err)
		}
	}

	cacheRootDir := filepath.Join(c.BaseLVMountPoint, CACHE_PATH_SUFFIX)
	cacheVersions := client.ReadCacheVersionFile(cacheRootDir)

	if cacheVersion == -1 || !slices.Contains(cacheVersions, cacheVersion) {
		logger.Info(ctx, "downloading cache", key.CacheVersion.Field(cacheVersion))
		startTime := time.Now()

		var cachedCount uint32
		cacheVersion, cachedCount, err = c.Client.GetCache(ctx, cacheRootDir, cacheVersion)
		if err != nil {
			return err
		}

		logger.Info(ctx, "downloaded cache",
			key.Path.Field(cacheRootDir),
			key.CacheVersion.Field(cacheVersion),
			key.CachedCount.Field(cachedCount),
			key.DurationMS.Field(time.Since(startTime)),
		)

		if c.CacheUid != NO_CHANGE_USER || c.CacheGid != NO_CHANGE_USER {
			logger.Info(ctx, "setting ownership of cache", zap.Int("uid", c.CacheUid), zap.Int("gid", c.CacheGid))
			err := fastwalk.Walk(nil, c.BaseLVMountPoint, func(walkPath string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				return os.Lchown(walkPath, c.CacheUid, c.CacheGid)
			})
			if err != nil {
				return fmt.Errorf("failed to set ownership of %s: %w", c.BaseLVMountPoint, err)
			}
		}

		if err := exec.Run(ctx, "fstrim", "-v", c.BaseLVMountPoint); err != nil {
			logger.Warn(ctx, "failed to trim filesystem", zap.Error(err))
		}
	}

	logger.Info(ctx, "unmounting base lv", key.Path.Field(c.BaseLVMountPoint), key.Device.Field(lvDevice))
	if err := mounter.Unmount(c.BaseLVMountPoint); err != nil {
		logger.Error(ctx, "failed to unmount base lv", zap.Error(err))
	}

	logger.Info(ctx, "making base lv read-only", key.LV.Field(lv))
	if err := exec.Run(ctx, "lvchange", "--permission", "r", lv); err != nil {
		return fmt.Errorf("failed to set permission on base lv %s: %w", lv, err)
	}

	logger.Info(ctx, "deactivating base lv", key.LV.Field(lv))
	if err := exec.Run(ctx, "lvchange", "--activate", "n", lv); err != nil {
		return fmt.Errorf("failed to deactivate base lv %s: %w", lv, err)
	}

	logger.Info(ctx, "prepared base pv",
		key.PV.Field(c.BasePV),
		key.VG.Field(vg),
		key.LV.Field(lv),
		key.Device.Field(lvDevice),
	)

	return nil
}

func (c *Cached) importBasePV(ctx context.Context) error {
	ctx = logger.With(ctx, key.PV.Field(c.BasePV), key.VG.Field(c.VG))
	ctx, span := telemetry.Start(ctx, "cached.import-base-pv")
	defer span.End()

	logger.Debug(ctx, "checking volume group")
	err := exec.Run(ctx, "vgdisplay", c.VG)
	if err == nil {
		logger.Info(ctx, "volume group already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check vg %s: %w", c.VG, err)
	}

	logger.Info(ctx, "importing base pv")
	if err := exec.Run(ctx, "vgimportclone", "-n", c.VG, c.BasePV); err != nil {
		return fmt.Errorf("failed to import base pv %s: %w", c.BasePV, err)
	}

	logger.Info(ctx, "extending vg with thinpool pvs", key.DeviceGlobs.Field(c.ThinpoolPVGlobs), key.PVs.Field(c.ThinpoolPVs))
	if err := exec.Run(ctx, "vgextend", append([]string{"--config=devices/allow_mixed_block_sizes=1", c.VG}, c.ThinpoolPVs...)...); err != nil {
		return fmt.Errorf("failed to extend volume group with devices: %w", err)
	}

	return nil
}

// createThinpool creates the LVM thin pool if it doesn't exist
func (c *Cached) createThinpool(ctx context.Context) error {
	ctx = logger.With(ctx, key.LV.Field(c.ThinpoolLV))
	ctx, span := telemetry.Start(ctx, "cached.create-thin-pool")
	defer span.End()

	logger.Debug(ctx, "checking thin pool lv")
	err := exec.Run(ctx, "lvdisplay", c.ThinpoolLV)
	if err == nil {
		logger.Info(ctx, "thin pool already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check lvm thin pool %s: %w", c.ThinpoolLV, err)
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
		"--stripes=" + strconv.Itoa(len(c.ThinpoolPVs)),

		// Use a small stripe size for better IO performance on small files
		"--stripesize=64k",

		// Skip zeroing for faster creation and better write performance
		// TODO: figure out data leakage risk
		"--zero=n",

		// Pass TRIM/discard commands through to underlying storage
		"--discards=passdown",

		// Pass the volume group the thin pool should be created in
		c.VG,
	}

	// explicitly pass the devices to use for the thin pool
	lvCreateArgs = append(lvCreateArgs, c.ThinpoolPVs...)

	return lvm.EnsureLV(ctx, c.ThinpoolLV, lvCreateArgs...)
}

// EXT4FormatOptions returns the format options for ext4 filesystems optimized for node_modules
func EXT4FormatOptions() []string {
	return []string{
		// Force creation even if the target looks mounted or already formatted
		"-F",

		// 4 KiB logical blocks - better balance for small files on modern storage
		"-b", "4096",

		// One inode per 4 KiB of projected data. 256-byte inodes instead of 128, required for inline_data.
		"-i", "4096", "-I", "256",

		// Zero per-FS reserve since this is a dedicated node_modules volume
		"-m", "0",

		// Larger flex_bg groups for better allocation - 64 block groups
		"-G", "64",

		// Optimized feature flags for write performance and small files
		"-O", "extent,dir_index,sparse_super2,filetype,flex_bg,64bit,inline_data,^has_journal",

		// Extended parameters optimized for NVMe and small files:
		//   No stride/stripe-width for better flexibility
		//   lazy_itable_init=0 for predictable performance
		//   nodiscard during format for faster creation
		"-E", "lazy_itable_init=0,nodiscard,num_backup_sb=0",
	}
}

// EXT4MountOptions returns mount options optimized for maximum write performance
func EXT4MountOptions() []string {
	return []string{
		// Continue on errors rather than remounting read-only
		"errors=continue",

		// Eliminate all timestamp updates for maximum performance
		"noatime", "lazytime",

		// Disable write barriers - assumes battery-backed storage or acceptable data loss risk
		"nobarrier",

		// Disable delayed allocation - can help with small file workloads
		// Forces immediate allocation which can reduce fragmentation for node_modules
		"nodelalloc",

		// Enable discard for SSD/NVMe TRIM support
		"discard",

		// Note: data=writeback and commit options are journal-related
		// Since we disabled journaling (^has_journal), these will cause mount to fail
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
