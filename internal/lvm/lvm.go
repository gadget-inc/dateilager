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

func EnsurePV(ctx context.Context, pv string) error {
	ctx = logger.With(ctx, key.PV.Field(pv))

	err := exec.Run(ctx, "pvdisplay", pv)
	if err == nil {
		logger.Info(ctx, "physical volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find physical volume") {
		return fmt.Errorf("failed to check physical volume %s: %w", pv, err)
	}

	logger.Info(ctx, "creating physical volume")
	if err := exec.Run(ctx, "pvcreate", pv); err != nil && !strings.Contains(err.Error(), "signal: killed") {
		return fmt.Errorf("failed to create physical volume %s: %w", pv, err)
	}

	return nil
}

func RemovePV(ctx context.Context, pv string) error {
	ctx = logger.With(ctx, key.PV.Field(pv))

	err := exec.Run(ctx, "pvdisplay", pv)
	if err != nil && !strings.Contains(err.Error(), "Failed to find physical volume") && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check physical volume %s: %w", pv, err)
	}

	if err == nil {
		logger.Info(ctx, "removing physical volume")
		if err := exec.Run(ctx, "pvremove", pv); err != nil {
			return fmt.Errorf("failed to remove physical volume %s: %w", pv, err)
		}
	}

	return nil
}

func EnsureVG(ctx context.Context, vg string, pvs ...string) error {
	ctx = logger.With(ctx, key.VG.Field(vg), key.PVs.Field(pvs))

	err := exec.Run(ctx, "vgdisplay", vg)
	if err == nil {
		logger.Info(ctx, "volume group already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check volume group %s: %w", vg, err)
	}

	logger.Info(ctx, "creating volume group")
	if err := exec.Run(ctx, "vgcreate", append([]string{vg}, pvs...)...); err != nil {
		return fmt.Errorf("failed to create volume group %s: %w", vg, err)
	}

	return nil
}

func RemoveVG(ctx context.Context, vg string) error {
	ctx = logger.With(ctx, key.VG.Field(vg))

	err := exec.Run(ctx, "vgdisplay", vg)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check volume group %s: %w", vg, err)
	}

	if err == nil {
		logger.Info(ctx, "removing volume group")
		if err := exec.Run(ctx, "vgremove", "-y", vg); err != nil {
			return fmt.Errorf("failed to remove volume group %s: %w", vg, err)
		}
	}

	return nil
}

func EnsureLV(ctx context.Context, lv string, lvCreateArgs ...string) error {
	ctx = logger.With(ctx, key.LV.Field(lv))

	err := exec.Run(ctx, "lvdisplay", lv)
	if err == nil {
		logger.Info(ctx, "logical volume already exists")
		return nil
	}

	if !strings.Contains(err.Error(), "Failed to find logical volume") {
		return fmt.Errorf("failed to check if logical volume %s exists: %w", lv, err)
	}

	logger.Info(ctx, "creating logical volume")
	if err := exec.Run(ctx, "lvcreate", lvCreateArgs...); err != nil {
		return fmt.Errorf("failed to create logical volume %s: %w", lv, err)
	}

	// Wait for device to appear and settle udev
	if err := udevSettle(ctx, "/dev/"+lv); err != nil {
		// keep going, the device might still be available
		logger.Warn(ctx, "udev settle failed for logical volume", zap.Error(err))
	}

	return nil
}

func RemoveLV(ctx context.Context, lv string) error {
	ctx = logger.With(ctx, key.LV.Field(lv))

	err := exec.Run(ctx, "lvdisplay", lv)
	if err != nil && !strings.Contains(err.Error(), "Failed to find logical volume") && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check if logical volume %s exists: %w", lv, err)
	}

	if err == nil {
		logger.Info(ctx, "removing logical volume")
		if err := exec.Run(ctx, "lvremove", "-y", lv); err != nil {
			return fmt.Errorf("failed to remove logical volume %s: %w", lv, err)
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
func waitForDevice(ctx context.Context, device string, timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(device); err == nil {
				logger.Debug(ctx, "device is available")
				return nil
			}
		case <-timeoutCtx.Done():
			return fmt.Errorf("device node did not appear in time: %s (timeout: %v)", device, timeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
