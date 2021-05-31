package api

import (
	"context"
	"io"

	"go.uber.org/zap"

	"github.com/angelini/dateilager/pkg/pb"
	"github.com/jackc/pgx/v4"
)

type Fs struct {
	pb.UnimplementedFsServer

	Log    *zap.Logger
	DbConn DbConnector
}

func (f *Fs) getLatestVersion(ctx context.Context, conn *pgx.Conn, project int32) (int64, error) {
	var latest_version int64

	err := conn.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
		`, project).Scan(&latest_version)
	if err != nil {
		return -1, err
	}

	return latest_version, nil
}

type objectStream func() (*pb.Object, error)

func (f *Fs) getObjects(ctx context.Context, conn *pgx.Conn, project int32, version int64, query *pb.ObjectQuery) (objectStream, error) {
	rows, err := conn.Query(ctx, `
		SELECT path, mode, size
		FROM dl.objects
		WHERE project = $1
		  AND start_version <= $2
		  AND (stop_version IS NULL OR stop_version > $2);
		`, project, version)
	if err != nil {
		return nil, err
	}

	return func() (*pb.Object, error) {
		remaining := rows.Next()
		if !remaining {
			return nil, io.EOF
		}

		var path string
		var mode int32
		var size int32

		err := rows.Scan(&path, &mode, &size)
		if err != nil {
			return nil, err
		}

		return &pb.Object{Path: path, Mode: mode, Size: size}, nil
	}, nil
}

func (f *Fs) Get(req *pb.GetRequest, stream pb.Fs_GetServer) error {
	ctx := stream.Context()

	conn, cancel, err := f.DbConn.Connect(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	var version int64
	if req.Version != nil {
		version = *req.Version
	} else {
		version, err = f.getLatestVersion(ctx, conn, req.Project)
		if err != nil {
			return err
		}
	}

	for _, query := range req.Queries {
		objects, err := f.getObjects(ctx, conn, req.Project, version, query)
		if err != nil {
			return err
		}

		for {
			object, err := objects()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			stream.Send(&pb.GetResponse{Version: version, Object: object})
		}
	}

	return nil
}

func (f *Fs) Update(stream pb.Fs_UpdateServer) error {
	return nil
}
