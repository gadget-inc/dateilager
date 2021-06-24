package client

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/angelini/dateilager/internal/pb"
	"github.com/angelini/dateilager/pkg/api"
	"go.uber.org/zap"
	"google.golang.org/grpc"
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

	conn, err := grpc.DialContext(ctx, server, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	return NewClientConn(ctx, log, conn), nil
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) Get(ctx context.Context, project int32, prefix string, vrange VersionRange) ([]*pb.Object, error) {
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

func (c *Client) Rebuild(ctx context.Context, project int32, prefix string, vrange VersionRange, output string) error {
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

		tarReader := api.NewTarReader(response.Bytes)
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

func (c *Client) Update(ctx context.Context, project int32, paths []string, directory string) (int64, error) {
	stream, err := c.fs.Update(ctx)
	if err != nil {
		return -1, fmt.Errorf("connect fs.Update: %w", err)
	}

	for _, path := range paths {
		if path == "" {
			continue
		}

		object, err := readFileObject(directory, path)
		if err != nil {
			return -1, fmt.Errorf("read file object: %w", err)
		}

		err = stream.Send(&pb.UpdateRequest{
			Project: project,
			Object:  object,
		})
		if err != nil {
			return -1, fmt.Errorf("send fs.Update: %w", err)
		}
	}

	response, err := stream.CloseAndRecv()
	if err != nil {
		return -1, fmt.Errorf("close fs.Update: %w", err)
	}

	return response.Version, nil
}

func (c *Client) Pack(ctx context.Context, project int32, path string) (int64, error) {
	response, err := c.fs.Pack(ctx, &pb.PackRequest{
		Project: project,
		Path:    path,
	})

	if err != nil {
		return -1, fmt.Errorf("pack path %v in project %v: %w", path, project, err)
	}

	return response.Version, nil
}
