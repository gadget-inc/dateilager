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
	flags.IntVar(&cacheGid, "cache-gid", cached.NO_CHANGE_USER, "gid for cache files")
	flags.IntVar(&cacheUid, "cache-uid", cached.NO_CHANGE_USER, "uid for cache files")
	flags.StringVar(&baseLVFormat, "base-lv-format", firstNonEmpty(os.Getenv("DL_BASE_LV_FORMAT"), cached.EXT4), "filesystem format to use for the base LV")
	flags.StringVar(&basePV, "base-pv", os.Getenv("DL_BASE_PV"), "PV to use for the base LV")

	return cmd
}
