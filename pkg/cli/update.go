package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdUpdate() *cobra.Command {
	var (
		project  int64
		dir      string
		subpaths []string
	)

	cmd := &cobra.Command{
		Use: "update",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			version, count, err := client.Update(ctx, project, dir, subpaths)
			if err != nil {
				return fmt.Errorf("update objects: %w", err)
			}

			logger.Info(ctx, "updated objects",
				key.Project.Field(project),
				key.Version.Field(version),
				key.DiffCount.Field(count),
			)
			fmt.Println(version)

			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&dir, "dir", "", "Directory containing updated files")
	cmd.Flags().StringSliceVar(&subpaths, "subpaths", nil, "Subpaths to update")
	_ = cmd.MarkFlagRequired("project")

	return cmd
}
