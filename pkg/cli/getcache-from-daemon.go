package cli

import (
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdGetCacheFromDaemon() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "getcache-from-daemon <destination_path>",
		Args: cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c := client.FromContext(ctx)

			version, err := c.PopulateDiskCache(ctx, args[0])
			if err != nil {
				return err
			}

			logger.Info(ctx, "cache populated", key.Version.Field(version))

			return nil
		},
	}

	return cmd
}
