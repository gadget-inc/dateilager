package cli

import (
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdGetCacheFromDaemon() *cobra.Command {
	var (
		path string
	)

	cmd := &cobra.Command{
		Use: "getcached",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c := client.CachedFromContext(ctx)

			version, err := c.PopulateDiskCache(ctx, path)
			if err != nil {
				return err
			}

			logger.Info(ctx, "cache populated", key.Version.Field(version))

			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Cache directory")

	_ = cmd.MarkFlagRequired("path")

	return cmd
}
