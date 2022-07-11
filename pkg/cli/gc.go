package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdGc() *cobra.Command {
	var (
		project int64
		keep    int64
		from    *int64
	)

	cmd := &cobra.Command{
		Use: "gc",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if *from == -1 {
				from = nil
			}

			ctx := cmd.Context()

			c := client.FromContext(ctx)

			count, err := c.GcProject(ctx, project, keep, from)
			if err != nil {
				return fmt.Errorf("could not gc project %v: %w", project, err)
			}

			logger.Info(ctx, "cleaned object", key.Project.Field(project), key.Count.Field(count))

			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().Int64Var(&keep, "keep", -1, "Amount of versions to keep (required)")
	from = cmd.Flags().Int64("from", -1, "Delete as of this version")

	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("keep")

	return cmd
}
