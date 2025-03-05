// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.29.3
// source: internal/pb/fs.proto

package pb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Fs_NewProject_FullMethodName       = "/pb.Fs/NewProject"
	Fs_DeleteProject_FullMethodName    = "/pb.Fs/DeleteProject"
	Fs_ListProjects_FullMethodName     = "/pb.Fs/ListProjects"
	Fs_Get_FullMethodName              = "/pb.Fs/Get"
	Fs_GetCompress_FullMethodName      = "/pb.Fs/GetCompress"
	Fs_GetUnary_FullMethodName         = "/pb.Fs/GetUnary"
	Fs_Update_FullMethodName           = "/pb.Fs/Update"
	Fs_Rollback_FullMethodName         = "/pb.Fs/Rollback"
	Fs_Inspect_FullMethodName          = "/pb.Fs/Inspect"
	Fs_Snapshot_FullMethodName         = "/pb.Fs/Snapshot"
	Fs_Reset_FullMethodName            = "/pb.Fs/Reset"
	Fs_GcProject_FullMethodName        = "/pb.Fs/GcProject"
	Fs_GcRandomProjects_FullMethodName = "/pb.Fs/GcRandomProjects"
	Fs_GcContents_FullMethodName       = "/pb.Fs/GcContents"
	Fs_CloneToProject_FullMethodName   = "/pb.Fs/CloneToProject"
	Fs_GetCache_FullMethodName         = "/pb.Fs/GetCache"
)

// FsClient is the client API for Fs service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FsClient interface {
	NewProject(ctx context.Context, in *NewProjectRequest, opts ...grpc.CallOption) (*NewProjectResponse, error)
	DeleteProject(ctx context.Context, in *DeleteProjectRequest, opts ...grpc.CallOption) (*DeleteProjectResponse, error)
	ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error)
	Get(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[GetResponse], error)
	GetCompress(ctx context.Context, in *GetCompressRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[GetCompressResponse], error)
	GetUnary(ctx context.Context, in *GetUnaryRequest, opts ...grpc.CallOption) (*GetUnaryResponse, error)
	Update(ctx context.Context, opts ...grpc.CallOption) (grpc.ClientStreamingClient[UpdateRequest, UpdateResponse], error)
	Rollback(ctx context.Context, in *RollbackRequest, opts ...grpc.CallOption) (*RollbackResponse, error)
	Inspect(ctx context.Context, in *InspectRequest, opts ...grpc.CallOption) (*InspectResponse, error)
	Snapshot(ctx context.Context, in *SnapshotRequest, opts ...grpc.CallOption) (*SnapshotResponse, error)
	Reset(ctx context.Context, in *ResetRequest, opts ...grpc.CallOption) (*ResetResponse, error)
	GcProject(ctx context.Context, in *GcProjectRequest, opts ...grpc.CallOption) (*GcProjectResponse, error)
	GcRandomProjects(ctx context.Context, in *GcRandomProjectsRequest, opts ...grpc.CallOption) (*GcRandomProjectsResponse, error)
	GcContents(ctx context.Context, in *GcContentsRequest, opts ...grpc.CallOption) (*GcContentsResponse, error)
	CloneToProject(ctx context.Context, in *CloneToProjectRequest, opts ...grpc.CallOption) (*CloneToProjectResponse, error)
	GetCache(ctx context.Context, in *GetCacheRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[GetCacheResponse], error)
}

type fsClient struct {
	cc grpc.ClientConnInterface
}

func NewFsClient(cc grpc.ClientConnInterface) FsClient {
	return &fsClient{cc}
}

