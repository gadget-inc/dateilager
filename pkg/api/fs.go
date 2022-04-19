package api

import (
	"context"
	"errors"
	"io"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/pb"
)

var (
	ErrMultipleProjectsPerUpdate = errors.New("multiple objects in one update")
)

func requireAdminAuth(ctx context.Context) error {
	ctxAuth := ctx.Value(auth.AuthCtxKey).(auth.Auth)

	if ctxAuth.Role == auth.Admin {
		return nil
	}

	return status.Errorf(codes.PermissionDenied, "FS endpoint requires admin access")
}

func requireProjectAuth(ctx context.Context) (int64, error) {
	ctxAuth := ctx.Value(auth.AuthCtxKey).(auth.Auth)

	if ctxAuth.Role == auth.Admin {
		return -1, nil
	}

	if ctxAuth.Role == auth.Project {
		return *ctxAuth.Project, nil
	}

	return -1, status.Errorf(codes.PermissionDenied, "FS endpoint requires project access")
}

type Fs struct {
	pb.UnimplementedFsServer

	Env    environment.Env
	DbConn db.DbConnector
}

func (f *Fs) NewProject(ctx context.Context, req *pb.NewProjectRequest) (*pb.NewProjectResponse, error) {
	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.NewProject[Init]", zap.Int64("id", req.Id), zap.Int64p("template", req.Template))

	err = db.CreateProject(ctx, tx, req.Id, req.PackPatterns)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS new project %v, %v", req.Id, err)
	}

	if req.Template != nil {
		err = db.CopyAllObjects(ctx, tx, *req.Template, req.Id)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "FS new project copy from template %v to %v, %v", req.Template, req.Id, err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS new project commit tx: %v", err)
	}

	logger.Debug(ctx, "FS.NewProject[Commit]", zap.Int64("id", req.Id), zap.Int64p("template", req.Template))

	return &pb.NewProjectResponse{}, nil
}

func (f *Fs) DeleteProject(ctx context.Context, req *pb.DeleteProjectRequest) (*pb.DeleteProjectResponse, error) {
	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.DeleteProject[Init]", zap.Int64("project", req.Project))
	err = db.DeleteProject(ctx, tx, req.Project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS delete project %v: %v", req.Project, err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS delete commit tx: %v", err)
	}
	logger.Debug(ctx, "FS.DeleteProject[Commit]")

	return &pb.DeleteProjectResponse{}, nil
}

func (f *Fs) ListProjects(ctx context.Context, req *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.ListProjects[Query]")

	projects, err := db.ListProjects(ctx, tx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS snapshot: %v", err)
	}

	return &pb.ListProjectsResponse{
		Projects: projects,
	}, nil
}

func validateObjectQuery(query *pb.ObjectQuery) error {
	if !query.IsPrefix && len(query.Ignores) > 0 {
		return status.Error(codes.InvalidArgument, "Invalid ObjectQuery: cannot mix unprefixed queries with ignore predicates")
	}

	for _, ignore := range query.Ignores {
		if !strings.HasPrefix(ignore, query.Path) {
			return status.Errorf(codes.InvalidArgument, "Invalid ObjectQuery: ignore pattern (%v) must fully include the path predicate (%v)", ignore, query.Path)
		}
	}

	return nil
}

func (f *Fs) Get(req *pb.GetRequest, stream pb.Fs_GetServer) error {
	ctx := stream.Context()

	project, err := requireProjectAuth(ctx)
	if err != nil {
		return err
	}

	if project > -1 && req.Project != project {
		return status.Errorf(codes.PermissionDenied, "Mismatch project authorization and request")
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	vrange, err := db.NewVersionRange(ctx, tx, req.Project, req.FromVersion, req.ToVersion)
	if errors.Is(err, db.ErrNotFound) {
		return status.Errorf(codes.NotFound, "FS get missing latest version: %v", err)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "FS get latest version: %v", err)
	}

	logger.Debug(ctx, "FS.Get[Init]", zap.Int64("project", req.Project), zap.Any("vrange", vrange))

	packManager, err := db.NewPackManager(ctx, tx, req.Project)
	if err != nil {
		return status.Errorf(codes.Internal, "FS create packed cache: %v", err)
	}

	for _, query := range req.Queries {
		err = validateObjectQuery(query)
		if err != nil {
			return err
		}

		logger.Debug(ctx, "FS.Get[Query]",
			zap.Int64("project", req.Project),
			zap.Any("vrange", vrange),
			zap.String("path", query.Path),
			zap.Bool("isPrefix", query.IsPrefix),
			zap.Bool("withContent", query.WithContent),
			zap.Strings("ignores", query.Ignores),
		)

		objects, err := db.GetObjects(ctx, tx, packManager, req.Project, vrange, query)
		if err != nil {
			return status.Errorf(codes.Internal, "FS get objects: %v", err)
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
				return status.Errorf(codes.Internal, "FS get next object: %v", err)
			}

			err = stream.Send(&pb.GetResponse{Version: vrange.To, Object: object})
			if err != nil {
				return status.Errorf(codes.Internal, "FS send GetResponse: %v", err)
			}
		}
	}

	return nil
}

