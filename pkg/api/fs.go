package api

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrMultipleProjectsPerUpdate = errors.New("multiple objects in one update")

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

func requireSharedReaderAuth(ctx context.Context) error {
	ctxAuth := ctx.Value(auth.AuthCtxKey).(auth.Auth)

	if ctxAuth.Role == auth.Admin {
		return nil
	}

	if ctxAuth.Role == auth.Project {
		return nil
	}

	if ctxAuth.Role == auth.SharedReader {
		return nil
	}

	return status.Errorf(codes.PermissionDenied, "FS endpoint requires shared reader access")
}

type Fs struct {
	pb.UnimplementedFsServer

	Env           environment.Env
	DbConn        db.DbConnector
	ContentLookup *db.ContentLookup
}

func (f *Fs) NewProject(ctx context.Context, req *pb.NewProjectRequest) (*pb.NewProjectResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Id),
		key.Template.Attribute(req.Template),
		key.PackPatterns.Attribute(req.PackPatterns),
	)

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.NewProject[Init]",
		key.Project.Field(req.Id),
		key.Template.Field(req.Template),
	)

	err = db.CreateProject(ctx, tx, req.Id, req.PackPatterns)
	if err != nil {
		rpcErrorCode := codes.Internal
		if err.Error() == "project id already exists" {
			rpcErrorCode = codes.AlreadyExists
		}
		return nil, status.Errorf(rpcErrorCode, "FS new project %v, %v", req.Id, err)
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

	logger.Debug(ctx, "FS.NewProject[Commit]",
		key.Project.Field(req.Id),
		key.Template.Field(req.Template),
	)

	return &pb.NewProjectResponse{}, nil
}

func (f *Fs) CloneToProject(ctx context.Context, req *pb.CloneToProjectRequest) (*pb.CloneToProjectResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Source),
		key.Version.Attribute(req.Version),
		key.CloneToProject.Attribute(req.Target),
	)

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.CloneToProject[Init]", key.Project.Field(req.Source))

	samePackPatterns, err := db.HasSamePackPattern(ctx, tx, req.Source, req.Target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS verifying pack patterns: %v", err)
	}

	if !samePackPatterns {
		return nil, status.Errorf(codes.InvalidArgument, "FS projects have different pack patterns: %v", err)
	}

	latestVersion, err := db.LockLatestVersion(ctx, tx, req.Target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS copy to project could not lock target (%d) version: %v", req.Target, err)
	}

	newVersion := latestVersion + 1

	logger.Debug(ctx, "FS.CloneToProject[Query]", key.Project.Field(req.Source), key.CloneToProject.Field(req.Target), key.Version.Field(req.Version))

	err = db.CloneToProject(ctx, tx, req.Source, req.Target, req.Version, newVersion)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS copy to project from source (%d) to target (%d) with version (%d): %v", req.Source, req.Target, req.Version, err)
	}

	err = db.UpdateLatestVersion(ctx, tx, req.Target, newVersion)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS copy to project could not update target (%d) to latest version (%d): %v", req.Target, newVersion, err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS clone commit tx: %v", err)
	}
	logger.Debug(ctx, "FS.CloneToProject[Commit]")

	return &pb.CloneToProjectResponse{
		LatestVersion: newVersion,
	}, nil
}

func (f *Fs) DeleteProject(ctx context.Context, req *pb.DeleteProjectRequest) (*pb.DeleteProjectResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Project),
	)

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.DeleteProject[Init]", key.Project.Field(req.Project))
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
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Project),
		key.FromVersion.Attribute(req.FromVersion),
		key.ToVersion.Attribute(req.ToVersion),
	)

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

	logger.Debug(ctx, "FS.Get[Init]",
		key.Project.Field(req.Project),
		key.FromVersion.Field(&vrange.From),
		key.ToVersion.Field(&vrange.To),
	)

	packManager, err := db.NewPackManager(ctx, tx, req.Project)
	if err != nil {
		return status.Errorf(codes.Internal, "FS create packed cache: %v", err)
	}

	for _, query := range req.Queries {
		err = validateObjectQuery(query)
		if err != nil {
			return err
		}

		logger.Info(ctx, "FS.Get[Query]",
			key.Project.Field(req.Project),
			key.FromVersion.Field(&vrange.From),
			key.ToVersion.Field(&vrange.To),
			key.QueryPath.Field(query.Path),
			key.QueryIsPrefix.Field(query.IsPrefix),
			key.QueryIgnores.Field(query.Ignores),
		)

		objects, err := db.GetObjects(ctx, tx, f.ContentLookup, packManager, req.Project, vrange, query, -1)
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
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Project),
		key.FromVersion.Attribute(req.FromVersion),
		key.ToVersion.Attribute(req.ToVersion),
	)

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

	logger.Debug(ctx, "FS.GetCompress[Init]",
		key.Project.Field(req.Project),
		key.FromVersion.Field(&vrange.From),
		key.ToVersion.Field(&vrange.To),
		key.CacheVersions.Field(req.AvailableCacheVersions),
	)

	for _, query := range req.Queries {
		err = validateObjectQuery(query)
		if err != nil {
			return err
		}

		logger.Info(ctx, "FS.GetCompress[Query]",
			key.Project.Field(req.Project),
			key.FromVersion.Field(&vrange.From),
			key.ToVersion.Field(&vrange.To),
			key.QueryPath.Field(query.Path),
			key.QueryIsPrefix.Field(query.IsPrefix),
			key.QueryIgnores.Field(query.Ignores),
		)

		tars, err := db.GetTars(ctx, tx, f.ContentLookup, req.Project, req.AvailableCacheVersions, vrange, query)
		if err != nil {
			return status.Errorf(codes.Internal, "FS get tars: %v", err)
		}

		for {
			tar, packPath, err := tars()
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
				Version:  vrange.To,
				Format:   pb.GetCompressResponse_S2_TAR,
				Bytes:    tar,
				PackPath: packPath,
			})
			if err != nil {
				return status.Errorf(codes.Internal, "FS send GetCompressResponse: %v", err)
			}
		}
	}

	return nil
}

