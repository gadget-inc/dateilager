package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	fsdiff_pb "github.com/gadget-inc/fsdiff/pkg/pb"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/keepalive"
)

const (
	KB                       = 1024
	MB                       = KB * KB
	BUFFER_SIZE              = 64 * KB
	INITIAL_WINDOW_SIZE      = 1 * MB
	INITIAL_CONN_WINDOW_SIZE = 2 * INITIAL_WINDOW_SIZE
	MAX_MESSAGE_SIZE         = 300 * MB
)

type VersionRange struct {
	From *int64
	To   *int64
}

type Client struct {
	conn *grpc.ClientConn
	fs   pb.FsClient
}

type CachedClient struct {
	conn     *grpc.ClientConn
	cached   pb.CachedClient
	identity csi.IdentityClient
	node     csi.NodeClient
}

func NewClientConn(conn *grpc.ClientConn) *Client {
	return &Client{conn: conn, fs: pb.NewFsClient(conn)}
}

func NewCachedClientConn(conn *grpc.ClientConn) *CachedClient {
	return &CachedClient{
		conn:     conn,
		cached:   pb.NewCachedClient(conn),
		identity: csi.NewIdentityClient(conn),
		node:     csi.NewNodeClient(conn),
	}
}

type options struct {
	headlessHost string
	token        string
}

func WithToken(token string) func(*options) {
	return func(o *options) {
		o.token = token
	}
}

func WithheadlessHost(host string) func(*options) {
	return func(o *options) {
		o.headlessHost = host
	}
}

func grpcClientConn(ctx context.Context, host string, port uint16, opts ...func(*options)) (*grpc.ClientConn, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system cert pool: %w", err)
	}

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	sslVerification := os.Getenv("DL_SKIP_SSL_VERIFICATION")
	creds := credentials.NewTLS(&tls.Config{
		RootCAs:            pool,
		InsecureSkipVerify: sslVerification == "1",
		ServerName:         host,
	})

	if o.token == "" {
		o.token, err = getToken()
		if err != nil {
			return nil, err
		}
	}

	auth := oauth.TokenSource{
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: o.token,
		}),
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	server := fmt.Sprintf("%s:%d", host, port)
	if o.headlessHost != "" {
		server = fmt.Sprintf("%s:%d", o.headlessHost, port)
	}

	//nolint:staticcheck // Using DialContext until we're ready to migrate to NewClient
	return grpc.DialContext(connectCtx, server,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(auth),
		grpc.WithReadBufferSize(BUFFER_SIZE),
		grpc.WithWriteBufferSize(BUFFER_SIZE),
		grpc.WithInitialConnWindowSize(INITIAL_CONN_WINDOW_SIZE),
		grpc.WithInitialWindowSize(INITIAL_WINDOW_SIZE),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(MAX_MESSAGE_SIZE),
			grpc.MaxCallSendMsgSize(MAX_MESSAGE_SIZE),
		),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                5 * time.Second,
			Timeout:             1 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithDefaultServiceConfig(`
			{
				"loadBalancingConfig": [{ "round_robin": {} }],
				"methodConfig": [
					{
						"name": [
							{ "service": "pb.Fs", "method": "Get" },
							{ "service": "pb.Fs", "method": "GetUnary" },
							{ "service": "pb.Fs", "method": "GetCompress" }
						],
						"retryPolicy": {
							"maxAttempts": 3,
							"initialBackoff": "0.1s",
							"maxBackoff": "1s",
							"backoffMultiplier": 2,
							"retryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED"]
						}
					}
				]
			}
		`),
	)
}

func NewClient(ctx context.Context, host string, port uint16, opts ...func(*options)) (*Client, error) {
	ctx, span := telemetry.Start(ctx, "client.new", trace.WithAttributes(
		key.Server.Attribute(host),
	))
	defer span.End()

	conn, err := grpcClientConn(ctx, host, port, opts...)
	if err != nil {
		return nil, err
	}

	return NewClientConn(conn), nil
}

func (c *Client) Close() {
	// Give a chance for the upstream socket to finish writing it's response
	// https://github.com/grpc/grpc-go/issues/2869#issuecomment-503310136
	time.Sleep(1 * time.Millisecond)
	c.conn.Close()
}

