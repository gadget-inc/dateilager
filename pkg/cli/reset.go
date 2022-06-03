package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

type resetArgs struct {
	state string
}

func NewCmdReset(b client.ClientBuilder) *cobra.Command {
	a := resetArgs{}

	cmd := &cobra.Command{
		Use: "reset",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client, err := b.Build(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			err = client.Reset(ctx, a.state)
			if err != nil {
				return fmt.Errorf("reset: %w", err)
			}

			logger.Info(ctx, "successful reset", key.State.Field(a.state))
			return nil
		},
	}

	cmd.Flags().StringVar(&a.state, "state", "", "State string from a snapshot command")

	return cmd
}