func (f *Fs) GetUnary(ctx context.Context, req *pb.GetUnaryRequest) (*pb.GetUnaryResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Project),
		key.FromVersion.Attribute(req.FromVersion),
		key.ToVersion.Attribute(req.ToVersion),
	)

	var maxContentSize int64
	if req.MaxContentSendSize != nil {
		maxContentSize = *req.MaxContentSendSize
	} else {
		maxContentSize = -1
	}

	trace.SpanFromContext(ctx).SetAttributes(
		key.MaxContentSendSize.Attribute(maxContentSize),
	)

	project, err := requireProjectAuth(ctx)
	if err != nil {
		return nil, err
	}

	if project > -1 && req.Project != project {
		return nil, status.Errorf(codes.PermissionDenied, "Mismatch project authorization and request")
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	vrange, err := db.NewVersionRange(ctx, tx, req.Project, req.FromVersion, req.ToVersion)
	if errors.Is(err, db.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "FS get missing latest version: %v", err)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS get latest version: %v", err)
	}

	logger.Debug(ctx, "FS.GetUnary[Init]",
		key.Project.Field(req.Project),
		key.FromVersion.Field(&vrange.From),
		key.ToVersion.Field(&vrange.To),
	)

	packManager, err := db.NewPackManager(ctx, tx, req.Project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS create packed cache: %v", err)
	}

	var response pb.GetUnaryResponse

	for _, query := range req.Queries {
		err = validateObjectQuery(query)
		if err != nil {
			return nil, err
		}

		logger.Info(ctx, "FS.GetUnary[Query]",
			key.Project.Field(req.Project),
			key.FromVersion.Field(&vrange.From),
			key.ToVersion.Field(&vrange.To),
			key.QueryPath.Field(query.Path),
			key.QueryIsPrefix.Field(query.IsPrefix),
			key.QueryIgnores.Field(query.Ignores),
		)

		objects, err := db.GetObjects(ctx, tx, f.ContentLookup, packManager, req.Project, vrange, query, maxContentSize)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "FS get objects: %v", err)
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
				return nil, status.Errorf(codes.Internal, "FS get next object: %v", err)
			}

			response.Version = vrange.To
			response.Objects = append(response.Objects, object)
		}
	}

	return &response, nil
}