func (c *Client) ListProjects(ctx context.Context) ([]*pb.Project, error) {
	ctx, span := telemetry.Start(ctx, "client.list-projects", trace.WithAttributes())
	defer span.End()

	resp, err := c.fs.ListProjects(ctx, &pb.ListProjectsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	return resp.Projects, nil
}

func (c *Client) NewProject(ctx context.Context, id int64, template *int64, packPatternsString *string) error {
	var packPatterns []string
	if packPatternsString != nil && *packPatternsString != "" {
		packPatterns = strings.Split(*packPatternsString, ",")
	}

	ctx, span := telemetry.Start(ctx, "client.new-project", trace.WithAttributes(
		key.Project.Attribute(id),
		key.Template.Attribute(template),
		key.PackPatterns.Attribute(packPatterns),
	))
	defer span.End()

	request := &pb.NewProjectRequest{
		Id:           id,
		Template:     template,
		PackPatterns: packPatterns,
	}

	_, err := c.fs.NewProject(ctx, request)
	if err != nil {
		return fmt.Errorf("create new project: %w", err)
	}

	return nil
}

func (c *Client) DeleteProject(ctx context.Context, project int64) error {
	ctx, span := telemetry.Start(ctx, "client.delete-project", trace.WithAttributes(
		key.Project.Attribute(project),
	))
	defer span.End()

	request := &pb.DeleteProjectRequest{
		Project: project,
	}

	_, err := c.fs.DeleteProject(ctx, request)
	if err != nil {
		return fmt.Errorf("delete project %v: %w", project, err)
	}

	return nil
}

func (c *Client) Get(ctx context.Context, project int64, prefix string, ignores []string, vrange VersionRange) ([]*pb.Object, error) {
	ctx, span := telemetry.Start(ctx, "client.get", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Prefix.Attribute(prefix),
		key.FromVersion.Attribute(vrange.From),
		key.ToVersion.Attribute(vrange.To),
		key.Ignores.Attribute(ignores),
	))
	defer span.End()

	var objects []*pb.Object

	query := &pb.ObjectQuery{
		Path:     prefix,
		IsPrefix: true,
		Ignores:  ignores,
	}

	request := &pb.GetRequest{
		Project:     project,
		FromVersion: vrange.From,
		ToVersion:   vrange.To,
		Queries:     []*pb.ObjectQuery{query},
	}

	stream, err := c.fs.Get(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("connect fs.Get: %w", err)
	}

	for {
		object, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive fs.Get: %w", err)
		}

		objects = append(objects, object.GetObject())
	}

	return objects, nil
}

type RebuildResult struct {
	Version     int64  `json:"version"`
	Count       uint32 `json:"count"`
	CachedCount uint32 `json:"cachedCount"`
	FileMatch   bool   `json:"fileMatch"`
}

func emptyResult(version int64) RebuildResult {
	return RebuildResult{
		Version:     version,
		Count:       0,
		CachedCount: 0,
		FileMatch:   false,
	}
}

type rebuildResultTracker struct {
	version     atomic.Int64
	count       atomic.Uint32
	cachedCount atomic.Uint32
	match       atomic.Bool
	matcher     *files.FileMatcher
}

func newResultTracker(matcher *files.FileMatcher) *rebuildResultTracker {
	tracker := rebuildResultTracker{
		version:     atomic.Int64{},
		count:       atomic.Uint32{},
		cachedCount: atomic.Uint32{},
		match:       atomic.Bool{},
		matcher:     matcher,
	}

	tracker.match.Store(true)

	return &tracker
}

func (t *rebuildResultTracker) checkVersion(version int64) error {
	old := t.version.Swap(version)
	if old != version && old != 0 {
		return fmt.Errorf("invalid response version from GetComrpess, expected %v got %v", old, version)
	}
	return nil
}

func (t *rebuildResultTracker) add(count uint32, cachedCount uint32, match bool) {
	t.count.Add(count)
	t.cachedCount.Add(cachedCount)

	if count > 0 && !match {
		t.match.Store(false)
	}
}

