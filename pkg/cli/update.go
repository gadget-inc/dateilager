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
		checkGlobs []string
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
				key.DiffCount.Field(DiffUpdateCount(diff)),
			)

			if diff != nil {
				err = LogIfGlobsMatched(checkGlobs, diff)
				if err != nil {
					return fmt.Errorf("could not check for matching globs: %w", err)
				}

				if logUpdates {
					LogUpdates(diff)
				}
			}

			fmt.Println(version)

			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&dir, "dir", "", "Directory containing updated files")
	cmd.Flags().BoolVar(&logUpdates, "log-updates", false, "Log all updated files to the console")
	cmd.Flags().StringSliceVar(&checkGlobs, "check-glob", []string{}, "Report if any files matching the given globs were changed by this update operation")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