//nolint:gocyclo // we should try to break this one up, but ignoring for now
func (f *Fs) Update(stream pb.Fs_UpdateServer) error {
	ctx := stream.Context()

	project, err := requireProjectAuth(ctx)
	if err != nil {
		return err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	var packManager *db.PackManager

	var objectBuffer []*pb.Object
	packedBuffer := make(map[string][]*pb.Object)

	err = telemetry.Trace(ctx, "receive-update-objects", func(ctx context.Context, span trace.Span) error {
		for {
			req, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				errStatus, _ := status.FromError(err)
				return status.Errorf(errStatus.Code(), "FS receive update request: %v", err)
			}

			// project can be -1 if the client authenticated using an Admin token
			if project == -1 {
				project = req.Project
			}
			if project != req.Project {
				return status.Errorf(codes.InvalidArgument, "initial project %v, next project %v: %v", project, req.Project, ErrMultipleProjectsPerUpdate)
			}

			// We can only create the pack manager once we have the project ID and that requires a least one stream message
			if packManager == nil {
				packManager, err = db.NewPackManager(ctx, tx, project)
				if err != nil {
					return status.Errorf(codes.Internal, "FS create packed cache: %v", err)
				}

				span.SetAttributes(
					key.Project.Attribute(project),
				)
			}

			packParent := packManager.IsPathPacked(req.Object.Path)
			if packParent != nil {
				packedBuffer[*packParent] = append(packedBuffer[*packParent], req.Object)
				continue
			}

			objectBuffer = append(objectBuffer, req.Object)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// No updates were received from the stream which prevented us from detecting the project and version
	if len(objectBuffer) == 0 && len(packedBuffer) == 0 {
		logger.Info(ctx, "FS.Update[Empty]")
		return stream.SendAndClose(&pb.UpdateResponse{Version: -1})
	}

	contentEncoder := db.NewContentEncoder()
	defer contentEncoder.Close()

	latestVersion := int64(-1)
	nextVersion := int64(-1)
	shouldUpdateVersion := false

	err = telemetry.Trace(ctx, "update-objects", func(ctx context.Context, span trace.Span) error {
		latestVersion, err = db.LockLatestVersion(ctx, tx, project)
		if errors.Is(err, db.ErrNotFound) {
			return status.Errorf(codes.NotFound, "FS update missing latest version: %v", err)
		}
		if err != nil {
			return status.Errorf(codes.Internal, "FS update lock latest version: %v", err)
		}

		nextVersion = latestVersion + 1
		logger.Info(ctx, "FS.Update[Init]", key.Project.Field(project), key.Version.Field(nextVersion))

		var deletedPaths []string
		var objectsToUpdate []*pb.Object
		for _, object := range objectBuffer {
			if object.Deleted {
				logger.Debug(ctx, "FS.Delete[Object]",
					key.Project.Field(project),
					key.Version.Field(nextVersion),
					key.ObjectPath.Field(object.Path),
				)
				deletedPaths = append(deletedPaths, object.Path)
			} else {
				logger.Debug(ctx, "FS.Update[Object]",
					key.Project.Field(project),
					key.Version.Field(nextVersion),
					key.ObjectPath.Field(object.Path),
				)
				objectsToUpdate = append(objectsToUpdate, object)
			}
		}

		if len(deletedPaths) > 0 {
			err = db.DeleteObjects(ctx, tx, project, nextVersion, deletedPaths)
			if err != nil {
				return status.Errorf(codes.Internal, "FS update: %v", err)
			}
			shouldUpdateVersion = true
		}

		if len(objectsToUpdate) > 0 {
			contentChanged, err := db.UpdateObjects(ctx, tx, f.DbConn, contentEncoder, project, nextVersion, objectsToUpdate)
			if err != nil {
				return status.Errorf(codes.Internal, "FS update: %v", err)
			}

			if contentChanged {
				shouldUpdateVersion = true
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	err = telemetry.Trace(ctx, "update-packed-objects", func(ctx context.Context, span trace.Span) error {
		for parent, objects := range packedBuffer {
			logger.Debug(ctx, "FS.Update[PackedObject]",
				key.Project.Field(project),
				key.Version.Field(nextVersion),
				key.ObjectsParent.Field(parent),
				key.ObjectsCount.Field(len(objects)),
			)

			contentChanged, err := db.UpdatePackedObjects(ctx, tx, f.DbConn, project, nextVersion, parent, objects)
			if err != nil {
				return status.Errorf(codes.Internal, "FS update packed objects for %v: %v", parent, err)
			}

			if contentChanged {
				shouldUpdateVersion = true
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if !shouldUpdateVersion {
		return stream.SendAndClose(&pb.UpdateResponse{Version: latestVersion})
	}

	err = db.UpdateLatestVersion(ctx, tx, project, nextVersion)
	if err != nil {
		return status.Errorf(codes.Internal, "FS update latest version: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "FS update commit tx: %v", err)
	}

	logger.Debug(ctx, "FS.Update[Commit]", key.Project.Field(project), key.Version.Field(nextVersion))

	return stream.SendAndClose(&pb.UpdateResponse{Version: nextVersion})
}

func (f *Fs) Rollback(ctx context.Context, req *pb.RollbackRequest) (*pb.RollbackResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Project),
	)

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.Rollback[Project]", key.Project.Field(req.Project), key.Version.Field(req.Version))

	err = db.ResetProject(ctx, tx, req.Project, req.Version)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS rollback project %v: %v", req.Project, err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS rollback commit tx: %v", err)
	}
	logger.Debug(ctx, "FS.Rollback[Commit]")

	return &pb.RollbackResponse{}, nil
}

func (f *Fs) Inspect(ctx context.Context, req *pb.InspectRequest) (*pb.InspectResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Project),
	)

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.Inspect[Query]", key.Project.Field(req.Project))

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
		Path:     "",
		IsPrefix: true,
	}
	objects, err := db.GetObjects(ctx, tx, f.ContentLookup, packManager, req.Project, vrange, query, -1)
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
			logger.Debug(ctx, "FS.Reset[Project]", key.Project.Field(project.Id), key.Version.Field(project.Version))
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

func (f *Fs) GcProject(ctx context.Context, req *pb.GcProjectRequest) (*pb.GcProjectResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.Project.Attribute(req.Project),
		key.KeepVersions.Attribute(req.KeepVersions),
		key.FromVersion.Attribute(req.FromVersion),
	)

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.KeepVersions <= 0 {
		return nil, status.Error(codes.InvalidArgument, "Invalid GC KeepVersions: cannot keep 0 versions")
	}

	logger.Debug(ctx, "FS.GcProject[Init]", key.Project.Field(req.Project))

	fromVersion := int64(0)
	if req.FromVersion != nil {
		fromVersion = *req.FromVersion
	}

	hashes, err := db.GcProjectObjects(ctx, f.DbConn, req.Project, req.KeepVersions, fromVersion)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS gc project objects %v: %v", req.Project, err)
	}

	count, err := db.GcContentHashes(ctx, f.DbConn, hashes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS gc content hashes %v: %v", req.Project, err)
	}

	return &pb.GcProjectResponse{
		Count:   count,
		Project: req.Project,
	}, nil
}

func (f *Fs) GcRandomProjects(ctx context.Context, req *pb.GcRandomProjectsRequest) (*pb.GcRandomProjectsResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.SampleRate.Attribute(req.Sample),
		key.KeepVersions.Attribute(req.KeepVersions),
		key.FromVersion.Attribute(req.FromVersion),
	)

	ctx, span := telemetry.Start(ctx, "fs.gc-random-projects")
	defer span.End()

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.KeepVersions <= 0 {
		return nil, status.Error(codes.InvalidArgument, "Invalid GC KeepVersions: cannot keep 0 versions")
	}

	logger.Debug(ctx, "FS.GcRandomProjects[Init]", key.SampleRate.Field(req.Sample))

	fromVersion := int64(0)
	if req.FromVersion != nil {
		fromVersion = *req.FromVersion
	}

	projects, err := db.RandomProjects(ctx, f.DbConn, req.Sample)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS gc random projects %f: %v", req.Sample, err)
	}

	hashes, err := db.GcProjectsObjects(ctx, f.DbConn, projects, req.KeepVersions, fromVersion)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS gc random project objects: %v", err)
	}

	count, err := db.GcContentHashes(ctx, f.DbConn, hashes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS gc random content hashes: %v", err)
	}

	return &pb.GcRandomProjectsResponse{
		Count:    count,
		Projects: projects,
	}, nil
}

func (f *Fs) GcContents(ctx context.Context, req *pb.GcContentsRequest) (*pb.GcContentsResponse, error) {
	trace.SpanFromContext(ctx).SetAttributes(
		key.SampleRate.Attribute(req.Sample),
	)

	err := requireAdminAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger.Debug(ctx, "FS.GcContents[Init]")

	hashes, err := db.RandomContents(ctx, f.DbConn, req.Sample)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS gc random contents %f: %v", req.Sample, err)
	}

	count, err := db.GcContentHashes(ctx, f.DbConn, hashes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FS gc random content hashes: %v", err)
	}

	return &pb.GcContentsResponse{
		Count: count,
	}, nil
}