func (t *rebuildResultTracker) result() RebuildResult {
	count := t.count.Load()
	if count == 0 {
		t.match.Store(false)
	}

	return RebuildResult{
		Version:     t.version.Load(),
		Count:       count,
		CachedCount: t.cachedCount.Load(),
		FileMatch:   t.match.Load(),
	}
}

func (c *Client) Rebuild(ctx context.Context, project int64, prefix string, toVersion *int64, dir string, ignores []string, subpaths []string, cacheDir string, matcher *files.FileMatcher, summarize bool) (RebuildResult, error) {
	ctx, span := telemetry.Start(ctx, "client.rebuild", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Prefix.Attribute(prefix),
		key.ToVersion.Attribute(toVersion),
		key.Directory.Attribute(dir),
	))
	defer span.End()

	fromVersion, err := ReadVersionFile(dir)
	if err != nil {
		return emptyResult(fromVersion), err
	}
	if toVersion != nil && fromVersion == *toVersion {
		return emptyResult(fromVersion), nil
	}

	span.SetAttributes(key.FromVersion.Attribute(&fromVersion))

	query := &pb.ObjectQuery{
		Path:     prefix,
		IsPrefix: true,
		Ignores:  ignores,
		Subpaths: subpaths,
	}

	availableCacheVersions := ReadCacheVersionFile(cacheDir)

	request := &pb.GetCompressRequest{
		Project:                project,
		FromVersion:            &fromVersion,
		ToVersion:              toVersion,
		Queries:                []*pb.ObjectQuery{query},
		AvailableCacheVersions: availableCacheVersions,
	}

	stream, err := c.fs.GetCompress(ctx, request)
	if err != nil {
		return emptyResult(fromVersion), fmt.Errorf("connect fs.GetCompress: %w", err)
	}

	// Pull one response before booting workers
	// This is a short circuit for cases where there are no diffs to apply
	response, err := stream.Recv()
	if err == io.EOF {
		return emptyResult(fromVersion), nil
	}
	if err != nil {
		return emptyResult(fromVersion), fmt.Errorf("receive fs.GetCompress: %w", err)
	}

	err = ensureMetadataDir(dir)
	if err != nil {
		return emptyResult(fromVersion), err
	}

	tracker := newResultTracker(matcher)

	tarChan := make(chan *pb.GetCompressResponse, 32)
	group, ctx := errgroup.WithContext(ctx)
	ctx, cancel := context.WithCancel(ctx)

	group.Go(func() error {
		ctx, span := telemetry.Start(ctx, "object-receiver")
		defer span.End()
		defer close(tarChan)

		for {
			err := tracker.checkVersion(response.Version)
			if err != nil {
				return err
			}

			select {
			case <-ctx.Done():
				return nil
			case tarChan <- response:
				response, err = stream.Recv()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					cancel()
					return fmt.Errorf("receive fs.GetCompress: %w", err)
				}
			}
		}
	})

	workerCount := parallelWorkerCount()
	span.SetAttributes(key.WorkerCount.Attribute(workerCount))

	for i := 0; i < workerCount; i++ {
		// create the attribute here when `i` is different
		attr := key.Worker.Attribute(i)

		group.Go(func() error {
			ctx, span := telemetry.Start(ctx, "object-writer", trace.WithAttributes(attr))
			defer span.End()

			tarReader := db.NewTarReader()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case response, ok := <-tarChan:
					if !ok {
						return nil
					}

					tarReader.FromBytes(response.Bytes)

					count, cachedCount, match, err := files.WriteTar(dir, CacheObjectsDir(cacheDir), tarReader, response.PackPath, matcher)
					if err != nil {
						cancel()
						return err
					}

					tracker.add(count, cachedCount, match)
				}
			}
		})
	}

	err = group.Wait()
	if err != nil {
		return emptyResult(fromVersion), err
	}

	result := tracker.result()

	err = WriteVersionFile(dir, result.Version)
	if err != nil {
		return emptyResult(fromVersion), err
	}

	if summarize {
		_, err = DiffAndSummarize(ctx, dir)
		if err != nil {
			return emptyResult(fromVersion), err
		}
	}

	return result, nil
}

