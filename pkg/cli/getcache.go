package cli

import (
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdGetCache() *cobra.Command {
	var (
		path    string
		version int64
	)

	cmd := &cobra.Command{
		Use: "getcache",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c := client.FromContext(ctx)

			version, _, err := c.GetCache(ctx, path, version)
			if err != nil {
				return err
			}

			logger.Info(ctx, "cache built", key.Version.Field(version))

			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Cache directory")
	cmd.Flags().Int64Var(&version, "version", -1, "Cache version")
	_ = cmd.MarkFlagRequired("path")

	return cmd
}