func (f *Fs) GetCache(req *pb.GetCacheRequest, stream pb.Fs_GetCacheServer) error {
	cacheVersion := int64(-1)
	if req.Version != nil {
		cacheVersion = *req.Version
	}

	ctx := stream.Context()
	trace.SpanFromContext(ctx).SetAttributes(key.CacheVersion.Attribute(cacheVersion))

	err := requireSharedReaderAuth(ctx)
	if err != nil {
		return err
	}

	tx, close, err := f.DbConn.Connect(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "FS db connection unavailable: %v", err)
	}
	defer close(ctx)

	logger.Debug(ctx, "FS.GetCache[Init]", key.CacheVersion.Field(cacheVersion))

	tars, closeFunc, err := db.GetCacheTars(ctx, tx, cacheVersion)
	defer closeFunc(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "FS get cached tars: %v", err)
	}

	for {
		version, tar, hash, err := tars()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "FS get next tar: %v", err)
		}

		err = stream.Send(&pb.GetCacheResponse{
			Version: version,
			Format:  pb.GetCacheResponse_S2_TAR,
			Bytes:   tar,
			Hash:    hash.Bytes(),
		})
		if err != nil {
			return status.Errorf(codes.Internal, "FS send GetCacheResponse: %v", err)
		}
	}

	return nil
}
