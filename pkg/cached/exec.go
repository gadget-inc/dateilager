package cached

import (
	"context"
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/logger"
	"go.uber.org/zap"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

type cmd struct {
	utilexec.Cmd
	command string
	args    []string
	ctx     context.Context
}

func (c *cmd) Start() error {
	logger.Debug(c.ctx, "executing command", zap.String("command", c.command), zap.Strings("args", c.args))
	return c.Cmd.Start()
}

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

func (e *executor) Command(command string, args ...string) utilexec.Cmd {
	return e.CommandContext(context.TODO(), command, args...)
}

func (e *executor) CommandContext(ctx context.Context, command string, args ...string) utilexec.Cmd {
	return &cmd{
		Cmd:     e.Interface.CommandContext(ctx, command, args...),
		command: command,
		args:    args,
		ctx:     ctx,
	}
}

func (e *executor) Exec(command string, args ...string) error {
	cmd := e.CommandContext(context.TODO(), command, args...)
	bs, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command %s %s: %w: %s", command, strings.Join(args, " "), err, string(bs))
	}
	return nil
}