func (c *Client) Update(rootCtx context.Context, project int64, dir string, subpaths []string) (int64, uint32, error) {
	rootCtx, span := telemetry.Start(rootCtx, "client.update", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Directory.Attribute(dir),
	))
	defer span.End()

	fromVersion, err := ReadVersionFile(dir)
	if err != nil {
		return -1, 0, err
	}

	diff, err := DiffAndSummarize(rootCtx, dir)
	if err != nil {
		return -1, 0, err
	}

	if len(diff.Updates) == 0 {
		return fromVersion, 0, nil
	}

	if len(subpaths) > 0 {
		filteredUpdates := make([]*fsdiff_pb.Update, 0, len(diff.Updates))
		for _, update := range diff.Updates {
			for _, subpath := range subpaths {
				if strings.HasPrefix(update.Path, subpath) {
					filteredUpdates = append(filteredUpdates, update)
					break
				}
			}
		}
		diff.Updates = filteredUpdates
	}

	toVersion := int64(-1)

	updateChan := make(chan *fsdiff_pb.Update, len(diff.Updates))
	objectChan := make(chan *pb.Object, 32)

	group, ctx := errgroup.WithContext(rootCtx)
	ctx, cancel := context.WithCancel(ctx)

	workerCount := parallelWorkerCount()
	span.SetAttributes(key.WorkerCount.Attribute(workerCount))

	for i := 0; i < workerCount; i++ {
		// create the attribute here when `i` is different
		attr := key.Worker.Attribute(i)

		group.Go(func() error {
			ctx, span := telemetry.Start(ctx, "object-reader", trace.WithAttributes(attr))
			defer span.End()

			for {
				select {
				case <-ctx.Done():
					return nil
				case update, ok := <-updateChan:
					if !ok {
						return nil
					}

					var object *pb.Object

					if update.Action == fsdiff_pb.Update_REMOVE {
						object = &pb.Object{
							Path:    update.Path,
							Deleted: true,
						}
					} else {
						object, err = pb.ObjectFromFilePath(dir, update.Path)
						if err != nil {
							cancel()
							return fmt.Errorf("read file object: %w", err)
						}
					}

					select {
					case <-ctx.Done():
						return nil
					case objectChan <- object:
						continue
					}
				}
			}
		})
	}

	group.Go(func() error {
		ctx, span := telemetry.Start(ctx, "object-sender")
		defer span.End()

		stream, err := c.fs.Update(ctx)
		if err != nil {
			cancel()
			return fmt.Errorf("connect fs.Update: %w", err)
		}

		count := 0

		for {
			if count == len(diff.Updates) {
				close(objectChan)
			}

			select {
			case <-ctx.Done():
				return nil
			case object, ok := <-objectChan:
				if !ok {
					response, err := stream.CloseAndRecv()
					if err != nil {
						cancel()
						return fmt.Errorf("close and receive fs.Update: %w", err)
					}
					toVersion = response.Version
					return nil
				}

				count += 1

				err := stream.Send(&pb.UpdateRequest{
					Project: project,
					Object:  object,
				})
				if err != nil {
					cancel()
					return fmt.Errorf("send fs.Update, path %v, size %v, mode %v, deleted %v: %w", object.Path, object.Size, object.Mode, object.Deleted, err)
				}
			}
		}
	})

	for _, update := range diff.Updates {
		updateChan <- update
	}
	close(updateChan)

	err = group.Wait()
	if err != nil {
		return -1, 0, err
	}

	updateCount := uint32(len(diff.Updates))

	if (fromVersion + 1) == toVersion {
		err = WriteVersionFile(dir, toVersion)
		if err != nil {
			return -1, updateCount, err
		}
	} else {
		result, err := c.Rebuild(rootCtx, project, "", nil, dir, nil, nil, "", nil, true)
		if err != nil {
			return -1, updateCount, err
		}

		toVersion = result.Version
	}

	return toVersion, updateCount, nil
}

func (c *Client) Inspect(ctx context.Context, project int64) (*pb.InspectResponse, error) {
	ctx, span := telemetry.Start(ctx, "client.inspect", trace.WithAttributes(
		key.Project.Attribute(project),
	))
	defer span.End()

	inspect, err := c.fs.Inspect(ctx, &pb.InspectRequest{Project: project})
	if err != nil {
		return nil, fmt.Errorf("inspect project %v: %w", project, err)
	}

	return inspect, nil
}

