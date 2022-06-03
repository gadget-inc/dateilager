package cli

import (
	"fmt"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/spf13/cobra"
)

type newArgs struct {
	id       int64
	template int64
	patterns string
}

func NewCmdNew(b client.ClientBuilder) *cobra.Command {
	a := newArgs{}

	cmd := &cobra.Command{
		Use: "new",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client, err := b.Build(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			err = client.NewProject(ctx, a.id, &a.template, a.patterns)
			if err != nil {
				return fmt.Errorf("could not create new project: %w", err)
			}

			logger.Info(ctx, "created new project", key.Project.Field(a.id))
			return nil
		},
	}

	cmd.Flags().Int64Var(&a.id, "id", -1, "Project ID (required)")
	cmd.Flags().Int64Var(&a.template, "template", -1, "Template ID")
	cmd.Flags().StringVar(&a.patterns, "patterns", "", "Comma separated pack patterns")

	_ = cmd.MarkFlagRequired("id")

	return cmd
}
