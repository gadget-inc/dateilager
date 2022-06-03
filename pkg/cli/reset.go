package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdReset() *cobra.Command {
	var state string

	cmd := &cobra.Command{
		Use: "reset",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			err := client.Reset(ctx, state)
			if err != nil {
				return fmt.Errorf("reset: %w", err)
			}

			logger.Info(ctx, "successful reset", key.State.Field(state))
			return nil
		},
	}

	cmd.Flags().StringVar(&state, "state", "", "State string from a snapshot command")

	return cmd
}
