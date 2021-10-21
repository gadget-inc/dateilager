package client

import (
	"archive/tar"
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	fsdiff "github.com/gadget-inc/fsdiff/pkg/diff"
	fsdiff_pb "github.com/gadget-inc/fsdiff/pkg/pb"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
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
	log  *zap.Logger
	conn *grpc.ClientConn
	fs   pb.FsClient
}

func NewClientConn(log *zap.Logger, conn *grpc.ClientConn) *Client {
	return &Client{log: log, conn: conn, fs: pb.NewFsClient(conn)}
}

func NewClient(ctx context.Context, server, token string) (*Client, error) {
	log, _ := zap.NewDevelopment()

	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system cert pool: %w", err)
	}
	creds := credentials.NewClientTLSFromCert(pool, "")

	auth := oauth.NewOauthAccess(&oauth2.Token{
		AccessToken: token,
	})

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(connectCtx, server,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(auth),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*MB), grpc.MaxCallSendMsgSize(100*MB)),
	)
	if err != nil {
		return nil, err
	}

	return NewClientConn(log, conn), nil
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) ListProjects(ctx context.Context) ([]*pb.Project, error) {
	resp, err := c.fs.ListProjects(ctx, &pb.ListProjectsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	return resp.Projects, nil
}

func (c *Client) Get(ctx context.Context, project int64, prefix string, vrange VersionRange) ([]*pb.Object, error) {
	var objects []*pb.Object

	query := &pb.ObjectQuery{
		Path:        prefix,
		IsPrefix:    true,
		WithContent: true,
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

func (c *Client) Rebuild(ctx context.Context, project int64, prefix string, vrange VersionRange, output string) error {
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

	stream, err := c.fs.GetCompress(ctx, request)
	if err != nil {
		return fmt.Errorf("connect fs.GetCompress: %w", err)
	}

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("receive fs.GetCompress: %w", err)
		}

		tarReader := db.NewTarReader(response.Bytes)
		defer tarReader.Close()

		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("next TAR header: %w", err)
			}

			path := filepath.Join(output, header.Name)

			switch header.Typeflag {
			case tar.TypeReg:
				err = os.MkdirAll(filepath.Dir(path), 0777)
				if err != nil {
					return fmt.Errorf("mkdir -p %v: %w", filepath.Dir(path), err)
				}

				file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
				if err != nil {
					return fmt.Errorf("open file %v: %w", path, err)
				}

				err = tarReader.CopyContent(file)
				if err != nil {
					return fmt.Errorf("write %v to disk: %w", path, err)
				}

			case tar.TypeDir:
				err = os.MkdirAll(path, os.FileMode(header.Mode))
				if err != nil {
					return fmt.Errorf("mkdir -p %v: %w", path, err)
				}

			case tar.TypeSymlink:
				err = os.MkdirAll(filepath.Dir(path), 0777)
				if err != nil {
					return fmt.Errorf("mkdir -p %v: %w", filepath.Dir(path), err)
				}

				err = os.Symlink(header.Linkname, path)
				if err != nil {
					return fmt.Errorf("ln -s %v %v: %w", header.Linkname, path, err)
				}

			case 'D':
				err = os.Remove(path)
				if err != nil {
					return fmt.Errorf("remove %v from disk: %w", path, err)
				}

			default:
				c.log.Warn("skipping unhandled TAR type", zap.Any("flag", header.Typeflag))
			}
		}
	}

	return nil
}

func (c *Client) Update(ctx context.Context, project int64, diffPath string, directory string) (int64, int, error) {
	stream, err := c.fs.Update(ctx)
	if err != nil {
		return -1, 0, fmt.Errorf("connect fs.Update: %w", err)
	}

	diff, err := fsdiff.ReadDiff(diffPath)
	if err != nil {
		return -1, 0, fmt.Errorf("parse diff file: %w", err)
	}

	var object *pb.Object

	for _, update := range diff.Updates {
		if update.Action == fsdiff_pb.Update_REMOVE {
			object = &pb.Object{
				Path:    update.Path,
				Deleted: true,
			}
		} else {
			object, err = pb.ObjectFromFilePath(directory, update.Path)
			if err != nil {
				return -1, 0, fmt.Errorf("read file object: %w", err)
			}
		}

		err = stream.Send(&pb.UpdateRequest{
			Project: project,
			Object:  object,
		})
		if err != nil {
			return -1, 0, fmt.Errorf("send fs.Update, path %v, size %v, mode %v, deleted %v: %w", object.Path, object.Size, object.Mode, object.Deleted, err)
		}
	}

	response, err := stream.CloseAndRecv()
	if err != nil {
		return -1, 0, fmt.Errorf("close fs.Update: %w", err)
	}

	return response.Version, len(diff.Updates), nil
}

func (c *Client) Inspect(ctx context.Context, project int64) (*pb.InspectResponse, error) {
	inspect, err := c.fs.Inspect(ctx, &pb.InspectRequest{Project: project})
	if err != nil {
		return nil, fmt.Errorf("inspect project %v: %w", project, err)
	}

	return inspect, nil
}

func (c *Client) Snapshot(ctx context.Context) (string, error) {
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
