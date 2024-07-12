package cachedcli

import (
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func NewCmdProbe() *cobra.Command {
	cmd := &cobra.Command{
		Use: "probe",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c := client.CachedFromContext(ctx)

			ready, err := c.Probe(ctx)
			if err != nil {
				return err
			}

			logger.Info(ctx, "server probe", zap.Bool("ready", ready))
			return nil
		},
	}

	return cmd
}
