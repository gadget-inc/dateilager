package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdSnapshot() *cobra.Command {
	return &cobra.Command{
		Use: "snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			state, err := client.Snapshot(ctx)
			if err != nil {
				return fmt.Errorf("snapshot: %w", err)
			}

			logger.Info(ctx, "successful snapshot")
			fmt.Println(state)

			return nil
		},
	}
}
