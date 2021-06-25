package api

import (
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v4"
)

type Fs struct {
	pb.UnimplementedFsServer

	Log    *zap.Logger
	DbConn db.DbConnector
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

func (f *Fs) buildVersionRange(ctx context.Context, tx pgx.Tx, project int32, from *int64, to *int64) (db.VersionRange, error) {
	vrange := db.VersionRange{}

	if from == nil {
		vrange.From = 0
	} else {
		vrange.From = *from
	}

	if to == nil {
		latest, err := f.getLatestVersion(ctx, tx, project)
		if err != nil {
			return vrange, err
		}
		vrange.To = latest
	} else {
		vrange.To = *to
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
		objects, err := db.GetObjects(ctx, tx, req.Project, vrange, query)
		if err != nil {
			return err
		}

		for {
			object, err := objects()
			if err == db.SKIP {
				continue
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			err = stream.Send(&pb.GetResponse{Version: vrange.To, Object: object})
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
		tars, err := db.GetTars(ctx, tx, req.Project, vrange, query)
		if err != nil {
			return err
		}

		for {
			tar, err := tars()
			if err == io.EOF {
				break
			}
			if err == db.SKIP {
				continue
			}
			if err != nil {
				return err
			}

			err = stream.Send(&pb.GetCompressResponse{
				Version: vrange.To,
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

func (f *Fs) updateObject(ctx context.Context, tx pgx.Tx, encoder *db.ContentEncoder, project int32, version int64, object *pb.Object) error {
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
	h1, h2 := db.HashContent(content)

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
	`, project, version, object.Path, h1, h2, object.Mode, object.Size, false)
	if err != nil {
		return fmt.Errorf("FS insert new object version: %w", err)
	}

	encoded, err := encoder.Encode(content)
	if err != nil {
		return fmt.Errorf("FS update encode content: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.contents (hash, bytes, names_tar)
		VALUES (($1, $2), $3, NULL)
		ON CONFLICT
		   DO NOTHING
	`, h1, h2, encoded)
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

	contentEncoder := db.NewContentEncoder()

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
			err = f.updateObject(ctx, tx, contentEncoder, project, version, request.Object)
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
		RETURNING path;
	`, version, project, fmt.Sprintf("%s%%", path))
	if err != nil {
		return fmt.Errorf("FS delete objects: %w", err)
	}

	return nil
}

func (f *Fs) insertPackedObject(ctx context.Context, tx pgx.Tx, project int32, version int64, path string, contentTar, namesTar []byte) error {
	h1, h2 := db.HashContent(contentTar)

	_, err := tx.Exec(ctx, `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
	`, project, version, path, h1, h2, 0, len(contentTar), true)
	if err != nil {
		return fmt.Errorf("FS insert new packed object: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.contents (hash, bytes, names_tar)
		VALUES (($1, $2), $3, $4)
		ON CONFLICT
		DO NOTHING
	`, h1, h2, contentTar, namesTar)
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

	vrange := db.VersionRange{From: 0, To: latest_version}
	query := pb.ObjectQuery{
		Path:        request.Path,
		IsPrefix:    true,
		WithContent: true,
	}

	objects, err := db.GetObjects(ctx, tx, request.Project, vrange, &query)
	if err != nil {
		return nil, err
	}

	fullTar, namesTar, err := db.PackObjects(objects)
	if err == db.ErrEmptyPack {
		return &pb.PackResponse{
			Version: latest_version,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	version := latest_version + 1

	err = f.deleteObjects(ctx, tx, request.Project, version, request.Path)
	if err != nil {
		return nil, err
	}

	err = f.insertPackedObject(ctx, tx, request.Project, version, request.Path, fullTar, namesTar)
	if err != nil {
		return nil, err
	}

	err = f.updateLatestVersion(ctx, tx, request.Project, version)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("FS pack commit tx: %w", err)
	}

	return &pb.PackResponse{
		Version: version,
	}, nil
}
