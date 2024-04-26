package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdCommit() *cobra.Command {
	var (
		project int64
		version int64
	)

	cmd := &cobra.Command{
		Use: "commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			err := client.CommitUpdate(ctx, project, version)
			if err != nil {
				return fmt.Errorf("commit objects: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().Int64Var(&version, "version", -1, "Staged version to commit")

	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("version")

	return cmd
}
