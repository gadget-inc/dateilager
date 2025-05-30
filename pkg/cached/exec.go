package cached

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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
	// Serialize LVM operations to prevent metadata corruption
	lvmLock sync.Mutex
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

// ExecContext executes a command with context and returns combined output
func (e *executor) ExecContext(ctx context.Context, command string, args ...string) error {
	cmd := e.CommandContext(ctx, command, args...)
	bs, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command %s %s: %w: %s", command, strings.Join(args, " "), err, string(bs))
	}
	return nil
}

// ExecLVM executes LVM commands with proper serialization
func (e *executor) ExecLVM(ctx context.Context, command string, args ...string) error {
	e.lvmLock.Lock()
	defer e.lvmLock.Unlock()

	return e.ExecContext(ctx, command, args...)
}

// UdevSettle triggers udev events and waits for device to appear
func (e *executor) UdevSettle(ctx context.Context, devPath string) error {
	// Trigger udev events for the device
	if err := e.ExecContext(ctx, "udevadm", "trigger", "--action=add", devPath); err != nil {
		logger.Warn(ctx, "udevadm trigger failed", zap.String("device", devPath), zap.Error(err))
		// Continue anyway, the device might still be available
	}

	// Wait for udev to settle with timeout
	settleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := e.ExecContext(settleCtx, "udevadm", "settle", "--exit-if-exists="+devPath); err != nil {
		logger.Warn(ctx, "udevadm settle failed", zap.String("device", devPath), zap.Error(err))
		// Fallback to polling
		return e.WaitForDevice(ctx, devPath, 5*time.Second)
	}

	return nil
}

// WaitForDevice polls for device availability with timeout
func (e *executor) WaitForDevice(ctx context.Context, devicePath string, timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(devicePath); err == nil {
				logger.Debug(ctx, "device is available", zap.String("device", devicePath))
				return nil
			}
		case <-timeoutCtx.Done():
			return fmt.Errorf("device node did not appear in time: %s (timeout: %v)", devicePath, timeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// CheckDeviceExists checks if a device path exists without waiting
func (e *executor) CheckDeviceExists(devicePath string) bool {
	_, err := os.Stat(devicePath)
	return err == nil
}
