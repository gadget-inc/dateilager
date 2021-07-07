package api

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v4"
)

var (
	ErrMultipleProjectsPerUpdate = errors.New("multiple objects in one update")
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
	f.Log.Info("FS.Get[Init]", zap.Int32("project", req.Project), zap.Any("vrange", vrange))

	packedCache, err := db.NewPackedCache(ctx, tx, req.Project, vrange)
	if err != nil {
		return err
	}

	for _, query := range req.Queries {
		f.Log.Info("FS.Get[Query]",
			zap.Int32("project", req.Project),
			zap.Any("vrange", vrange),
			zap.String("path", query.Path),
			zap.Bool("isPrefix", query.IsPrefix),
			zap.Bool("withContent", query.WithContent),
		)

		objects, err := db.GetObjects(ctx, tx, packedCache, req.Project, vrange, query)
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
				return fmt.Errorf("FS send GetResponse: %w", err)
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
	f.Log.Info("FS.GetCompress[Init]", zap.Int32("project", req.Project), zap.Any("vrange", vrange))

	for _, query := range req.Queries {
		f.Log.Info("FS.GetCompress[Query]",
			zap.Int32("project", req.Project),
			zap.Any("vrange", vrange),
			zap.String("path", query.Path),
			zap.Bool("isPrefix", query.IsPrefix),
			zap.Bool("withContent", query.WithContent),
		)

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
				return fmt.Errorf("FS send GetCompressResponse: %w", err)
			}
		}
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

	var packedCache *db.PackedCache
	buffer := make(map[string][]*pb.Object)

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
			f.Log.Info("FS.Update[Init]", zap.Int32("project", project), zap.Int64("version", version))

			packedCache, err = db.NewPackedCache(ctx, tx, project, db.VersionRange{From: 0, To: version})
			if err != nil {
				return err
			}
		}

		if project != request.Project {
			return fmt.Errorf("initial project %v, next project %v: %w", project, request.Project, ErrMultipleProjectsPerUpdate)
		}

		parent, isPacked := packedCache.IsParentPacked(request.Object.Path)
		if isPacked {
			buffer[parent] = append(buffer[parent], request.Object)
			continue
		}

		if request.Object.Deleted {
			err = db.DeleteObject(ctx, tx, project, version, request.Object.Path)
		} else {
			err = db.UpdateObject(ctx, tx, contentEncoder, project, version, request.Object)
		}

		if err != nil {
			return fmt.Errorf("FS update: %w", err)
		}
	}

	for parent, objects := range buffer {
		err = db.UpdatePackedObjects(ctx, tx, project, version, parent, objects)
		if err != nil {
			return fmt.Errorf("FS update packed objects for %v: %w", parent, err)
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

	f.Log.Info("FS.Update[Commit]", zap.Int32("project", project), zap.Int64("version", version))

	return stream.SendAndClose(&pb.UpdateResponse{Version: version})
}

func (f *Fs) Pack(ctx context.Context, req *pb.PackRequest) (*pb.PackResponse, error) {
	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer close()

	f.Log.Info("FS.Pack[Init]", zap.Int32("project", req.Project), zap.String("path", req.Path))

	latest_version, err := f.lockLatestVersion(ctx, tx, req.Project)
	if err != nil {
		return nil, err
	}

	vrange := db.VersionRange{From: 0, To: latest_version}
	query := pb.ObjectQuery{
		Path:        req.Path,
		IsPrefix:    true,
		WithContent: true,
	}

	packedCache, err := db.NewPackedCache(ctx, tx, req.Project, vrange)
	if err != nil {
		return nil, err
	}

	objects, err := db.GetObjects(ctx, tx, packedCache, req.Project, vrange, &query)
	if err != nil {
		return nil, err
	}

	fullTar, namesTar, err := db.PackObjects(objects)
	if err == db.ErrEmptyPack {
		f.Log.Info("FS.Pack[Empty]", zap.Int32("project", req.Project), zap.String("path", req.Path), zap.Int64("version", latest_version))

		return &pb.PackResponse{
			Version: latest_version,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	version := latest_version + 1

	err = db.DeleteObjects(ctx, tx, req.Project, version, req.Path)
	if err != nil {
		return nil, err
	}

	err = db.InsertPackedObject(ctx, tx, req.Project, version, req.Path, fullTar, namesTar)
	if err != nil {
		return nil, err
	}

	err = f.updateLatestVersion(ctx, tx, req.Project, version)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("FS pack commit tx: %w", err)
	}
	f.Log.Info("FS.Pack[Commit]", zap.Int32("project", req.Project), zap.String("path", req.Path), zap.Int64("version", version))

	return &pb.PackResponse{
		Version: version,
	}, nil
}
