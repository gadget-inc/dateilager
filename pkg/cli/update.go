package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

type updateArgs struct {
	project int64
	dir     string
}

func NewCmdUpdate(b client.ClientBuilder) *cobra.Command {
	a := updateArgs{}

	cmd := &cobra.Command{
		Use: "update",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client, err := b.Build(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			version, count, err := client.Update(ctx, a.project, a.dir)
			if err != nil {
				return fmt.Errorf("update objects: %w", err)
			}

			logger.Info(ctx, "updated objects",
				key.Project.Field(a.project),
				key.Version.Field(version),
				key.DiffCount.Field(count),
			)
			fmt.Println(version)

			return nil
		},
	}

	cmd.Flags().Int64Var(&a.project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&a.dir, "dir", "", "Directory containing updated files")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
