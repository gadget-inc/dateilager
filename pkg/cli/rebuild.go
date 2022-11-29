package cli

import (
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdRebuild() *cobra.Command {
	var (
		project   int64
		to        *int64
		prefix    string
		dir       string
		ignores   string
		summarize bool
	)

	cmd := &cobra.Command{
		Use: "rebuild",
		RunE: func(cmd *cobra.Command, args []string) error {
			if *to == -1 {
				to = nil
			}

			ctx := cmd.Context()
			client := client.FromContext(ctx)

			var ignoreList []string
			if len(ignores) > 0 {
				ignoreList = strings.Split(ignores, ",")
			}

			version, count, err := client.Rebuild(ctx, project, prefix, to, dir, ignoreList, "", summarize)
			if err != nil {
				return fmt.Errorf("could not rebuild project: %w", err)
			}

			if version == -1 {
				logger.Debug(ctx, "latest version already checked out",
					key.Project.Field(project),
					key.Directory.Field(dir),
					key.ToVersion.Field(to),
				)
			} else {
				logger.Info(ctx, "wrote files",
					key.Project.Field(project),
					key.Directory.Field(dir),
					key.Version.Field(version),
					key.DiffCount.Field(count),
				)
			}

			fmt.Println(version)
			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Search prefix")
	cmd.Flags().StringVar(&dir, "dir", "", "Output directory")
	cmd.Flags().StringVar(&ignores, "ignores", "", "Comma separated list of ignore paths")
	cmd.Flags().BoolVar(&summarize, "summarize", true, "Should include the summary file (required for future updates)")
	to = cmd.Flags().Int64("to", -1, "To version ID (optional)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
