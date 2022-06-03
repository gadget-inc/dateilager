package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

type inspectArgs struct {
	project int64
}

func NewCmdInspect(b client.ClientBuilder) *cobra.Command {
	a := inspectArgs{}

	cmd := &cobra.Command{
		Use: "inspect",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client, err := b.Build(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			inspect, err := client.Inspect(ctx, a.project)
			if err != nil {
				return fmt.Errorf("inspect project: %w", err)
			}

			logger.Info(ctx, "inspect objects",
				key.Project.Field(a.project),
				key.LatestVersion.Field(inspect.LatestVersion),
				key.LiveObjectsCount.Field(inspect.LiveObjectsCount),
				key.TotalObjectsCount.Field(inspect.TotalObjectsCount),
			)

			return nil
		},
	}

	cmd.Flags().Int64Var(&a.project, "project", -1, "Project ID (required)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
