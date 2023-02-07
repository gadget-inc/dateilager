package cli

import (
	"fmt"
	"strings"

	doublestar "github.com/bmatcuk/doublestar/v4"
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
		checkGlobs []string
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
					key.DiffCount.Field(DiffUpdateCount(diff)),
				)
			}

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
	cmd.Flags().StringVar(&prefix, "prefix", "", "Search prefix")
	cmd.Flags().StringVar(&dir, "dir", "", "Output directory")
	cmd.Flags().StringVar(&ignores, "ignores", "", "Comma separated list of ignore paths")
	cmd.Flags().BoolVar(&logUpdates, "log-updates", false, "Log all updated files to the console")
	cmd.Flags().StringSliceVar(&checkGlobs, "check-glob", []string{}, "Report if any files matching the given globs were changed by this rebuild operation")
	to = cmd.Flags().Int64("to", -1, "To version ID (optional)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}

func LogUpdates(diff *fsdiff_pb.Diff) {
	for _, update := range diff.Updates {
		fmt.Printf("%v %v\n", update.Action, update.Path)
	}
}

func anyMatchedGlobs(globs []string, diff *fsdiff_pb.Diff) (bool, error) {
	for _, glob := range globs {
		for _, update := range diff.Updates {
			match, err := doublestar.Match(glob, update.Path)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		}
	}

	return false, nil
}

func LogIfGlobsMatched(globs []string, diff *fsdiff_pb.Diff) error {
	if len(globs) > 0 {
		matched, err := anyMatchedGlobs(globs, diff)
		if err != nil {
			return err
		}
		if matched {
			fmt.Printf("GLOB-MATCH: files matching globs were matched during rebuild\n")
		}
	}
	return nil
}

func DiffUpdateCount(diff *fsdiff_pb.Diff) uint32 {
	if diff != nil {
		return uint32(len(diff.Updates))
	} else {
		return 0
	}
}
