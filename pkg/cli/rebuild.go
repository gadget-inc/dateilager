package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

type rebuildArgs struct {
	project int64
	to      *int64
	prefix  string
	dir     string
}

func NewCmdRebuild(b client.ClientBuilder) *cobra.Command {
	a := rebuildArgs{}

	cmd := &cobra.Command{
		Use: "rebuild",
		RunE: func(cmd *cobra.Command, args []string) error {
			if *a.to == -1 {
				a.to = nil
			}

			ctx := cmd.Context()

			client, err := b.Build(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			version, count, err := client.Rebuild(ctx, a.project, a.prefix, a.to, a.dir)
			if err != nil {
				return fmt.Errorf("could not rebuild project: %w", err)
			}

			if version == -1 {
				logger.Debug(ctx, "latest version already checked out",
					key.Project.Field(a.project),
					key.Directory.Field(a.dir),
					key.ToVersion.Field(a.to),
				)
			} else {
				logger.Info(ctx, "wrote files",
					key.Project.Field(a.project),
					key.Directory.Field(a.dir),
					key.Version.Field(version),
					key.DiffCount.Field(count),
				)
			}

			fmt.Println(version)
			return nil
		},
	}

	cmd.Flags().Int64Var(&a.project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&a.prefix, "prefix", "", "Search prefix")
	cmd.Flags().StringVar(&a.dir, "dir", "", "Output directory")
	a.to = cmd.Flags().Int64("to", -1, "To version ID (optional)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
