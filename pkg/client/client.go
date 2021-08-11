package client

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	fsdiff "github.com/gadget-inc/fsdiff/pkg/diff"
	fsdiff_pb "github.com/gadget-inc/fsdiff/pkg/pb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
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

func NewClientConn(ctx context.Context, log *zap.Logger, conn *grpc.ClientConn) *Client {
	return &Client{log: log, conn: conn, fs: pb.NewFsClient(conn)}
}

func NewClient(ctx context.Context, server string) (*Client, error) {
	log, _ := zap.NewDevelopment()

	conn, err := grpc.DialContext(ctx, server, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(50*MB), grpc.MaxCallSendMsgSize(50*MB)),
	)
	if err != nil {
		return nil, err
	}

	return NewClientConn(ctx, log, conn), nil
}

func (c *Client) Close() {
	c.conn.Close()
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
			return -1, 0, fmt.Errorf("send fs.Update: %w", err)
		}
	}

	response, err := stream.CloseAndRecv()
	if err != nil {
		return -1, 0, fmt.Errorf("close fs.Update: %w", err)
	}

	return response.Version, len(diff.Updates), nil
}

func (c *Client) Pack(ctx context.Context, project int64, path string) (int64, error) {
	response, err := c.fs.Pack(ctx, &pb.PackRequest{
		Project: project,
		Path:    path,
	})

	if err != nil {
		return -1, fmt.Errorf("pack path %v in project %v: %w", path, project, err)
	}

	return response.Version, nil
}
