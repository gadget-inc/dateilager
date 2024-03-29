package cli

import (
	"encoding/json"
	"fmt"

	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

type GcResult struct {
	Count int64 `json:"count"`
}

func NewCmdGc() *cobra.Command {
	var (
		mode    string
		project int64
		keep    int64
		from    *int64
		sample  float32
	)

	cmd := &cobra.Command{
		Use: "gc",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c := client.FromContext(ctx)

			var count int64
			var err error

			switch mode {
			case "contents":
				if sample == -1 {
					return fmt.Errorf("--sample required for contents mode")
				}

				count, err = c.GcContents(ctx, sample)
				if err != nil {
					return fmt.Errorf("could not gc contents: %w", err)
				}
			case "project":
				if project == -1 {
					return fmt.Errorf("--project required for project mode")
				}
				if keep == -1 {
					return fmt.Errorf("--keep required for project mode")
				}
				if *from == -1 {
					from = nil
				}

				count, err = c.GcProject(ctx, project, keep, from)
				if err != nil {
					return fmt.Errorf("could not gc project %v: %w", project, err)
				}
			case "random-projects":
				if sample == -1 {
					return fmt.Errorf("--sample required for random-projects mode")
				}
				if keep == -1 {
					return fmt.Errorf("--keep required for project mode")
				}
				if *from == -1 {
					from = nil
				}

				count, err = c.GcRandomProjects(ctx, sample, keep, from)
				if err != nil {
					return fmt.Errorf("could not gc project %v: %w", project, err)
				}
			default:
				return fmt.Errorf("Invalid mode type: %s", mode)
			}

			encoded, err := json.Marshal(GcResult{count})
			if err != nil {
				return fmt.Errorf("could not marshal result: %w", err)
			}

			fmt.Println(string(encoded))

			return nil
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "contents", "GC Mode (contents | project | random-projects)")
	cmd.Flags().Int64Var(&project, "project", -1, "Project ID (used by project mode)")
	cmd.Flags().Int64Var(&keep, "keep", -1, "Amount of versions to keep (used by project and random-projects mode)")
	from = cmd.Flags().Int64("from", -1, "Delete as of this version (used by project and random-projects mode)")
	cmd.Flags().Float32Var(&sample, "sample", -1, "Percent of rows to sample (used by contents and random-projects mode)")

	_ = cmd.MarkFlagRequired("mode")

	return cmd
}
