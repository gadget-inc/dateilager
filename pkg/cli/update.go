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
		project    int64
		dir        string
		logUpdates bool
	)

	cmd := &cobra.Command{
		Use: "update",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			version, diff, err := client.Update(ctx, project, dir)
			if err != nil {
				return fmt.Errorf("update objects: %w", err)
			}

			logger.Info(ctx, "updated objects",
				key.Project.Field(project),
				key.Version.Field(version),
				key.DiffCount.Field(uint32(len(diff.Updates))),
			)

			if logUpdates {
				LogUpdates(diff)
			}

			fmt.Println(version)

			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&dir, "dir", "", "Directory containing updated files")
	cmd.Flags().BoolVar(&logUpdates, "log-updates", false, "Log all updated files to the console")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
