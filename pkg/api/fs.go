package api

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/angelini/dateilager/internal/pb"
	"github.com/jackc/pgx/v4"
)

type Fs struct {
	pb.UnimplementedFsServer

	Log    *zap.Logger
	DbConn DbConnector
}

func (f *Fs) getLatestVersion(ctx context.Context, tx pgx.Tx, project int32) (int64, error) {
	var latest_version int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
	`, project).Scan(&latest_version)
	if err != nil {
		return -1, fmt.Errorf("FS get latest version: %w", err)
	}

	return latest_version, nil
}

func (f *Fs) getProjectSize(ctx context.Context, tx pgx.Tx, project int32, version int64) (int, error) {
	var size int

	err := tx.QueryRow(ctx, `
		SELECT sum(size)
		FROM dl.objects
		WHERE project = $1
		  AND start_version <= $2
		  AND (stop_version IS NULL OR stop_version > $2)
	`, project, version).Scan(&size)
	if err != nil {
		return -1, fmt.Errorf("FS get project size: %w", err)
	}

	return size, nil
}

type objectStream func() (*pb.Object, error)

func (f *Fs) getObjects(ctx context.Context, tx pgx.Tx, project int32, version int64, query *pb.ObjectQuery) (objectStream, error) {
	sql := `
		SELECT o.path, o.mode, o.size, c.bytes
		FROM dl.objects o
		JOIN dl.contents c
		  ON o.hash = c.hash
		WHERE o.project = $1
		  AND o.start_version <= $2
		  AND (o.stop_version IS NULL OR o.stop_version > $2)
	`

	var path string
	if query.IsPrefix {
		sql = sql + `
			AND o.path LIKE $3
		`
		path = fmt.Sprintf("%s%%", query.Path)
	} else {
		sql = sql + `
			AND o.path = $3
		`
		path = query.Path
	}

	sql = sql + `
		ORDER BY o.path
	`

	rows, err := tx.Query(ctx, sql, project, version, path)
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
		var bytes []byte

		err := rows.Scan(&path, &mode, &size, &bytes)
		if err != nil {
			return nil, err
		}

		return &pb.Object{
			Path:     path,
			Mode:     mode,
			Size:     size,
			Contents: bytes,
		}, nil
	}, nil
}

func (f *Fs) Get(req *pb.GetRequest, stream pb.Fs_GetServer) error {
	ctx := stream.Context()

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return err
	}
	defer close()

	var version int64
	if req.Version != nil {
		version = *req.Version
	} else {
		version, err = f.getLatestVersion(ctx, tx, req.Project)
		if err != nil {
			return err
		}
	}

	for _, query := range req.Queries {
		objects, err := f.getObjects(ctx, tx, req.Project, version, query)
		if err != nil {
			return fmt.Errorf("FS get objects: %w", err)
		}

		for {
			object, err := objects()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			err = stream.Send(&pb.GetResponse{Version: version, Object: object})
			if err != nil {
				return fmt.Errorf("send GetResponse: %w", err)
			}
		}
	}

	return nil
}

func writeObjectToTar(tarWriter *tar.Writer, object *pb.Object) error {
	header := &tar.Header{
		Name: object.Path,
		Mode: int64(object.Mode),
		Size: int64(object.Size),
	}

	err := tarWriter.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("write header to TAR %v: %w", object.Path, err)
	}

	_, err = tarWriter.Write(object.Contents)
	if err != nil {
		return fmt.Errorf("write contents to TAR %v: %w", object.Path, err)
	}

	return nil
}

func sendTar(tarWriter *tar.Writer, buffer *bytes.Buffer, stream pb.Fs_GetCompressServer) error {
	err := tarWriter.Close()
	if err != nil {
		return fmt.Errorf("close TAR writer: %w", err)
	}

	err = stream.Send(&pb.GetCompressResponse{
		Format: pb.GetCompressResponse_ZSTD_TAR,
		Bytes:  buffer.Bytes(),
	})
	if err != nil {
		return fmt.Errorf("send GetCompressResponse: %w", err)
	}

	return nil
}

func (f *Fs) GetCompress(req *pb.GetCompressRequest, stream pb.Fs_GetCompressServer) error {
	ctx := stream.Context()

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return err
	}
	defer close()

	var version int64
	if req.Version != nil {
		version = *req.Version
	} else {
		version, err = f.getLatestVersion(ctx, tx, req.Project)
		if err != nil {
			return err
		}
	}

	size, err := f.getProjectSize(ctx, tx, req.Project, version)
	if err != nil {
		return err
	}

	targetSize := size / int(req.ResponseCount)
	currentSize := 0

	var buffer bytes.Buffer
	tarWriter := tar.NewWriter(&buffer)

	for _, query := range req.Queries {
		objects, err := f.getObjects(ctx, tx, req.Project, version, query)
		if err != nil {
			return fmt.Errorf("FS get objects: %w", err)
		}

		for {
			object, err := objects()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			err = writeObjectToTar(tarWriter, object)
			if err != nil {
				return err
			}

			currentSize = currentSize + int(object.Size)
			if currentSize > targetSize {
				currentSize = 0

				err = sendTar(tarWriter, &buffer, stream)
				if err != nil {
					return err
				}

				buffer.Truncate(0)
				tarWriter = tar.NewWriter(&buffer)
			}
		}
	}

	if currentSize > 0 {
		return sendTar(tarWriter, &buffer, stream)
	}

	return nil
}

func (f *Fs) deleteObject(ctx context.Context, tx pgx.Tx, project int32, version int64, object *pb.Object) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = $1
		WHERE project = $2
		  AND path = $3
		  AND stop_version IS NULL
	`, version, project, object.Path)
	if err != nil {
		return fmt.Errorf("update deleted object: %w", err)
	}

	return nil
}

