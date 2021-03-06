package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

const (
	MB = 1000 * 1000
)

type VersionRange struct {
	From *int64
	To   *int64
}

type Client struct {
	conn *grpc.ClientConn
	fs   pb.FsClient
}

func NewClientConn(conn *grpc.ClientConn) *Client {
	return &Client{conn: conn, fs: pb.NewFsClient(conn)}
}

type options struct {
	token string
}

func WithToken(token string) func(*options) {
	return func(o *options) {
		o.token = token
	}
}

func NewClient(ctx context.Context, server string, opts ...func(*options)) (*Client, error) {
	ctx, span := telemetry.Start(ctx, "client.new", trace.WithAttributes(
		key.Server.Attribute(server),
	))
	defer span.End()

	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system cert pool: %w", err)
	}

	sslVerification := os.Getenv("DL_SKIP_SSL_VERIFICATION")
	creds := credentials.NewTLS(&tls.Config{RootCAs: pool, InsecureSkipVerify: sslVerification == "1"})

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	if o.token == "" {
		o.token, err = getToken()
		if err != nil {
			return nil, err
		}
	}

	auth := oauth.NewOauthAccess(&oauth2.Token{
		AccessToken: o.token,
	})

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(connectCtx, server,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(auth),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*MB), grpc.MaxCallSendMsgSize(100*MB)),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
	)
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

func (c *Client) NewProject(ctx context.Context, id int64, template *int64, packPatterns string) error {
	splitPackPatterns := strings.Split(packPatterns, ",")
	ctx, span := telemetry.Start(ctx, "client.new-project", trace.WithAttributes(
		key.Project.Attribute(id),
		key.Template.Attribute(template),
		key.PackPatterns.Attribute(splitPackPatterns),
	))
	defer span.End()

	request := &pb.NewProjectRequest{
		Id:           id,
		Template:     template,
		PackPatterns: splitPackPatterns,
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
		return fmt.Errorf("delete project: %w", err)
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
		Path:        prefix,
		IsPrefix:    true,
		WithContent: true,
		Ignores:     ignores,
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

func (c *Client) Rebuild(ctx context.Context, project int64, prefix string, toVersion *int64, dir string) (int64, uint32, error) {
	ctx, span := telemetry.Start(ctx, "client.rebuild", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Prefix.Attribute(prefix),
		key.ToVersion.Attribute(toVersion),
		key.Directory.Attribute(dir),
	))
	defer span.End()

	var diffCount uint32

	fromVersion, err := ReadVersionFile(dir)
	if err != nil {
		return fromVersion, diffCount, err
	}
	if toVersion != nil && fromVersion == *toVersion {
		return *toVersion, diffCount, nil
	}

	query := &pb.ObjectQuery{
		Path:        prefix,
		IsPrefix:    true,
		WithContent: true,
	}

	request := &pb.GetCompressRequest{
		Project:     project,
		FromVersion: &fromVersion,
		ToVersion:   toVersion,
		Queries:     []*pb.ObjectQuery{query},
	}

	stream, err := c.fs.GetCompress(ctx, request)
	if err != nil {
		return fromVersion, diffCount, fmt.Errorf("connect fs.GetCompress: %w", err)
	}

	// Pull one response before booting workers
	// This is a short circuit for cases where there are no diffs to apply
	response, err := stream.Recv()
	if err == io.EOF {
		return fromVersion, diffCount, nil
	}
	if err != nil {
		return fromVersion, diffCount, fmt.Errorf("receive fs.GetCompress: %w", err)
	}

	tarChan := make(chan *pb.GetCompressResponse, 16)
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		ctx, span := telemetry.Start(ctx, "object-receiver")
		defer span.End()
		defer close(tarChan)

		for {
			if toVersion != nil && *toVersion != response.Version {
				return fmt.Errorf("invalid response version from GetComrpess, expected %v got %v", *toVersion, response.Version)
			}
			toVersion = &response.Version

			select {
			case <-ctx.Done():
				return ctx.Err()
			case tarChan <- response:
				response, err = stream.Recv()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return fmt.Errorf("receive fs.GetCompress: %w", err)
				}
			}
		}
	})

	for i := 0; i < parallelWorkerCount(); i++ {
		// create the attribute here when `i` is different
		attr := key.Worker.Attribute(i)

		group.Go(func() error {
			ctx, span := telemetry.Start(ctx, "object-writer", trace.WithAttributes(attr))
			defer span.End()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case response, ok := <-tarChan:
					if !ok {
						return nil
					}

					tarReader := db.NewTarReader(response.Bytes)

					count, err := files.WriteTar(dir, tarReader, response.PackPath)
					if err != nil {
						return err
					}

					atomic.AddUint32(&diffCount, count)
				}
			}
		})
	}

	err = group.Wait()
	if err != nil {
		return -1, diffCount, err
	}

	err = WriteVersionFile(dir, *toVersion)
	if err != nil {
		return -1, diffCount, err
	}

	_, err = DiffAndSummarize(dir)
	if err != nil {
		return -1, diffCount, err
	}

	return *toVersion, diffCount, nil
}

func (c *Client) Update(rootCtx context.Context, project int64, dir string) (int64, uint32, error) {
	rootCtx, span := telemetry.Start(rootCtx, "client.update", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Directory.Attribute(dir),
	))
	defer span.End()

	fromVersion, err := ReadVersionFile(dir)
	if err != nil {
		return -1, 0, err
	}

	diff, err := DiffAndSummarize(dir)
	if err != nil {
		return -1, 0, err
	}

	if len(diff.Updates) == 0 {
		return fromVersion, 0, nil
	}

	toVersion := int64(-1)

	updateChan := make(chan *fsdiff_pb.Update, len(diff.Updates))
	objectChan := make(chan *pb.Object, 16)

	group, ctx := errgroup.WithContext(rootCtx)

	for i := 0; i < parallelWorkerCount(); i++ {
		// create the attribute here when `i` is different
		attr := key.Worker.Attribute(i)

		group.Go(func() error {
			ctx, span := telemetry.Start(ctx, "object-reader", trace.WithAttributes(attr))
			defer span.End()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case update, ok := <-updateChan:
					if !ok {
						return nil
					}

					if update.Action == fsdiff_pb.Update_REMOVE {
						objectChan <- &pb.Object{
							Path:    update.Path,
							Deleted: true,
						}
					} else {
						object, err := pb.ObjectFromFilePath(dir, update.Path)
						if err != nil {
							return fmt.Errorf("read file object: %w", err)
						}
						objectChan <- object
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
			return fmt.Errorf("connect fs.Update: %w", err)
		}

		count := 0

		for {
			if count == len(diff.Updates) {
				close(objectChan)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case object, ok := <-objectChan:
				if !ok {
					response, err := stream.CloseAndRecv()
					if err != nil {
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
		toVersion, _, err = c.Rebuild(rootCtx, project, "", nil, dir)
		if err != nil {
			return -1, updateCount, err
		}
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

func parallelWorkerCount() int {
	halfNumCPU := runtime.NumCPU() / 2
	if halfNumCPU < 1 {
		halfNumCPU = 1
	}
	return halfNumCPU
}
