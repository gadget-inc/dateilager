package client

import (
	"context"
	"fmt"
	"io"

	"github.com/angelini/dateilager/pkg/pb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Client struct {
	log  *zap.Logger
	conn *grpc.ClientConn
	fs   pb.FsClient
}

func NewClient(ctx context.Context, server string) (*Client, error) {
	log, _ := zap.NewDevelopment()

	conn, err := grpc.DialContext(ctx, server, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	return &Client{log: log, conn: conn, fs: pb.NewFsClient(conn)}, nil
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) GetLatestRoot(ctx context.Context, project int32) ([]*pb.Object, error) {
	var objects []*pb.Object

	query := &pb.ObjectQuery{
		Path:        "/",
		IsPrefix:    true,
		WithContent: true,
	}

	request := &pb.GetRequest{
		Project: project,
		Queries: []*pb.ObjectQuery{query},
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
