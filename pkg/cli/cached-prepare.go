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
		cacheVersion int64
		cacheUid     int
		cacheGid     int
		basePV       string
		baseLVFormat string
	)

	cmd := &cobra.Command{
		Use:               "prepare",
		Short:             "Prepare a base PV for later use by cached server",
		DisableAutoGenTag: true,
		Version:           version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			c := cached.New(client.FromContext(ctx), "")
			c.CacheUid = cacheUid
			c.CacheGid = cacheGid
			c.BasePV = basePV
			c.BaseLVFormat = baseLVFormat

			return c.PrepareBasePV(ctx, cacheVersion)
		},
	}

	flags := cmd.PersistentFlags()

	flags.Int64Var(&cacheVersion, "cache-version", -1, "cache version to use")
	flags.IntVar(&cacheUid, "cache-uid", -1, "uid for cache files")
	flags.IntVar(&cacheGid, "cache-gid", -1, "gid for cache files")
	flags.StringVar(&basePV, "base-pv", os.Getenv("DL_BASE_PV"), "lvm base physical volume to use")
	flags.StringVar(&baseLVFormat, "base-lv-format", firstNonEmpty(os.Getenv("DL_BASE_LV_FORMAT"), cached.XFS), "lvm base logical volume format to use")

	_ = cmd.MarkPersistentFlagRequired("base-pv")

	return cmd
}
