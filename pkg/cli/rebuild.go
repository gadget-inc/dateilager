package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

func NewCmdRebuild() *cobra.Command {
	var (
		project        int64
		to             *int64
		prefix         string
		dir            string
		ignores        string
		summarize      bool
		cacheDir       string
		filePattern    string
		filePatternIff bool
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

			var pattern *files.FilePattern
			if filePattern != "" {
				var err error
				pattern, err = files.NewFilePattern(filePattern, filePatternIff)
				if err != nil {
					return fmt.Errorf("invalid file pattern: %w", err)
				}
			}

			result, err := client.Rebuild(ctx, project, prefix, to, dir, ignoreList, cacheDir, pattern, summarize)
			if err != nil {
				return fmt.Errorf("could not rebuild project: %w", err)
			}

			if result.Count > 0 {
				logger.Info(ctx, "wrote files",
					key.Project.Field(project),
					key.Directory.Field(dir),
					key.Version.Field(result.Version),
					key.DiffCount.Field(result.Count),
				)
			}

			encoded, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("could not marshal result: %w", err)
			}

			fmt.Println(string(encoded))
			return nil
		},
	}

	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (required)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Search prefix")
	cmd.Flags().StringVar(&dir, "dir", "", "Output directory")
	cmd.Flags().StringVar(&ignores, "ignores", "", "Comma separated list of ignore paths")
	cmd.Flags().BoolVar(&summarize, "summarize", true, "Should include the summary file (required for future updates)")
	cmd.Flags().StringVar(&cacheDir, "cachedir", "", "Path where the cache folder is mounted")
	cmd.Flags().StringVar(&filePattern, "filepattern", "", "A glob file pattern which drives the patternMatch output boolean")
	cmd.Flags().BoolVar(&filePatternIff, "iff", false, "Should the file pattern detection trigger if and only if those files have changed")
	to = cmd.Flags().Int64("to", -1, "To version ID (optional)")

	_ = cmd.MarkFlagRequired("project")

	return cmd
}
