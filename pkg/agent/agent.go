package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"go.uber.org/zap"
)

type Agent struct {
	varlibDir string
	cacheDir  string
	port      int
}

func NewAgent(cacheDir, varlibDir string, port int) Agent {
	return Agent{varlibDir, cacheDir, port}
}

func (a *Agent) Server(ctx context.Context) *http.Server {
	http.HandleFunc("GET /healthz", a.healthCheck)
	http.HandleFunc("POST /link_cache", a.linkCache)

	server := &http.Server{
		Addr:        ":" + strconv.Itoa(a.port),
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	return server
}

type healthStatus struct {
	Status string `json:"status"`
}

func (a *Agent) healthCheck(resp http.ResponseWriter, req *http.Request) {
	err := json.NewEncoder(resp).Encode(healthStatus{Status: "OK"})
	if err != nil {
		httpErr(req.Context(), resp, err, "failed to encode status")
		return
	}
}

type linkRequest struct {
	Uid    string `json:"uid"`
	Volume string `json:"volume"`
}

func (a *Agent) linkCache(resp http.ResponseWriter, req *http.Request) {
	linkReq := linkRequest{}
	err := json.NewDecoder(req.Body).Decode(&linkReq)
	if err != nil {
		httpReqErr(req.Context(), resp, err, "failed to decode link request")
		return
	}

	volumePath := filepath.Join(a.varlibDir, "kubelet/pods", linkReq.Uid, "volumes/kubernetes.io~empty-dir", linkReq.Volume)

	logger.Info(req.Context(), "linking cache directory", key.Directory.Field(filepath.Join(volumePath, "dl_cache")), zap.String("cacheDir", a.cacheDir))

	err = files.HardlinkDir(a.cacheDir, filepath.Join(volumePath, "dl_cache"))
	if err != nil {
		httpErr(req.Context(), resp, err, "failed to link cache director")
		return
	}

	resp.WriteHeader(http.StatusCreated)
}

func httpReqErr(ctx context.Context, resp http.ResponseWriter, err error, message string) {
	logger.Warn(ctx, message, zap.Error(err))
	http.Error(resp, err.Error(), http.StatusBadRequest)
}

func httpErr(ctx context.Context, resp http.ResponseWriter, err error, message string) {
	logger.Error(ctx, message, zap.Error(err))
	http.Error(resp, err.Error(), http.StatusInternalServerError)
}
