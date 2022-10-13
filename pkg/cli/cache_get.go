package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdCacheGet() *cobra.Command {
	var cacheRootDir string

	cmd := &cobra.Command{
		Use: "cache-get",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			version, err := client.GetCache(ctx, cacheRootDir)
			if err != nil {
				return fmt.Errorf("could not fetch data: %w", err)
			}

			logger.Info(ctx, "successfully downloaded cache version", key.CacheVersion.Field(version))

			return nil
		},
	}

	cmd.Flags().StringVar(&cacheRootDir, "cache-root-dir", "/cache", "Cache root directory (required)")
	_ = cmd.MarkFlagRequired("cache-root-dir")

	return cmd
}
