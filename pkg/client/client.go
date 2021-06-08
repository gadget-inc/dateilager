package client

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/angelini/dateilager/internal/pb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

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

func (c *Client) Get(ctx context.Context, project int32, prefix string, version *int64) ([]*pb.Object, error) {
	var objects []*pb.Object

	query := &pb.ObjectQuery{
		Path:     prefix,
		IsPrefix: true,
	}

	request := &pb.GetRequest{
		Project:     project,
		FromVersion: nil,
		ToVersion:   version,
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

func (c *Client) Rebuild(ctx context.Context, project int32, prefix string, version *int64, output string) error {
	query := &pb.ObjectQuery{
		Path:     prefix,
		IsPrefix: true,
	}

	request := &pb.GetCompressRequest{
		Project:       project,
		FromVersion:   nil,
		ToVersion:     version,
		ResponseCount: 8,
		Queries:       []*pb.ObjectQuery{query},
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

		tarReader := tar.NewReader(bytes.NewBuffer(response.Bytes))

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

				_, err = io.Copy(file, tarReader)
				if err != nil {
					return fmt.Errorf("write %v to disk: %w", path, err)
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

		object, deleted, err := readFileObject(directory, path)
		if err != nil {
			return -1, fmt.Errorf("read file object: %w", err)
		}

		err = stream.Send(&pb.UpdateRequest{
			Project: project,
			Object:  object,
			Delete:  deleted,
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