func (c *Client) Snapshot(ctx context.Context) (string, error) {
	ctx, span := telemetry.Start(ctx, "client.snapshot")
	defer span.End()

	resp, err := c.fs.Snapshot(ctx, &pb.SnapshotRequest{})
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}

	var state []string
	for _, projectSnapshot := range resp.Projects {
		state = append(state, fmt.Sprintf("%v=%v", projectSnapshot.Id, projectSnapshot.Version))
	}

	return strings.Join(state, ","), nil
}

func (c *Client) Reset(ctx context.Context, state string) error {
	ctx, span := telemetry.Start(ctx, "client.reset", trace.WithAttributes(
		key.State.Attribute(state),
	))
	defer span.End()

	var projects []*pb.Project

	for _, projectSplit := range strings.Split(state, ",") {
		parts := strings.Split(projectSplit, "=")
		if len(parts) != 2 {
			return fmt.Errorf("invalid state chunk: %v", projectSplit)
		}

		project, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid state int %v: %w", parts[0], err)
		}

		version, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid state int %v: %w", parts[1], err)
		}

		projects = append(projects, &pb.Project{
			Id:      project,
			Version: version,
		})
	}

	_, err := c.fs.Reset(ctx, &pb.ResetRequest{
		Projects: projects,
	})
	if err != nil {
		return fmt.Errorf("reset to state: %w", err)
	}

	return nil
}

func obtainCacheLockFile(cacheRootDir string) (*os.File, error) {
	lockFile, err := os.OpenFile(filepath.Join(cacheRootDir, ".lock"), os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("unable to obtain cache lock file; another process may be using it: %w", err)
	}
	lockFile.Close()

	return lockFile, nil
}

func cleanupCacheLockFile(lockFile *os.File) {
	os.Remove(lockFile.Name())
}

