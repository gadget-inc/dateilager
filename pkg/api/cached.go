package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/pkg/client"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Cached struct {
	pb.UnimplementedCachedServer

	Env         environment.Env
	Client      *client.Client
	StagingPath string

	// the current directory holding a fully formed downloaded cache
	currentDir string
	// the current version of the cache on disk at currentDir
	currentVersion int64
}

func (c *Cached) PopulateDiskCache(ctx context.Context, req *pb.PopulateDiskCacheRequest) (*pb.PopulateDiskCacheResponse, error) {
	if c.Env != environment.Dev && c.Env != environment.Test {
		return nil, status.Errorf(codes.Unimplemented, "Cached populateDiskCache only implemented in dev and test environments")
	}

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	destination := req.Path

	version, err := c.WriteCache(destination)
	if err != nil {
		return nil, err
	}

	return &pb.PopulateDiskCacheResponse{Version: version}, nil
}

// check if the destination exists, and if so, if its writable
// hardlink the golden copy into this downstream's destination, creating it if need be
func (c *Cached) WriteCache(destination string) (int64, error) {
	if c.currentDir == "" {
		return -1, errors.New("no cache prepared, currentDir is nil")
	}

	stat, err := os.Stat(destination)
	if !os.IsNotExist(err) {
		if err != nil {
			return -1, fmt.Errorf("failed to stat cache destination %s: %v", destination, err)
		}

		if !stat.IsDir() {
			return -1, fmt.Errorf("failed to open cache destination %s for writing -- it is already a file", destination)
		}

		if unix.Access(destination, unix.W_OK) != nil {
			return -1, fmt.Errorf("failed to open cache destination %s for writing -- write permission denied", destination)
		}
	}

	err = files.HardlinkDir(c.currentDir, destination)
	if err != nil {
		return -1, fmt.Errorf("failed to hardlink cache to destination %s: %v", destination, err)
	}
	return c.currentVersion, nil
}

// Fetch the cache into a spot in the staging dir
func (c *Cached) Prepare(ctx context.Context) error {
	start := time.Now()
	folderName, err := randomString()
	if err != nil {
		return err
	}
	newDir := path.Join(c.StagingPath, folderName)
	version, count, err := c.Client.GetCache(ctx, newDir)
	if err != nil {
		return err
	}

	c.currentDir = newDir
	c.currentVersion = version

	logger.Info(ctx, "downloaded golden copy", key.Directory.Field(newDir), key.DurationMS.Field(time.Since(start)), key.Version.Field(version), key.Count.Field(int64(count)))
	return nil
}

func randomString() (string, error) {
	// Generate a secure random string for the temporary directory name
	randBytes := make([]byte, 10) // Adjust the size of the byte slice as needed
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(randBytes), nil
}
