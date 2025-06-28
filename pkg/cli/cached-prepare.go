package cli

import (
	"os"

	"github.com/gadget-inc/dateilager/pkg/cached"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
)

func NewCachedPrepareCommand() *cobra.Command {
	var (
		stagingPath         string
		cacheVersion        int64
		cacheUid            int
		cacheGid            int
		lvmBaseDevice       string
		lvmBaseDeviceFormat string
	)

	cmd := &cobra.Command{
		Use:               "prepare",
		Short:             "DateiLager cache daemon prepare",
		DisableAutoGenTag: true,
		Version:           version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			c := cached.New(client.FromContext(ctx), "")
			c.BaseLVMountPoint = stagingPath
			c.CacheUid = cacheUid
			c.CacheGid = cacheGid
			c.BasePV = lvmBaseDevice
			c.BaseLVFormat = lvmBaseDeviceFormat

			return c.PrepareBasePV(ctx, cacheVersion)
		},
	}

	flags := cmd.PersistentFlags()

	flags.StringVar(&stagingPath, "staging-path", "", "path for staging downloaded caches")
	flags.Int64Var(&cacheVersion, "cache-version", -1, "cache version to use")
	flags.IntVar(&cacheUid, "cache-uid", -1, "uid for cache files")
	flags.IntVar(&cacheGid, "cache-gid", -1, "gid for cache files")
	flags.StringVar(&lvmBaseDevice, "lvm-base-device", os.Getenv("DL_BASE_PV"), "lvm base device to use")
	flags.StringVar(&lvmBaseDeviceFormat, "lvm-base-device-format", firstNonEmpty(os.Getenv("DL_BASE_LV_FORMAT"), "ext4"), "lvm base device format to use")

	_ = cmd.MarkPersistentFlagRequired("staging-path")
	_ = cmd.MarkPersistentFlagRequired("lvm-base-device")

	return cmd
}
