package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdGet() *cobra.Command {
	var (
		project int64
		to      *int64
		from    *int64
		prefix  string
	)

	cmd := &cobra.Command{
		Use: "get",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if *from == -1 {
				from = nil
			}
			if *to == -1 {
				to = nil
			}

			vrange := client.VersionRange{From: from, To: to}

			ctx := cmd.Context()

			client := client.FromContext(ctx)

			objects, err := client.Get(ctx, project, prefix, nil, vrange)
			if err != nil {
				return fmt.Errorf("could not fetch data: %w", err)
			}

			logger.Info(ctx, "listing objects in project", key.Project.Field(project), key.ObjectsCount.Field(len(objects)))
			for _, object := range objects {
				logger.Info(ctx, "object", key.ObjectPath.Field(object.Path), key.ObjectContent.Field(string(object.Content)))
			}

			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Search prefix")
	from = cmd.Flags().Int64("from", -1, "From version ID (optional)")
	to = cmd.Flags().Int64("to", -1, "To version ID (optional)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
