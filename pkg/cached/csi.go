package cached

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"syscall"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/lvm"
	"github.com/gadget-inc/dateilager/pkg/version"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
)

// GetPluginInfo returns metadata of the plugin
func (c *Cached) GetPluginInfo(ctx context.Context, _ *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{Name: DRIVER_NAME + c.NameSuffix, VendorVersion: version.Version}, nil
}

// GetPluginCapabilities returns available capabilities of the plugin
func (c *Cached) GetPluginCapabilities(ctx context.Context, _ *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{Capabilities: []*csi.PluginCapability{}}, nil
}

// Probe returns the health and readiness of the plugin
func (c *Cached) Probe(ctx context.Context, _ *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	ready := c.prepared.Load()
	if !ready {
		logger.Warn(ctx, "csi probe failed as daemon hasn't prepared cache yet")
	}
	return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: ready}}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
// This driver has no capabilities like expansion or staging, because we only use it for node local volumes.
func (c *Cached) NodeGetCapabilities(ctx context.Context, _ *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{Capabilities: []*csi.NodeServiceCapability{}}, nil
}

// NodeGetInfo returns the supported capabilities of the node server.
// Usually, a CSI driver would return some interesting stuff about the node here for the controller to use to place volumes, but because we're only supporting node local volumes, we return something very basic
func (c *Cached) NodeGetInfo(ctx context.Context, _ *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId:            firstNonEmptry(os.Getenv("NODE_ID"), os.Getenv("NODE_NAME"), os.Getenv("K8S_NODE_NAME"), "dev"),
		MaxVolumesPerNode: 110,
	}, nil
}

// NodePublishVolume publishes a volume to a target path
func (c *Cached) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID must be provided")
	}

	targetPath := req.GetTargetPath() // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target Path must be provided")
	}

	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume Capability must be provided")
	}

	lv := c.VG + "/" + volumeID
	lvDevice := "/dev/" + lv

	ctx = logger.With(ctx, key.VolumeID.Field(volumeID), key.TargetPath.Field(targetPath), key.LV.Field(lv), key.Device.Field(lvDevice))
	trace.SpanFromContext(ctx).SetAttributes(key.VolumeID.Attribute(volumeID), key.TargetPath.Attribute(targetPath), key.LV.Attribute(lv), key.Device.Attribute(lvDevice))
	logger.Info(ctx, "publishing volume")

	if err := lvm.EnsureLV(ctx, lv, LVCreateThinSnapshotArgs(c.BaseLV, c.ThinpoolLV, volumeID)...); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "failed to check if target path %s is mounted: %v", targetPath, err)
	}

	if notMounted {
		if err := os.MkdirAll(targetPath, 0o775); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create target path %s: %v", targetPath, err)
		}

		logger.Info(ctx, "mounting logical volume")
		if err := mounter.Mount(lvDevice, targetPath, c.BaseLVFormat, MountOptions(c.BaseLVFormat)); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to mount logical volume %s to %s: %v", lvDevice, targetPath, err)
		}
	}

	if err := os.Chmod(targetPath, 0o775); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to change permissions of target path %s: %v", targetPath, err)
	}

	logger.Info(ctx, "mounted logical volume")
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unpublishes a volume from a target path
func (c *Cached) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}

	targetPath := req.GetTargetPath() // e.g. /var/lib/kubelet/pods/967704ca-30eb-4df5-b299-690f78c51b30/volumes/kubernetes.io~csi/a/mount
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	lvName := c.VG + "/" + volumeID
	lvDevice := "/dev/" + lvName

	ctx = logger.With(ctx, key.VolumeID.Field(volumeID), key.TargetPath.Field(targetPath), key.LV.Field(lvName), key.Device.Field(lvDevice))
	trace.SpanFromContext(ctx).SetAttributes(key.VolumeID.Attribute(volumeID), key.TargetPath.Attribute(targetPath), key.LV.Attribute(lvName), key.Device.Attribute(lvDevice))
	logger.Info(ctx, "unpublishing volume")

	notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, status.Errorf(codes.Internal, "failed to check if target path %s is mounted: %v", targetPath, err)
	}

	if !notMounted {
		logger.Info(ctx, "unmounting target path")
		if err := mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmount logical volume at %s: %v", targetPath, err)
		}
	}

	if err := lvm.RemoveLV(ctx, lvName); err != nil {
		logger.Warn(ctx, "failed to remove logical volume", zap.Error(err))
	}

	logger.Info(ctx, "removed logical volume")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetVolumeStats returns the volume capacity statistics available for the given volume
func (c *Cached) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume ID must be provided")
	}

	volumePath := req.VolumePath
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume Path must be provided")
	}

	usedBytes, err := getFolderSize(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve used size statistics for volume path %s: %v", volumePath, err)
	}

	var stat syscall.Statfs_t
	err = syscall.Statfs(volumePath, &stat)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve total size statistics for volume path %s: %v", volumePath, err)
	}

	// Calculate free space in bytes
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	if freeBytes > math.MaxInt64 {
		return nil, status.Errorf(codes.Internal, "total size statistics for volume path too big for int64: %d", freeBytes)
	}
	signedFreeBytes := int64(freeBytes)

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: signedFreeBytes,
				Total:     signedFreeBytes + usedBytes,
				Used:      usedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
		},
	}, nil
}