func (f *Fs) updateObject(ctx context.Context, tx pgx.Tx, project int32, version int64, object *pb.Object) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.objects SET stop_version = $1
		WHERE project = $2
		  AND path = $3
		  AND stop_version IS NULL
	`, version, project, object.Path)
	if err != nil {
		return fmt.Errorf("FS update latest version: %w", err)
	}

	contents := object.Contents
	if contents == nil {
		contents = []byte("")
	}
	h1, h2 := HashContents(contents)

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7)
	`, project, version, object.Path, h1, h2, object.Mode, object.Size)
	if err != nil {
		return fmt.Errorf("FS insert new object version: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		   DO NOTHING
	`, h1, h2, object.Contents)
	if err != nil {
		return fmt.Errorf("FS insert contents: %w", err)
	}

	return nil
}

func (f *Fs) Update(stream pb.Fs_UpdateServer) error {
	ctx := stream.Context()

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return err
	}
	defer close()

	// We only receive a project ID after the first streamed update
	project := int32(-1)
	version := int64(-1)

	for {
		request, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("FS receive update request: %w", err)
		}

		if project == -1 {
			project = request.Project

			latest_version, err := f.getLatestVersion(ctx, tx, project)
			if err != nil {
				return err
			}

			version = latest_version + 1
			f.Log.Info("project update", zap.Int32("project", project), zap.Int64("version", version))
		}

		if project != request.Project {
			return fmt.Errorf("multiple projects in one update call: %v %v", project, request.Project)
		}

		if request.Delete {
			err = f.deleteObject(ctx, tx, project, version, request.Object)
		} else {
			err = f.updateObject(ctx, tx, project, version, request.Object)
		}

		if err != nil {
			f.Log.Error("FS update", zap.Error(err))
			return fmt.Errorf("FS update: %w", err)
		}
	}

	tx.Exec(ctx, `
		UPDATE dl.projects
		SET latest_version = $1
		WHERE id = $2
	`, version, project)

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("FS update commit tx: %w", err)
	}

	return stream.SendAndClose(&pb.UpdateResponse{Version: version})
}