func (f *Fs) GetCompress(req *pb.GetCompressRequest, stream pb.Fs_GetCompressServer) error {
	ctx := stream.Context()

	project, err := requireProjectAuth(ctx)
	if err != nil {
		return err
	}

	if project > -1 && req.Project != project {
		return status.Errorf(codes.PermissionDenied, "Mismatch project authorization and request")
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	vrange, err := db.NewVersionRange(ctx, tx, req.Project, req.FromVersion, req.ToVersion)
	if errors.Is(err, db.ErrNotFound) {
		return status.Errorf(codes.NotFound, "FS get compress missing latest version: %v", err)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "FS get compress latest version: %v", err)
	}

	logger.Debug(ctx, "FS.GetCompress[Init]", zap.Int64("project", req.Project), zap.Any("vrange", vrange))

	for _, query := range req.Queries {
		err = validateObjectQuery(query)
		if err != nil {
			return err
		}

		logger.Debug(ctx, "FS.GetCompress[Query]",
			zap.Int64("project", req.Project),
			zap.Any("vrange", vrange),
			zap.String("path", query.Path),
			zap.Bool("isPrefix", query.IsPrefix),
			zap.Bool("withContent", query.WithContent),
			zap.Strings("ignores", query.Ignores),
		)

		tars, err := db.GetTars(ctx, tx, req.Project, vrange, query)
		if err != nil {
			return status.Errorf(codes.Internal, "FS get tars: %v", err)
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
				return status.Errorf(codes.Internal, "FS get next tar: %v", err)
			}

			err = stream.Send(&pb.GetCompressResponse{
				Version: vrange.To,
				Format:  pb.GetCompressResponse_S2_TAR,
				Bytes:   tar,
			})
			if err != nil {
				return status.Errorf(codes.Internal, "FS send GetCompressResponse: %v", err)
			}
		}
	}

	return nil
}

func (f *Fs) Update(stream pb.Fs_UpdateServer) error {
	ctx := stream.Context()

	version := int64(-1)
	project, err := requireProjectAuth(ctx)
	if err != nil {
		return err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	contentEncoder := db.NewContentEncoder()

	var packManager *db.PackManager
	buffer := make(map[string][]*pb.Object)

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			errStatus, _ := status.FromError(err)
			return status.Errorf(errStatus.Code(), "FS receive update request: %v", err)
		}

		if version == -1 {
			if project == -1 {
				project = req.Project
			}

			latestVersion, err := db.LockLatestVersion(ctx, tx, project)
			if errors.Is(err, db.ErrNotFound) {
				return status.Errorf(codes.NotFound, "FS update missing latest version: %v", err)
			}
			if err != nil {
				return status.Errorf(codes.Internal, "FS update lock latest version: %v", err)
			}

			version = latestVersion + 1
			logger.Debug(ctx, "FS.Update[Init]", zap.Int64("project", project), zap.Int64("version", version))

			packManager, err = db.NewPackManager(ctx, tx, project)
			if err != nil {
				return status.Errorf(codes.Internal, "FS create packed cache: %v", err)
			}
		}

		if project != req.Project {
			return status.Errorf(codes.InvalidArgument, "initial project %v, next project %v: %v", project, req.Project, ErrMultipleProjectsPerUpdate)
		}

		packParent := packManager.IsPathPacked(req.Object.Path)
		if packParent != nil {
			buffer[*packParent] = append(buffer[*packParent], req.Object)
			continue
		}

		logger.Debug(ctx, "FS.Update[Object]", zap.Int64("project", project), zap.Int64("version", version), zap.String("path", req.Object.Path))

		if req.Object.Deleted {
			err = db.DeleteObject(ctx, tx, project, version, req.Object.Path)
		} else {
			err = db.UpdateObject(ctx, tx, contentEncoder, project, version, req.Object)
		}

		if err != nil {
			return status.Errorf(codes.Internal, "FS update: %v", err)
		}
	}

	// No updates were received from the stream which prevented us from detecting the project and version
	if version == -1 {
		err = tx.Rollback(ctx)
		if err != nil {
			return status.Errorf(codes.Internal, "FS rollback empty update: %v", err)
		}

		logger.Debug(ctx, "FS.Update[Empty]")
		return stream.SendAndClose(&pb.UpdateResponse{Version: -1})
	}

	for parent, objects := range buffer {
		logger.Debug(ctx, "FS.Update[PackedObject]", zap.Int64("project", project), zap.Int64("version", version), zap.String("parent", parent), zap.Int("object_count", len(objects)))

		err = db.UpdatePackedObjects(ctx, tx, project, version, parent, objects)
		if err != nil {
			return status.Errorf(codes.Internal, "FS update packed objects for %v: %v", parent, err)
		}
	}

	err = db.UpdateLatestVersion(ctx, tx, project, version)
	if err != nil {
		return status.Errorf(codes.Internal, "FS update latest version: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "FS update commit tx: %v", err)
	}

	logger.Debug(ctx, "FS.Update[Commit]", zap.Int64("project", project), zap.Int64("version", version))

	return stream.SendAndClose(&pb.UpdateResponse{Version: version})
}

