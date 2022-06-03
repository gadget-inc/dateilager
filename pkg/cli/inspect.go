package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdInspect() *cobra.Command {
	var project int64

	cmd := &cobra.Command{
		Use: "inspect",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			inspect, err := client.Inspect(ctx, project)
			if err != nil {
				return fmt.Errorf("inspect project: %w", err)
			}

			logger.Info(ctx, "inspect objects",
				key.Project.Field(project),
				key.LatestVersion.Field(inspect.LatestVersion),
				key.LiveObjectsCount.Field(inspect.LiveObjectsCount),
				key.TotalObjectsCount.Field(inspect.TotalObjectsCount),
			)

			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
