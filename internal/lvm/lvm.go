package lvm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/exec"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"go.uber.org/zap"
)

func EnsurePV(ctx context.Context, device string) error {
	ctx = logger.With(ctx, key.Device.Field(device))
	logger.Debug(ctx, "checking physical volume")

	err := exec.Run(ctx, "pvdisplay", device)
	if err == nil {
		logger.Info(ctx, "physical volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find physical volume") {
		return fmt.Errorf("failed to check lvm physical volume %s: %w", device, err)
	}

	logger.Info(ctx, "creating physical volume")
	if err := exec.Run(ctx, "pvcreate", device); err != nil && !strings.Contains(err.Error(), "signal: killed") {
		return fmt.Errorf("failed to create lvm physical volume %s: %w", device, err)
	}

	logger.Info(ctx, "physical volume created successfully")
	return nil
}

func RemovePV(ctx context.Context, device string) error {
	ctx = logger.With(ctx, key.Device.Field(device))
	logger.Debug(ctx, "checking physical volume for removal")

	err := exec.Run(ctx, "pvdisplay", device)
	if err != nil && !strings.Contains(err.Error(), "Failed to find physical volume") {
		return fmt.Errorf("failed to check lvm physical volume %s: %w", device, err)
	}

	if err == nil {
		logger.Info(ctx, "removing physical volume")
		if err := exec.Run(ctx, "pvremove", device); err != nil {
			return fmt.Errorf("failed to remove physical volume %s: %w", device, err)
		}
	}

	return nil
}

func EnsureVG(ctx context.Context, vgName string, devices ...string) error {
	ctx = logger.With(ctx, key.VG.Field(vgName), key.Device.Field(strings.Join(devices, ",")))
	logger.Debug(ctx, "checking volume group")

	err := exec.Run(ctx, "vgdisplay", vgName)
	if err == nil {
		logger.Debug(ctx, "volume group already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", vgName, err)
	}

	logger.Info(ctx, "creating volume group")
	if err := exec.Run(ctx, "vgcreate", append([]string{vgName}, devices...)...); err != nil {
		return fmt.Errorf("failed to create lvm volume group %s: %w", vgName, err)
	}

	return nil
}

func RemoveVG(ctx context.Context, vgName string) error {
	ctx = logger.With(ctx, key.VG.Field(vgName))
	logger.Debug(ctx, "checking volume group for removal")

	err := exec.Run(ctx, "vgdisplay", vgName)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check lvm volume group %s: %w", vgName, err)
	}

	if err == nil {
		logger.Info(ctx, "removing volume group", key.VG.Field(vgName))
		if err := exec.Run(ctx, "vgremove", "-y", vgName); err != nil {
			return fmt.Errorf("failed to remove lvm volume group %s: %w", vgName, err)
		}
	}

	return nil
}

func EnsureLV(ctx context.Context, lvName string, lvCreateArgs ...string) error {
	ctx = logger.With(ctx, key.LV.Field(lvName))
	logger.Debug(ctx, "checking logical volume")

	err := exec.Run(ctx, "lvdisplay", lvName)
	if err == nil {
		logger.Info(ctx, "logical volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check if logical volume %s exists: %w", lvName, err)
	}

	logger.Info(ctx, "creating logical volume")
	if err := exec.Run(ctx, "lvcreate", lvCreateArgs...); err != nil {
		return fmt.Errorf("failed to create logical volume %s: %w", lvName, err)
	}

	// Wait for device to appear and settle udev
	if err := udevSettle(ctx, "/dev/"+lvName); err != nil {
		// keep going, the device might still be available
		logger.Warn(ctx, "udev settle failed for logical volume", zap.Error(err))
	}

	return nil
}

func RemoveLV(ctx context.Context, lvName string) error {
	ctx = logger.With(ctx, key.LV.Field(lvName))
	logger.Debug(ctx, "checking logical volume for removal")

	err := exec.Run(ctx, "lvdisplay", lvName)
	if err != nil && !strings.Contains(err.Error(), "Failed to find logical volume") && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check if logical volume %s exists: %w", lvName, err)
	}

	if err == nil {
		logger.Info(ctx, "removing logical volume", key.LV.Field(lvName))
		if err := exec.Run(ctx, "lvremove", "-y", lvName); err != nil {
			return fmt.Errorf("failed to remove logical volume %s: %w", lvName, err)
		}
	}

	return nil
}

// udevSettle triggers udev events and waits for device to appear
func udevSettle(ctx context.Context, device string) error {
	// Trigger udev events for the device
	if err := exec.Run(ctx, "udevadm", "trigger", "--action=add", device); err != nil {
		logger.Warn(ctx, "udevadm trigger failed", zap.Error(err))
		// Continue anyway, the device might still be available
	}

	// Wait for udev to settle with timeout
	settleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := exec.Run(settleCtx, "udevadm", "settle", "--exit-if-exists="+device); err != nil {
		logger.Warn(ctx, "udev settle failed", zap.Error(err))
		// Fallback to polling
		return waitForDevice(ctx, device, 5*time.Second)
	}

	return nil
}

// waitForDevice polls for device availability with timeout
func waitForDevice(ctx context.Context, devicePath string, timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(devicePath); err == nil {
				logger.Debug(ctx, "device is available")
				return nil
			}
		case <-timeoutCtx.Done():
			return fmt.Errorf("device node did not appear in time: %s (timeout: %v)", devicePath, timeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