func (c *fsClient) NewProject(ctx context.Context, in *NewProjectRequest, opts ...grpc.CallOption) (*NewProjectResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(NewProjectResponse)
	err := c.cc.Invoke(ctx, Fs_NewProject_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) DeleteProject(ctx context.Context, in *DeleteProjectRequest, opts ...grpc.CallOption) (*DeleteProjectResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(DeleteProjectResponse)
	err := c.cc.Invoke(ctx, Fs_DeleteProject_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListProjectsResponse)
	err := c.cc.Invoke(ctx, Fs_ListProjects_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Get(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[GetResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[0], Fs_Get_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[GetRequest, GetResponse]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_GetClient = grpc.ServerStreamingClient[GetResponse]

func (c *fsClient) GetCompress(ctx context.Context, in *GetCompressRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[GetCompressResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[1], Fs_GetCompress_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[GetCompressRequest, GetCompressResponse]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_GetCompressClient = grpc.ServerStreamingClient[GetCompressResponse]

func (c *fsClient) GetUnary(ctx context.Context, in *GetUnaryRequest, opts ...grpc.CallOption) (*GetUnaryResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetUnaryResponse)
	err := c.cc.Invoke(ctx, Fs_GetUnary_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Update(ctx context.Context, opts ...grpc.CallOption) (grpc.ClientStreamingClient[UpdateRequest, UpdateResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[2], Fs_Update_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[UpdateRequest, UpdateResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_UpdateClient = grpc.ClientStreamingClient[UpdateRequest, UpdateResponse]

func (c *fsClient) Rollback(ctx context.Context, in *RollbackRequest, opts ...grpc.CallOption) (*RollbackResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(RollbackResponse)
	err := c.cc.Invoke(ctx, Fs_Rollback_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Inspect(ctx context.Context, in *InspectRequest, opts ...grpc.CallOption) (*InspectResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InspectResponse)
	err := c.cc.Invoke(ctx, Fs_Inspect_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Snapshot(ctx context.Context, in *SnapshotRequest, opts ...grpc.CallOption) (*SnapshotResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SnapshotResponse)
	err := c.cc.Invoke(ctx, Fs_Snapshot_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Reset(ctx context.Context, in *ResetRequest, opts ...grpc.CallOption) (*ResetResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ResetResponse)
	err := c.cc.Invoke(ctx, Fs_Reset_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GcProject(ctx context.Context, in *GcProjectRequest, opts ...grpc.CallOption) (*GcProjectResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GcProjectResponse)
	err := c.cc.Invoke(ctx, Fs_GcProject_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GcRandomProjects(ctx context.Context, in *GcRandomProjectsRequest, opts ...grpc.CallOption) (*GcRandomProjectsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GcRandomProjectsResponse)
	err := c.cc.Invoke(ctx, Fs_GcRandomProjects_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GcContents(ctx context.Context, in *GcContentsRequest, opts ...grpc.CallOption) (*GcContentsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GcContentsResponse)
	err := c.cc.Invoke(ctx, Fs_GcContents_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) CloneToProject(ctx context.Context, in *CloneToProjectRequest, opts ...grpc.CallOption) (*CloneToProjectResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CloneToProjectResponse)
	err := c.cc.Invoke(ctx, Fs_CloneToProject_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GetCache(ctx context.Context, in *GetCacheRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[GetCacheResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[3], Fs_GetCache_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[GetCacheRequest, GetCacheResponse]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_GetCacheClient = grpc.ServerStreamingClient[GetCacheResponse]

// FsServer is the server API for Fs service.
// All implementations must embed UnimplementedFsServer
// for forward compatibility.
type FsServer interface {
	NewProject(context.Context, *NewProjectRequest) (*NewProjectResponse, error)
	DeleteProject(context.Context, *DeleteProjectRequest) (*DeleteProjectResponse, error)
	ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error)
	Get(*GetRequest, grpc.ServerStreamingServer[GetResponse]) error
	GetCompress(*GetCompressRequest, grpc.ServerStreamingServer[GetCompressResponse]) error
	GetUnary(context.Context, *GetUnaryRequest) (*GetUnaryResponse, error)
	Update(grpc.ClientStreamingServer[UpdateRequest, UpdateResponse]) error
	Rollback(context.Context, *RollbackRequest) (*RollbackResponse, error)
	Inspect(context.Context, *InspectRequest) (*InspectResponse, error)
	Snapshot(context.Context, *SnapshotRequest) (*SnapshotResponse, error)
	Reset(context.Context, *ResetRequest) (*ResetResponse, error)
	GcProject(context.Context, *GcProjectRequest) (*GcProjectResponse, error)
	GcRandomProjects(context.Context, *GcRandomProjectsRequest) (*GcRandomProjectsResponse, error)
	GcContents(context.Context, *GcContentsRequest) (*GcContentsResponse, error)
	CloneToProject(context.Context, *CloneToProjectRequest) (*CloneToProjectResponse, error)
	GetCache(*GetCacheRequest, grpc.ServerStreamingServer[GetCacheResponse]) error
	mustEmbedUnimplementedFsServer()
}

// UnimplementedFsServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedFsServer struct{}

func (UnimplementedFsServer) NewProject(context.Context, *NewProjectRequest) (*NewProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NewProject not implemented")
}
func (UnimplementedFsServer) DeleteProject(context.Context, *DeleteProjectRequest) (*DeleteProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteProject not implemented")
}
func (UnimplementedFsServer) ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListProjects not implemented")
}
func (UnimplementedFsServer) Get(*GetRequest, grpc.ServerStreamingServer[GetResponse]) error {
	return status.Errorf(codes.Unimplemented, "method Get not implemented")
}
func (UnimplementedFsServer) GetCompress(*GetCompressRequest, grpc.ServerStreamingServer[GetCompressResponse]) error {
	return status.Errorf(codes.Unimplemented, "method GetCompress not implemented")
}
func (UnimplementedFsServer) GetUnary(context.Context, *GetUnaryRequest) (*GetUnaryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetUnary not implemented")
}
func (UnimplementedFsServer) Update(grpc.ClientStreamingServer[UpdateRequest, UpdateResponse]) error {
	return status.Errorf(codes.Unimplemented, "method Update not implemented")
}
func (UnimplementedFsServer) Rollback(context.Context, *RollbackRequest) (*RollbackResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Rollback not implemented")
}
func (UnimplementedFsServer) Inspect(context.Context, *InspectRequest) (*InspectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Inspect not implemented")
}
func (UnimplementedFsServer) Snapshot(context.Context, *SnapshotRequest) (*SnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Snapshot not implemented")
}
func (UnimplementedFsServer) Reset(context.Context, *ResetRequest) (*ResetResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Reset not implemented")
}
func (UnimplementedFsServer) GcProject(context.Context, *GcProjectRequest) (*GcProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GcProject not implemented")
}
func (UnimplementedFsServer) GcRandomProjects(context.Context, *GcRandomProjectsRequest) (*GcRandomProjectsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GcRandomProjects not implemented")
}
func (UnimplementedFsServer) GcContents(context.Context, *GcContentsRequest) (*GcContentsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GcContents not implemented")
}
func (UnimplementedFsServer) CloneToProject(context.Context, *CloneToProjectRequest) (*CloneToProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CloneToProject not implemented")
}
func (UnimplementedFsServer) GetCache(*GetCacheRequest, grpc.ServerStreamingServer[GetCacheResponse]) error {
	return status.Errorf(codes.Unimplemented, "method GetCache not implemented")
}
func (UnimplementedFsServer) mustEmbedUnimplementedFsServer() {}
func (UnimplementedFsServer) testEmbeddedByValue()            {}

// UnsafeFsServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FsServer will
// result in compilation errors.
type UnsafeFsServer interface {
	mustEmbedUnimplementedFsServer()
}

func RegisterFsServer(s grpc.ServiceRegistrar, srv FsServer) {
	// If the following call pancis, it indicates UnimplementedFsServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Fs_ServiceDesc, srv)
}

func _Fs_NewProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(NewProjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).NewProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_NewProject_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).NewProject(ctx, req.(*NewProjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_DeleteProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteProjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).DeleteProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_DeleteProject_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).DeleteProject(ctx, req.(*DeleteProjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_ListProjects_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListProjectsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).ListProjects(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_ListProjects_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).ListProjects(ctx, req.(*ListProjectsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_Get_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(FsServer).Get(m, &grpc.GenericServerStream[GetRequest, GetResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_GetServer = grpc.ServerStreamingServer[GetResponse]

func _Fs_GetCompress_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetCompressRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(FsServer).GetCompress(m, &grpc.GenericServerStream[GetCompressRequest, GetCompressResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_GetCompressServer = grpc.ServerStreamingServer[GetCompressResponse]

func _Fs_GetUnary_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetUnaryRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).GetUnary(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_GetUnary_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).GetUnary(ctx, req.(*GetUnaryRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_Update_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FsServer).Update(&grpc.GenericServerStream[UpdateRequest, UpdateResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_UpdateServer = grpc.ClientStreamingServer[UpdateRequest, UpdateResponse]

func _Fs_Rollback_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RollbackRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).Rollback(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_Rollback_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).Rollback(ctx, req.(*RollbackRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_Inspect_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InspectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).Inspect(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_Inspect_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).Inspect(ctx, req.(*InspectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_Snapshot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SnapshotRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).Snapshot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_Snapshot_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).Snapshot(ctx, req.(*SnapshotRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_Reset_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ResetRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).Reset(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_Reset_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).Reset(ctx, req.(*ResetRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_GcProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GcProjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).GcProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_GcProject_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).GcProject(ctx, req.(*GcProjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_GcRandomProjects_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GcRandomProjectsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).GcRandomProjects(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_GcRandomProjects_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).GcRandomProjects(ctx, req.(*GcRandomProjectsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_GcContents_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GcContentsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).GcContents(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_GcContents_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).GcContents(ctx, req.(*GcContentsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_CloneToProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CloneToProjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).CloneToProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Fs_CloneToProject_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).CloneToProject(ctx, req.(*CloneToProjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Fs_GetCache_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetCacheRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(FsServer).GetCache(m, &grpc.GenericServerStream[GetCacheRequest, GetCacheResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Fs_GetCacheServer = grpc.ServerStreamingServer[GetCacheResponse]

// Fs_ServiceDesc is the grpc.ServiceDesc for Fs service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Fs_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "pb.Fs",
	HandlerType: (*FsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "NewProject",
			Handler:    _Fs_NewProject_Handler,
		},
		{
			MethodName: "DeleteProject",
			Handler:    _Fs_DeleteProject_Handler,
		},
		{
			MethodName: "ListProjects",
			Handler:    _Fs_ListProjects_Handler,
		},
		{
			MethodName: "GetUnary",
			Handler:    _Fs_GetUnary_Handler,
		},
		{
			MethodName: "Rollback",
			Handler:    _Fs_Rollback_Handler,
		},
		{
			MethodName: "Inspect",
			Handler:    _Fs_Inspect_Handler,
		},
		{
			MethodName: "Snapshot",
			Handler:    _Fs_Snapshot_Handler,
		},
		{
			MethodName: "Reset",
			Handler:    _Fs_Reset_Handler,
		},
		{
			MethodName: "GcProject",
			Handler:    _Fs_GcProject_Handler,
		},
		{
			MethodName: "GcRandomProjects",
			Handler:    _Fs_GcRandomProjects_Handler,
		},
		{
			MethodName: "GcContents",
			Handler:    _Fs_GcContents_Handler,
		},
		{
			MethodName: "CloneToProject",
			Handler:    _Fs_CloneToProject_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Get",
			Handler:       _Fs_Get_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "GetCompress",
			Handler:       _Fs_GetCompress_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "Update",
			Handler:       _Fs_Update_Handler,
			ClientStreams: true,
		},
		{
			StreamName:    "GetCache",
			Handler:       _Fs_GetCache_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "internal/pb/fs.proto",
}
