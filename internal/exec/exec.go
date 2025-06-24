package exec

import (
	"context"
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	utilexec "k8s.io/utils/exec"
)

var executor = utilexec.New()

func Command(ctx context.Context, command string, args ...string) utilexec.Cmd {
	return executor.CommandContext(ctx, command, args...)
}

// Run executes a command
func Run(ctx context.Context, command string, args ...string) error {
	_, err := Output(ctx, command, args...)
	if err != nil {
		return err
	}
	return nil
}

// Output executes a command and returns the output
func Output(ctx context.Context, command string, args ...string) (string, error) {
	logger.Debug(ctx, "executing command", key.Command.Field(command), key.Args.Field(args))
	cmd := Command(ctx, command, args...)
	bs, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command %s %s: %w: %s", command, strings.Join(args, " "), err, string(bs))
	}
	return strings.TrimSpace(string(bs)), nil
}
