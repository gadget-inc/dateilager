package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdCacheCreate() *cobra.Command {
	var prefix string
	var count int32

	cmd := &cobra.Command{
		Use: "cache-create",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			version, err := client.CreateCache(ctx, prefix, count)
			if err != nil {
				return fmt.Errorf("could not fetch data: %w", err)
			}

			logger.Info(ctx, "successfully created a new cache version", key.CacheVersion.Field(version))

			return nil
		},
	}

	cmd.Flags().StringVar(&prefix, "prefix", "node_modules", "Prefix of packed objects path")
	cmd.Flags().Int32Var(&count, "count", 100, "Number of packed objects to include in the cache")

	return cmd
}
