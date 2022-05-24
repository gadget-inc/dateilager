package client

import (
	"archive/tar"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gadget-inc/dateilager/internal/db"
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

func NewClient(ctx context.Context, server, token string) (*Client, error) {
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

	auth := oauth.NewOauthAccess(&oauth2.Token{
		AccessToken: token,
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

func (c *Client) FetchArchives(ctx context.Context, project int64, prefix string, vrange VersionRange, output string) (int64, error) {
	ctx, span := telemetry.Start(ctx, "client.fetch-archives", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Prefix.Attribute(prefix),
		key.FromVersion.Attribute(vrange.From),
		key.ToVersion.Attribute(vrange.To),
		key.Output.Attribute(output),
	))
	defer span.End()

	query := &pb.ObjectQuery{
		Path:        prefix,
		IsPrefix:    true,
		WithContent: true,
	}

	request := &pb.GetCompressRequest{
		Project:     project,
		FromVersion: vrange.From,
		ToVersion:   vrange.To,
		Queries:     []*pb.ObjectQuery{query},
	}

	index := 0
	version := int64(-1)

	stream, err := c.fs.GetCompress(ctx, request)
	if err != nil {
		return -1, fmt.Errorf("connect fs.GetCompress: %w", err)
	}

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return -1, fmt.Errorf("receive fs.GetCompress: %w", err)
		}

		version = response.Version
		path := filepath.Join(output, fmt.Sprintf("%v.tar.s2", index))

		err = os.WriteFile(path, response.Bytes, 0644)
		if err != nil {
			return version, fmt.Errorf("writing archive: %w", err)
		}

		index += 1
	}

	return version, nil
}

func writeObject(outputDir string, reader *db.TarReader, header *tar.Header) error {
	path := filepath.Join(outputDir, header.Name)

	switch header.Typeflag {
	case tar.TypeReg:
		err := os.MkdirAll(filepath.Dir(path), 0777)
		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", filepath.Dir(path), err)
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("open file %v: %w", path, err)
		}

		err = reader.CopyContent(file)
		file.Close()
		if err != nil {
			return fmt.Errorf("write %v to disk: %w", path, err)
		}

	case tar.TypeDir:
		err := os.MkdirAll(path, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", path, err)
		}

	case tar.TypeSymlink:
		err := os.MkdirAll(filepath.Dir(path), 0777)
		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", filepath.Dir(path), err)
		}

		// Remove existing link
		if _, err = os.Stat(path); err == nil {
			err = os.Remove(path)
			if err != nil {
				return fmt.Errorf("rm %v before symlinking %v: %w", path, header.Linkname, err)
			}
		}

		err = os.Symlink(header.Linkname, path)
		if err != nil {
			return fmt.Errorf("ln -s %v %v: %w", header.Linkname, path, err)
		}

	case 'D':
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return fmt.Errorf("remove %v from disk: %w", path, err)
		}

	default:
		return fmt.Errorf("unhandle TAR type: %v", header.Typeflag)
	}

	return nil
}

func (c *Client) Rebuild(ctx context.Context, project int64, prefix string, vrange VersionRange, output string) (int64, uint32, error) {
	ctx, span := telemetry.Start(ctx, "client.rebuild", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Prefix.Attribute(prefix),
		key.FromVersion.Attribute(vrange.From),
		key.ToVersion.Attribute(vrange.To),
		key.Output.Attribute(output),
	))
	defer span.End()

	query := &pb.ObjectQuery{
		Path:        prefix,
		IsPrefix:    true,
		WithContent: true,
	}

	request := &pb.GetCompressRequest{
		Project:     project,
		FromVersion: vrange.From,
		ToVersion:   vrange.To,
		Queries:     []*pb.ObjectQuery{query},
	}

	version := int64(-1)
	var diffCount uint32

	stream, err := c.fs.GetCompress(ctx, request)
	if err != nil {
		return -1, diffCount, fmt.Errorf("connect fs.GetCompress: %w", err)
	}

	// Pull one response before booting workers
	// This is a short circuit for cases where there are no diffs to apply
	response, err := stream.Recv()
	if err == io.EOF {
		return version, diffCount, nil
	}
	if err != nil {
		return -1, diffCount, fmt.Errorf("receive fs.GetCompress: %w", err)
	}

	tarBytesChan := make(chan []byte, 16)
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		ctx, span := telemetry.Start(ctx, "object-receiver")
		defer span.End()
		defer close(tarBytesChan)

		for {
			version = response.Version

			select {
			case <-ctx.Done():
				return ctx.Err()
			case tarBytesChan <- response.Bytes:
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
				case tarBytes, ok := <-tarBytesChan:
					if !ok {
						return nil
					}

					tarReader := db.NewTarReader(tarBytes)

					for {
						header, err := tarReader.Next()
						if err == io.EOF {
							break
						}
						if err != nil {
							return fmt.Errorf("next TAR header: %w", err)
						}

						atomic.AddUint32(&diffCount, 1)
						err = writeObject(output, tarReader, header)
						if err != nil {
							return err
						}
					}
				}
			}
		})
	}

	if err = group.Wait(); err != nil {
		return -1, diffCount, err
	}

	return version, diffCount, nil
}

func (c *Client) Update(ctx context.Context, project int64, diff *fsdiff_pb.Diff, directory string) (int64, uint32, error) {
	ctx, span := telemetry.Start(ctx, "client.update", trace.WithAttributes(
		key.Project.Attribute(project),
		key.Directory.Attribute(directory),
	))
	defer span.End()

	version := int64(-1)

	updateChan := make(chan *fsdiff_pb.Update, len(diff.Updates))
	objectChan := make(chan *pb.Object, 16)

	group, ctx := errgroup.WithContext(ctx)

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
						object, err := pb.ObjectFromFilePath(directory, update.Path)
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
					version = response.Version
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

	if err := group.Wait(); err != nil {
		return -1, 0, err
	}

	return version, uint32(len(diff.Updates)), nil
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
