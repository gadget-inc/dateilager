package cached

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gadget-inc/dateilager/internal/logger"
	"go.uber.org/zap"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

type executor struct {
	utilexec.Interface
}

var exec = &executor{
	Interface: utilexec.New(),
}

var mounter = &mount.SafeFormatAndMount{
	Interface: mount.New(""),
	Exec:      exec,
}

func (e *executor) CommandContext(ctx context.Context, command string, args ...string) utilexec.Cmd {
	if os.Getenv("RUN_WITH_SUDO") != "" {
		args = append([]string{command}, args...)
		command = "sudo"
	}

	logger.Debug(ctx, "executing command", zap.String("command", command), zap.Strings("args", args))
	return e.Interface.CommandContext(ctx, command, args...)
}

func (e *executor) Command(command string, args ...string) utilexec.Cmd {
	return e.Interface.CommandContext(context.TODO(), command, args...)
}

func (e *executor) Exec(command string, args ...string) error {
	cmd := e.CommandContext(context.TODO(), command, args...)
	bs, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command %s %s: %w: %s", command, strings.Join(args, " "), err, string(bs))
	}
	return nil
}