func (f *Fs) Inspect(ctx context.Context, req *pb.InspectRequest) (*pb.InspectResponse, error) {
	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.Inspect[Query]", zap.Int64("project", req.Project))

	vrange, err := db.NewVersionRange(ctx, tx, req.Project, nil, nil)
	if errors.Is(err, db.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "FS inspect missing latest version: %v", err)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS inspect latest version: %v", err)
	}

	packManager, err := db.NewPackManager(ctx, tx, req.Project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS create packed cache: %v", err)
	}

	query := &pb.ObjectQuery{
		Path:        "",
		IsPrefix:    true,
		WithContent: false,
	}
	objects, err := db.GetObjects(ctx, tx, packManager, req.Project, vrange, query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS get objects: %v", err)
	}

	live_objects_count := int64(0)
	for {
		_, err := objects()
		if err == db.SKIP {
			continue
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, status.Errorf(codes.Internal, "FS get next object: %v", err)
		}

		live_objects_count += 1
	}

	total_objects_count, err := db.TotalObjectsCount(ctx, tx, req.Project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS inspect project: %v", err)
	}

	return &pb.InspectResponse{
		Project:           req.Project,
		LatestVersion:     vrange.To,
		LiveObjectsCount:  live_objects_count,
		TotalObjectsCount: total_objects_count,
	}, nil
}

func (f *Fs) Snapshot(ctx context.Context, req *pb.SnapshotRequest) (*pb.SnapshotResponse, error) {
	if f.Env != environment.Dev && f.Env != environment.Test {
		return nil, status.Errorf(codes.Unimplemented, "FS snapshot only implemented in dev and test environments")
	}

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.Snapshot[Query]")

	projects, err := db.ListProjects(ctx, tx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS snapshot: %v", err)
	}

	return &pb.SnapshotResponse{
		Projects: projects,
	}, nil
}

func (f *Fs) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	if f.Env != environment.Dev && f.Env != environment.Test {
		return nil, status.Errorf(codes.Unimplemented, "FS reset only implemented in dev and test environments")
	}

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.Reset[Init]")

	if len(req.Projects) == 0 {
		logger.Debug(ctx, "FS.Reset[All]")

		err = db.ResetAll(ctx, tx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "FS reset all: %v", err)
		}
	} else {
		var projects []int64

		for _, project := range req.Projects {
			logger.Debug(ctx, "FS.Reset[Project]", zap.Int64("project", project.Id), zap.Int64("version", project.Version))
			err = db.ResetProject(ctx, tx, project.Id, project.Version)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "FS reset project %v: %v", project.Id, err)
			}
			projects = append(projects, project.Id)
		}

		err = db.DropOtherProjects(ctx, tx, projects)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "FS reset drop others: %v", err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS reset commit tx: %v", err)
	}
	logger.Debug(ctx, "FS.Reset[Commit]")

	return &pb.ResetResponse{}, nil
}
