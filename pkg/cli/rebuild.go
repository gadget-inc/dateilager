package cli

import (
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	fsdiff_pb "github.com/gadget-inc/fsdiff/pkg/pb"
	"github.com/spf13/cobra"
)

func NewCmdRebuild() *cobra.Command {
	var (
		project    int64
		to         *int64
		prefix     string
		dir        string
		ignores    string
		logUpdates bool
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

			version, diff, err := client.Rebuild(ctx, project, prefix, to, dir, ignoreList, "")
			if err != nil {
				return fmt.Errorf("could not rebuild project: %w", err)
			}

			var count uint32
			if diff != nil {
				count = uint32(len(diff.Updates))
			} else {
				count = 0
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

			if logUpdates {
				LogUpdates(diff)
			}

			fmt.Println(version)
			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Search prefix")
	cmd.Flags().StringVar(&dir, "dir", "", "Output directory")
	cmd.Flags().StringVar(&ignores, "ignores", "", "Comma separated list of ignore paths")
	cmd.Flags().BoolVar(&logUpdates, "log-updates", false, "Log all updated files to the console")
	to = cmd.Flags().Int64("to", -1, "To version ID (optional)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}

func LogUpdates(diff *fsdiff_pb.Diff) {
	for _, update := range diff.Updates {
		fmt.Printf("%v %v\n", update.Action, update.Path)
	}
}
