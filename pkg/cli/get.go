package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

type getArgs struct {
	project int64
	to      *int64
	from    *int64
	prefix  string
}

func NewCmdGet(b client.ClientBuilder) *cobra.Command {
	a := getArgs{}

	cmd := &cobra.Command{
		Use: "get",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if *a.from == -1 {
				a.from = nil
			}
			if *a.to == -1 {
				a.to = nil
			}

			vrange := client.VersionRange{From: a.from, To: a.to}

			ctx := cmd.Context()

			client, err := b.Build(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			objects, err := client.Get(ctx, a.project, a.prefix, nil, vrange)
			if err != nil {
				return fmt.Errorf("could not fetch data: %w", err)
			}

			logger.Info(ctx, "listing objects in project", key.Project.Field(a.project), key.ObjectsCount.Field(len(objects)))
			for _, object := range objects {
				logger.Info(ctx, "object", key.ObjectPath.Field(object.Path), key.ObjectContent.Field(string(object.Content)[:10]))
			}

			return nil
		},
	}

	cmd.Flags().Int64Var(&a.project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&a.prefix, "prefix", "", "Search prefix")
	a.from = cmd.Flags().Int64("from", -1, "From version ID (optional)")
	a.to = cmd.Flags().Int64("to", -1, "To version ID (optional)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
