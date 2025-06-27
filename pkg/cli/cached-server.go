package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/cached"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func NewCachedServerCommand() *cobra.Command {
	var (
		healthzPort           uint16
		driverNameSuffix      string
		stagingPath           string
		csiSocket             string
		cacheVersion          int64
		cacheUid              int
		cacheGid              int
		lvmThinpoolDeviceGlob string
		lvmBaseDevice         string
		lvmBaseDeviceFormat   string
	)

	cmd := &cobra.Command{
		Use:               "server",
		Short:             "DateiLager cache daemon server",
		DisableAutoGenTag: true,
		Version:           version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			cd := cached.New(client.FromContext(ctx), driverNameSuffix)
			cd.StagingPath = stagingPath
			cd.CacheUid = cacheUid
			cd.CacheGid = cacheGid
			cd.LVMThinpoolDeviceGlob = lvmThinpoolDeviceGlob
			cd.LVMBaseDevice = lvmBaseDevice
			cd.LVMBaseDeviceFormat = lvmBaseDeviceFormat

			cachedServer := cached.NewServer(ctx)
			cachedServer.RegisterCSI(cd)

			healthMux := http.NewServeMux()
			healthMux.HandleFunc("/healthz", healthzHandler)

			healthServer := &http.Server{
				Addr:        fmt.Sprintf(":%d", healthzPort),
				Handler:     healthMux,
				BaseContext: func(l net.Listener) context.Context { return ctx },
			}

			err := cd.Prepare(ctx, cacheVersion)
			if err != nil {
				return fmt.Errorf("failed to prepare cache daemon in %s: %w", stagingPath, err)
			}

			group, ctx := errgroup.WithContext(ctx)

			osSignals := make(chan os.Signal, 1)
			signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
			group.Go(func() error {
				<-osSignals
				cachedServer.Grpc.GracefulStop()
				return healthServer.Shutdown(ctx)
			})

			group.Go(func() error {
				logger.Info(ctx, "starting cached server", key.Socket.Field(csiSocket))
				return cachedServer.Serve(csiSocket)
			})

			group.Go(func() error {
				logger.Info(ctx, "starting health server", key.Port.Field(int(healthzPort)))
				if err := healthServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
					return err
				}
				return nil
			})

			return group.Wait()
		},
	}

	flags := cmd.PersistentFlags()

	flags.Uint16Var(&healthzPort, "healthz-port", 5053, "Healthz HTTP port")
	flags.StringVar(&csiSocket, "csi-socket", "", "path for running the Kubernetes CSI Driver interface")
	flags.StringVar(&driverNameSuffix, "driver-name-suffix", "", "suffix for the driver name")
	flags.StringVar(&stagingPath, "staging-path", "", "path for staging downloaded caches")
	flags.Int64Var(&cacheVersion, "cache-version", -1, "cache version to use")
	flags.IntVar(&cacheUid, "cache-uid", -1, "uid for cache files")
	flags.IntVar(&cacheGid, "cache-gid", -1, "gid for cache files")
	flags.StringVar(&lvmThinpoolDeviceGlob, "lvm-thinpool-device-glob", os.Getenv("DL_LVM_THINPOOL_DEVICE_GLOB"), "glob of lvm devices to use for thinpool")
	flags.StringVar(&lvmBaseDevice, "lvm-base-device", os.Getenv("DL_LVM_BASE_DEVICE"), "lvm base device to use for base volume")
	flags.StringVar(&lvmBaseDeviceFormat, "lvm-base-device-format", firstNonEmpty(os.Getenv("DL_LVM_BASE_DEVICE_FORMAT"), "ext4"), "lvm base device format to use for base volume")

	_ = cmd.MarkPersistentFlagRequired("csi-socket")
	_ = cmd.MarkPersistentFlagRequired("staging-path")

	return cmd
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type response struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	resp := &response{}

	if ctx.Err() == nil {
		w.WriteHeader(http.StatusOK)
		resp.Status = "healthy"
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		resp.Status = "error"
		resp.Error = ctx.Err().Error()
	}

	data, err := json.MarshalIndent(&resp, "", "  ")
	if err != nil {
		logger.Error(ctx, "failed to marshal healthz response", zap.Error(err))
	}
	_, err = w.Write(data)
	if err != nil {
		logger.Error(ctx, "failed to write healthz response", zap.Error(err))
	}
}
