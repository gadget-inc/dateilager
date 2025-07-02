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
		baseLVFormat        string
		basePV              string
		cacheGid            int
		cacheUid            int
		cacheVersion        int64
		csiSocket           string
		healthzPort         uint16
		nameSuffix          string
		thinpoolCacheLVSize string
		thinpoolPVGlobs     string
	)

	cmd := &cobra.Command{
		Use:               "server",
		Short:             "DateiLager cache daemon server",
		DisableAutoGenTag: true,
		Version:           version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			cd := cached.New(client.FromContext(ctx), nameSuffix)
			cd.BaseLVFormat = baseLVFormat
			cd.BasePV = basePV
			cd.CacheGid = cacheGid
			cd.CacheUid = cacheUid
			cd.ThinpoolCacheLVSize = thinpoolCacheLVSize
			cd.ThinpoolPVGlobs = thinpoolPVGlobs

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
				return fmt.Errorf("failed to prepare: %w", err)
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
	flags.Int64Var(&cacheVersion, "cache-version", -1, "cache version to prepare")
	flags.IntVar(&cacheGid, "cache-gid", cached.NO_CHANGE_USER, "gid for cache files")
	flags.IntVar(&cacheUid, "cache-uid", cached.NO_CHANGE_USER, "uid for cache files")
	flags.StringVar(&baseLVFormat, "base-lv-format", firstNonEmpty(os.Getenv("DL_BASE_LV_FORMAT"), cached.EXT4), "filesystem format to use for the base LV")
	flags.StringVar(&basePV, "base-pv", os.Getenv("DL_BASE_PV"), "PV to use for the base LV")
	flags.StringVar(&csiSocket, "csi-socket", firstNonEmpty(os.Getenv("DL_CSI_SOCKET"), "unix:///csi/csi.sock"), "path for running the Kubernetes CSI Driver interface")
	flags.StringVar(&nameSuffix, "name-suffix", "", "hyphenated suffix to use for naming the driver and its components")
	flags.StringVar(&thinpoolCacheLVSize, "thinpool-cache-lv-size", os.Getenv("DL_THINPOOL_CACHE_LV_SIZE"), "size of the thinpool cache LV in KiB")
	flags.StringVar(&thinpoolPVGlobs, "thinpool-pv-globs", os.Getenv("DL_THINPOOL_PV_GLOBS"), "comma-separated globs of PVs to use for the thinpool")
	flags.Uint16Var(&healthzPort, "healthz-port", 5053, "healthz HTTP port")

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
