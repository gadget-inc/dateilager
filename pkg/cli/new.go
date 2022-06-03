package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdNew() *cobra.Command {
	var (
		id       int64
		template int64
		patterns string
	)

	cmd := &cobra.Command{
		Use: "new",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client := client.FromContext(ctx)

			err := client.NewProject(ctx, id, &template, patterns)
			if err != nil {
				return fmt.Errorf("could not create new project: %w", err)
			}

			logger.Info(ctx, "created new project", key.Project.Field(id))
			return nil
		},
	}

	cmd.Flags().Int64Var(&id, "id", -1, "Project ID (required)")
	cmd.Flags().Int64Var(&template, "template", -1, "Template ID")
	cmd.Flags().StringVar(&patterns, "patterns", "", "Comma separated pack patterns")

	_ = cmd.MarkFlagRequired("id")

	return cmd
}