func (c *Client) GetCache(ctx context.Context, cacheRootDir string, cacheVersion int64) (int64, uint32, error) {
	objectDir := CacheObjectsDir(cacheRootDir)
	err := os.MkdirAll(objectDir, 0o755)
	if err != nil {
		return -1, 0, fmt.Errorf("cannot create object folder: %w", err)
	}

	tmpObjectDir := CacheTmpDir(cacheRootDir)
	os.RemoveAll(tmpObjectDir)

	err = os.MkdirAll(tmpObjectDir, 0o755)
	if err != nil {
		return -1, 0, fmt.Errorf("cannot create tmp folder to unpack cached objects: %w", err)
	}
	defer os.RemoveAll(tmpObjectDir)

	lockFile, err := obtainCacheLockFile(cacheRootDir)
	if err != nil {
		return -1, 0, err
	}
	defer cleanupCacheLockFile(lockFile)

	ctx, span := telemetry.Start(ctx, "client.get_cache")
	defer span.End()

	request := &pb.GetCacheRequest{}
	if cacheVersion != -1 {
		request.Version = &cacheVersion
	}

	stream, err := c.fs.GetCache(ctx, request)
	if err != nil {
		return -1, 0, fmt.Errorf("fs.GetCache connect: %w", err)
	}

	// Pull one response before booting workers
	// This is a short circuit in case the cache doesn't exist
	response, err := stream.Recv()
	if err == io.EOF {
		return -1, 0, nil
	}
	if err != nil {
		return -1, 0, fmt.Errorf("fs.GetCache receive: %w", err)
	}
	if cacheVersion == -1 {
		cacheVersion = response.Version
	} else if cacheVersion != response.Version {
		return -1, 0, fmt.Errorf("fs.GetCache version mismatch: %d, %d", cacheVersion, response.Version)
	}

	// We cannot early exist here, even if `cacheVersion` is already available locally
	// since we've opened the GRPC stream we need to go ahead and read the whole thing
	// this should be split into 2 requests, one to get the latest version and another to download it.

	tarChan := make(chan *pb.GetCacheResponse, 16)
	group, ctx := errgroup.WithContext(ctx)
	ctx, cancel := context.WithCancel(ctx)

	group.Go(func() error {
		ctx, span := telemetry.Start(ctx, "cache-object-receiver")
		defer span.End()
		defer close(tarChan)

		for {
			select {
			case <-ctx.Done():
				return nil
			case tarChan <- response:
				response, err = stream.Recv()
				if err == io.EOF {
					return nil
				}
				if response.Version != cacheVersion {
					cancel()
					return fmt.Errorf("fs.GetCache version mismatch: %d, %d", response.Version, cacheVersion)
				}
				if err != nil {
					cancel()
					return fmt.Errorf("fs.GetCache receive: %w", err)
				}
			}
		}
	})

	workerCount := parallelWorkerCount()
	var writtenObjectCount atomic.Uint32

	span.SetAttributes(key.WorkerCount.Attribute(workerCount))

	for i := 0; i < workerCount; i++ {
		// create the attribute here when `i` is different
		attr := key.Worker.Attribute(i)

		group.Go(func() error {
			ctx, span := telemetry.Start(ctx, "cache-object-writer", trace.WithAttributes(attr))
			defer span.End()

			tarReader := db.NewTarReader()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case response, ok := <-tarChan:
					if !ok {
						return nil
					}

					tarReader.FromBytes(response.Bytes)
					hashHex := hex.EncodeToString(response.Hash)
					tempDest := filepath.Join(tmpObjectDir, hashHex)
					finalDest := filepath.Join(objectDir, hashHex)

					if fileExists(finalDest) {
						continue
					}

					if fileExists(tempDest) {
						err := os.RemoveAll(tempDest)
						if err != nil {
							cancel()
							return fmt.Errorf("temporary cache folder exists for %s and couldn't be removed: %w", tempDest, err)
						}
					}

					count, _, _, err := files.WriteTar(tempDest, CacheObjectsDir(cacheRootDir), tarReader, nil, nil)
					if err != nil {
						cancel()
						return err
					}

					err = os.Rename(tempDest, finalDest)
					if err != nil {
						cancel()
						return fmt.Errorf("couldn't rename temporary folder (%s) to final folder (%s): %w", tempDest, finalDest, err)
					}
					writtenObjectCount.Add(count)
				}
			}
		})
	}

	err = group.Wait()
	if err != nil {
		return -1, writtenObjectCount.Load(), err
	}

	versionFile, err := os.OpenFile(cacheVersionPath(cacheRootDir), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return -1, writtenObjectCount.Load(), fmt.Errorf("fs.GetCache cannot open cache versions file for writing: %w", err)
	}
	defer versionFile.Close()

	_, err = versionFile.WriteString(fmt.Sprintf("%d\n", cacheVersion))
	if err != nil {
		return -1, writtenObjectCount.Load(), fmt.Errorf("fs.GetCache failed to update the versions file: %w", err)
	}

	return cacheVersion, writtenObjectCount.Load(), nil
}

func (c *Client) GcProject(ctx context.Context, project int64, keep int64, from *int64) (int64, error) {
	ctx, span := telemetry.Start(ctx, "client.gc-project", trace.WithAttributes(
		key.Project.Attribute(project),
	))
	defer span.End()

	request := &pb.GcProjectRequest{
		Project:      project,
		KeepVersions: keep,
		FromVersion:  from,
	}

	response, err := c.fs.GcProject(ctx, request)
	if err != nil {
		return 0, fmt.Errorf("gc project %v: %w", project, err)
	}

	return response.Count, nil
}

func (c *Client) GcRandomProjects(ctx context.Context, sample float32, keep int64, from *int64) (int64, error) {
	ctx, span := telemetry.Start(ctx, "client.gc-random-project", trace.WithAttributes(
		key.SampleRate.Attribute(sample),
	))
	defer span.End()

	request := &pb.GcRandomProjectsRequest{
		Sample:       sample,
		KeepVersions: keep,
		FromVersion:  from,
	}

	response, err := c.fs.GcRandomProjects(ctx, request)
	if err != nil {
		return 0, fmt.Errorf("gc random projects %v: %w", sample, err)
	}

	return response.Count, nil
}

func (c *Client) GcContents(ctx context.Context, sample float32) (int64, error) {
	ctx, span := telemetry.Start(ctx, "client.gc-contents", trace.WithAttributes(
		key.SampleRate.Attribute(sample),
	))
	defer span.End()

	request := &pb.GcContentsRequest{
		Sample: sample,
	}

	response, err := c.fs.GcContents(ctx, request)
	if err != nil {
		return 0, fmt.Errorf("gc contents %v: %w", sample, err)
	}

	return response.Count, nil
}

