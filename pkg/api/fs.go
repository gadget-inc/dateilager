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

const (
	TargetTarSize = 512 * 1024
)

type versionRange struct {
	from int64
	to   int64
}

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
		return -1, fmt.Errorf("FS get latest version, project %v: %w", project, err)
	}

	return latest_version, nil
}

func (f *Fs) lockLatestVersion(ctx context.Context, tx pgx.Tx, project int32) (int64, error) {
	var latest_version int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
		FOR UPDATE
	`, project).Scan(&latest_version)
	if err != nil {
		return -1, fmt.Errorf("FS lock latest version, project %v: %w", project, err)
	}

	return latest_version, nil
}

func (f *Fs) updateLatestVersion(ctx context.Context, tx pgx.Tx, project int32, version int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.projects
		SET latest_version = $1
		WHERE id = $2
	`, version, project)
	if err != nil {
		return fmt.Errorf("FS update latest version, project %v version %v: %w", project, version, err)
	}

	return nil
}

func (f *Fs) buildVersionRange(ctx context.Context, tx pgx.Tx, project int32, from *int64, to *int64) (versionRange, error) {
	vrange := versionRange{}

	if from == nil {
		vrange.from = 0
	} else {
		vrange.from = *from
	}

	if to == nil {
		latest, err := f.getLatestVersion(ctx, tx, project)
		if err != nil {
			return vrange, err
		}
		vrange.to = latest
	} else {
		vrange.to = *to
	}

	return vrange, nil
}

func (f *Fs) Get(req *pb.GetRequest, stream pb.Fs_GetServer) error {
	ctx := stream.Context()

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return err
	}
	defer close()

	vrange, err := f.buildVersionRange(ctx, tx, req.Project, req.FromVersion, req.ToVersion)
	if err != nil {
		return err
	}

	for _, query := range req.Queries {
		objects, err := getObjects(ctx, tx, req.Project, vrange, query)
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

			err = stream.Send(&pb.GetResponse{Version: vrange.to, Object: object})
			if err != nil {
				return fmt.Errorf("send GetResponse: %w", err)
			}
		}
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

	vrange, err := f.buildVersionRange(ctx, tx, req.Project, req.FromVersion, req.ToVersion)
	if err != nil {
		return err
	}

	for _, query := range req.Queries {
		tars, err := getTars(ctx, tx, req.Project, vrange, query)
		if err != nil {
			return err
		}

		for {
			tar, err := tars()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			err = stream.Send(&pb.GetCompressResponse{
				Version: vrange.to,
				Format:  pb.GetCompressResponse_ZSTD_TAR,
				Bytes:   tar,
			})
			if err != nil {
				return fmt.Errorf("send GetCompressResponse: %w", err)
			}
		}
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
		return fmt.Errorf("delete object: %w", err)
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

	content := object.Content
	if content == nil {
		content = []byte("")
	}
	h1, h2 := HashContent(content)

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
	`, project, version, object.Path, h1, h2, object.Mode, object.Size, false)
	if err != nil {
		return fmt.Errorf("FS insert new object version: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		   DO NOTHING
	`, h1, h2, content)
	if err != nil {
		return fmt.Errorf("FS insert content: %w", err)
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

			latest_version, err := f.lockLatestVersion(ctx, tx, project)
			if err != nil {
				return err
			}

			version = latest_version + 1
			f.Log.Info("project update", zap.Int32("project", project), zap.Int64("version", version))
		}

		if project != request.Project {
			return fmt.Errorf("multiple projects in one update call: %v %v", project, request.Project)
		}

		if request.Object.Deleted {
			err = f.deleteObject(ctx, tx, project, version, request.Object)
		} else {
			err = f.updateObject(ctx, tx, project, version, request.Object)
		}

		if err != nil {
			f.Log.Error("FS update", zap.Error(err))
			return fmt.Errorf("FS update: %w", err)
		}
	}

	err = f.updateLatestVersion(ctx, tx, project, version)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("FS update commit tx: %w", err)
	}

	return stream.SendAndClose(&pb.UpdateResponse{Version: version})
}

func (f *Fs) deleteObjects(ctx context.Context, tx pgx.Tx, project int32, version int64, path string) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = $1
		WHERE project = $2
		  AND path LIKE $3
		  AND stop_version IS NULL
	`, version, project, fmt.Sprintf("%s%%", path))
	if err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}

	return nil
}

func (f *Fs) insertPackedObject(ctx context.Context, tx pgx.Tx, project int32, version int64, path string, content []byte) error {
	h1, h2 := HashContent(content)

	_, err := tx.Exec(ctx, `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
	`, project, version, path, h1, h2, 0, len(content), true)
	if err != nil {
		return fmt.Errorf("FS insert new packed object: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		DO NOTHING
	`, h1, h2, content)
	if err != nil {
		return fmt.Errorf("FS insert content: %w", err)
	}

	return nil
}

func (f *Fs) Pack(ctx context.Context, request *pb.PackRequest) (*pb.PackResponse, error) {
	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer close()

	latest_version, err := f.lockLatestVersion(ctx, tx, request.Project)
	if err != nil {
		return nil, err
	}

	vrange := versionRange{from: 0, to: latest_version}
	query := pb.ObjectQuery{
		Path:        request.Path,
		IsPrefix:    true,
		WithContent: true,
	}

	var buffer bytes.Buffer
	tarWriter := tar.NewWriter(&buffer)

	objects, err := getObjects(ctx, tx, request.Project, vrange, &query)
	if err != nil {
		return nil, err
	}

	for {
		object, err := objects()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		err = writeObjectToTar(tarWriter, object)
		if err != nil {
			return nil, err
		}
	}

	err = tarWriter.Close()
	if err != nil {
		return nil, err
	}

	version := latest_version + 1

	err = f.deleteObjects(ctx, tx, request.Project, version, request.Path)
	if err != nil {
		return nil, err
	}

	err = f.insertPackedObject(ctx, tx, request.Project, version, request.Path, buffer.Bytes())
	if err != nil {
		return nil, err
	}

	err = f.updateLatestVersion(ctx, tx, request.Project, version)
	if err != nil {
		return nil, err
	}

	return &pb.PackResponse{
		Version: version,
	}, nil
}