func (c *Client) CloneToProject(ctx context.Context, source int64, target int64, version int64) (*int64, error) {
	ctx, span := telemetry.Start(ctx, "client.clone-to-project", trace.WithAttributes(
		key.Project.Attribute(source),
		key.ToVersion.Attribute(&version),
		key.CloneToProject.Attribute(target),
	))
	defer span.End()

	response, err := c.fs.CloneToProject(ctx, &pb.CloneToProjectRequest{
		Source:  source,
		Target:  target,
		Version: version,
	})
	if err != nil {
		return nil, fmt.Errorf("clone to project: %w", err)
	}

	return &response.LatestVersion, nil
}

func NewCachedClient(ctx context.Context, host string, port uint16, opts ...func(*options)) (*CachedClient, error) {
	ctx, span := telemetry.Start(ctx, "cached-client.new", trace.WithAttributes(
		key.Server.Attribute(host),
	))
	defer span.End()

	conn, err := grpcClientConn(ctx, host, port, opts...)
	if err != nil {
		return nil, err
	}

	return NewCachedClientConn(conn), nil
}

func NewCachedUnixClient(ctx context.Context, socket string) (*CachedClient, error) {
	ctx, span := telemetry.Start(ctx, "cached-unix-client.new", trace.WithAttributes(
		key.Server.Attribute(socket),
	))
	defer span.End()

	bc := backoff.DefaultConfig
	bc.MaxDelay = time.Second
	//nolint:staticcheck, nolintlint // Using WithBlock for now
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: bc}),
		grpc.WithBlock(),
		grpc.WithIdleTimeout(time.Duration(0)),
		grpc.WithContextDialer(func(ctx context.Context, path string) (net.Conn, error) {
			var timeout time.Duration
			deadline, ok := ctx.Deadline()
			if ok {
				timeout = time.Until(deadline)
			}
			return net.DialTimeout("unix", path[len("unix://"):], timeout)
		}),
	}

	//nolint:staticcheck, nolintlint // Using DialContext until we're ready to migrate to NewClient
	conn, err := grpc.DialContext(ctx, socket, dialOptions...)
	if err != nil {
		return nil, err
	}

	return NewCachedClientConn(conn), nil
}

func (c *CachedClient) Close() {
	// Give a chance for the upstream socket to finish writing it's response
	// https://github.com/grpc/grpc-go/issues/2869#issuecomment-503310136
	time.Sleep(1 * time.Millisecond)
	c.conn.Close()
}

func (c *CachedClient) PopulateDiskCache(ctx context.Context, destination string) (int64, error) {
	ctx, span := telemetry.Start(ctx, "client.populate-disk-cache", trace.WithAttributes(
		key.CachePath.Attribute(destination),
	))
	defer span.End()

	request := &pb.PopulateDiskCacheRequest{
		Path: destination,
	}

	response, err := c.cached.PopulateDiskCache(ctx, request)
	if err != nil {
		return 0, fmt.Errorf("populate disk cache for %s: %w", destination, err)
	}

	return response.Version, nil
}

func (c *CachedClient) Probe(ctx context.Context) (bool, error) {
	request := &csi.ProbeRequest{}

	response, err := c.identity.Probe(ctx, request)
	if err != nil {
		return false, fmt.Errorf("failed to probe server: %w", err)
	}

	return response.Ready.Value, nil
}

func (c *CachedClient) NodeGetVolumeStats(ctx context.Context) ([]*csi.VolumeUsage, error) {
	request := &csi.NodeGetVolumeStatsRequest{}

	response, err := c.node.NodeGetVolumeStats(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to probe server: %w", err)
	}

	return response.Usage, nil
}

func parallelWorkerCount() int {
	envCount := os.Getenv("DL_WRITE_WORKERS")
	if envCount != "" {
		count, err := strconv.Atoi(envCount)
		if err == nil {
			return count
		}
	}

	halfNumCPU := runtime.NumCPU() / 2
	if halfNumCPU < 1 {
		halfNumCPU = 1
	}
	return halfNumCPU
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
