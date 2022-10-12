// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.19.4
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
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// FsClient is the client API for Fs service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FsClient interface {
	NewProject(ctx context.Context, in *NewProjectRequest, opts ...grpc.CallOption) (*NewProjectResponse, error)
	DeleteProject(ctx context.Context, in *DeleteProjectRequest, opts ...grpc.CallOption) (*DeleteProjectResponse, error)
	ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error)
	Get(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (Fs_GetClient, error)
	GetCompress(ctx context.Context, in *GetCompressRequest, opts ...grpc.CallOption) (Fs_GetCompressClient, error)
	Update(ctx context.Context, opts ...grpc.CallOption) (Fs_UpdateClient, error)
	Inspect(ctx context.Context, in *InspectRequest, opts ...grpc.CallOption) (*InspectResponse, error)
	Snapshot(ctx context.Context, in *SnapshotRequest, opts ...grpc.CallOption) (*SnapshotResponse, error)
	Reset(ctx context.Context, in *ResetRequest, opts ...grpc.CallOption) (*ResetResponse, error)
	GcProject(ctx context.Context, in *GcProjectRequest, opts ...grpc.CallOption) (*GcProjectResponse, error)
	GcRandomProjects(ctx context.Context, in *GcRandomProjectsRequest, opts ...grpc.CallOption) (*GcRandomProjectsResponse, error)
	GcContents(ctx context.Context, in *GcContentsRequest, opts ...grpc.CallOption) (*GcContentsResponse, error)
	CloneToProject(ctx context.Context, in *CloneToProjectRequest, opts ...grpc.CallOption) (*CloneToProjectResponse, error)
	GetCache(ctx context.Context, in *GetCacheRequest, opts ...grpc.CallOption) (Fs_GetCacheClient, error)
	CreateCache(ctx context.Context, in *CreateCacheRequest, opts ...grpc.CallOption) (*CreateCacheResponse, error)
}

type fsClient struct {
	cc grpc.ClientConnInterface
}

func NewFsClient(cc grpc.ClientConnInterface) FsClient {
	return &fsClient{cc}
}

func (c *fsClient) NewProject(ctx context.Context, in *NewProjectRequest, opts ...grpc.CallOption) (*NewProjectResponse, error) {
	out := new(NewProjectResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/NewProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) DeleteProject(ctx context.Context, in *DeleteProjectRequest, opts ...grpc.CallOption) (*DeleteProjectResponse, error) {
	out := new(DeleteProjectResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/DeleteProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error) {
	out := new(ListProjectsResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/ListProjects", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Get(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (Fs_GetClient, error) {
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[0], "/pb.Fs/Get", opts...)
	if err != nil {
		return nil, err
	}
	x := &fsGetClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Fs_GetClient interface {
	Recv() (*GetResponse, error)
	grpc.ClientStream
}

type fsGetClient struct {
	grpc.ClientStream
}

func (x *fsGetClient) Recv() (*GetResponse, error) {
	m := new(GetResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *fsClient) GetCompress(ctx context.Context, in *GetCompressRequest, opts ...grpc.CallOption) (Fs_GetCompressClient, error) {
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[1], "/pb.Fs/GetCompress", opts...)
	if err != nil {
		return nil, err
	}
	x := &fsGetCompressClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Fs_GetCompressClient interface {
	Recv() (*GetCompressResponse, error)
	grpc.ClientStream
}

type fsGetCompressClient struct {
	grpc.ClientStream
}

func (x *fsGetCompressClient) Recv() (*GetCompressResponse, error) {
	m := new(GetCompressResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *fsClient) Update(ctx context.Context, opts ...grpc.CallOption) (Fs_UpdateClient, error) {
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[2], "/pb.Fs/Update", opts...)
	if err != nil {
		return nil, err
	}
	x := &fsUpdateClient{stream}
	return x, nil
}

type Fs_UpdateClient interface {
	Send(*UpdateRequest) error
	CloseAndRecv() (*UpdateResponse, error)
	grpc.ClientStream
}

type fsUpdateClient struct {
	grpc.ClientStream
}

func (x *fsUpdateClient) Send(m *UpdateRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *fsUpdateClient) CloseAndRecv() (*UpdateResponse, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(UpdateResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *fsClient) Inspect(ctx context.Context, in *InspectRequest, opts ...grpc.CallOption) (*InspectResponse, error) {
	out := new(InspectResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/Inspect", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Snapshot(ctx context.Context, in *SnapshotRequest, opts ...grpc.CallOption) (*SnapshotResponse, error) {
	out := new(SnapshotResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/Snapshot", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) Reset(ctx context.Context, in *ResetRequest, opts ...grpc.CallOption) (*ResetResponse, error) {
	out := new(ResetResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/Reset", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GcProject(ctx context.Context, in *GcProjectRequest, opts ...grpc.CallOption) (*GcProjectResponse, error) {
	out := new(GcProjectResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/GcProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GcRandomProjects(ctx context.Context, in *GcRandomProjectsRequest, opts ...grpc.CallOption) (*GcRandomProjectsResponse, error) {
	out := new(GcRandomProjectsResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/GcRandomProjects", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GcContents(ctx context.Context, in *GcContentsRequest, opts ...grpc.CallOption) (*GcContentsResponse, error) {
	out := new(GcContentsResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/GcContents", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) CloneToProject(ctx context.Context, in *CloneToProjectRequest, opts ...grpc.CallOption) (*CloneToProjectResponse, error) {
	out := new(CloneToProjectResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/CloneToProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fsClient) GetCache(ctx context.Context, in *GetCacheRequest, opts ...grpc.CallOption) (Fs_GetCacheClient, error) {
	stream, err := c.cc.NewStream(ctx, &Fs_ServiceDesc.Streams[3], "/pb.Fs/GetCache", opts...)
	if err != nil {
		return nil, err
	}
	x := &fsGetCacheClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Fs_GetCacheClient interface {
	Recv() (*GetCacheResponse, error)
	grpc.ClientStream
}

type fsGetCacheClient struct {
	grpc.ClientStream
}

func (x *fsGetCacheClient) Recv() (*GetCacheResponse, error) {
	m := new(GetCacheResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *fsClient) CreateCache(ctx context.Context, in *CreateCacheRequest, opts ...grpc.CallOption) (*CreateCacheResponse, error) {
	out := new(CreateCacheResponse)
	err := c.cc.Invoke(ctx, "/pb.Fs/CreateCache", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FsServer is the server API for Fs service.
// All implementations must embed UnimplementedFsServer
// for forward compatibility
type FsServer interface {
	NewProject(context.Context, *NewProjectRequest) (*NewProjectResponse, error)
	DeleteProject(context.Context, *DeleteProjectRequest) (*DeleteProjectResponse, error)
	ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error)
	Get(*GetRequest, Fs_GetServer) error
	GetCompress(*GetCompressRequest, Fs_GetCompressServer) error
	Update(Fs_UpdateServer) error
	Inspect(context.Context, *InspectRequest) (*InspectResponse, error)
	Snapshot(context.Context, *SnapshotRequest) (*SnapshotResponse, error)
	Reset(context.Context, *ResetRequest) (*ResetResponse, error)
	GcProject(context.Context, *GcProjectRequest) (*GcProjectResponse, error)
	GcRandomProjects(context.Context, *GcRandomProjectsRequest) (*GcRandomProjectsResponse, error)
	GcContents(context.Context, *GcContentsRequest) (*GcContentsResponse, error)
	CloneToProject(context.Context, *CloneToProjectRequest) (*CloneToProjectResponse, error)
	GetCache(*GetCacheRequest, Fs_GetCacheServer) error
	CreateCache(context.Context, *CreateCacheRequest) (*CreateCacheResponse, error)
	mustEmbedUnimplementedFsServer()
}

// UnimplementedFsServer must be embedded to have forward compatible implementations.
type UnimplementedFsServer struct {
}

func (UnimplementedFsServer) NewProject(context.Context, *NewProjectRequest) (*NewProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NewProject not implemented")
}
func (UnimplementedFsServer) DeleteProject(context.Context, *DeleteProjectRequest) (*DeleteProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteProject not implemented")
}
func (UnimplementedFsServer) ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListProjects not implemented")
}
func (UnimplementedFsServer) Get(*GetRequest, Fs_GetServer) error {
	return status.Errorf(codes.Unimplemented, "method Get not implemented")
}
func (UnimplementedFsServer) GetCompress(*GetCompressRequest, Fs_GetCompressServer) error {
	return status.Errorf(codes.Unimplemented, "method GetCompress not implemented")
}
func (UnimplementedFsServer) Update(Fs_UpdateServer) error {
	return status.Errorf(codes.Unimplemented, "method Update not implemented")
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
func (UnimplementedFsServer) GetCache(*GetCacheRequest, Fs_GetCacheServer) error {
	return status.Errorf(codes.Unimplemented, "method GetCache not implemented")
}
func (UnimplementedFsServer) CreateCache(context.Context, *CreateCacheRequest) (*CreateCacheResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateCache not implemented")
}
func (UnimplementedFsServer) mustEmbedUnimplementedFsServer() {}

// UnsafeFsServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FsServer will
// result in compilation errors.
type UnsafeFsServer interface {
	mustEmbedUnimplementedFsServer()
}

func RegisterFsServer(s grpc.ServiceRegistrar, srv FsServer) {
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
		FullMethod: "/pb.Fs/NewProject",
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
		FullMethod: "/pb.Fs/DeleteProject",
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
		FullMethod: "/pb.Fs/ListProjects",
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
	return srv.(FsServer).Get(m, &fsGetServer{stream})
}

type Fs_GetServer interface {
	Send(*GetResponse) error
	grpc.ServerStream
}

type fsGetServer struct {
	grpc.ServerStream
}

func (x *fsGetServer) Send(m *GetResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _Fs_GetCompress_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetCompressRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(FsServer).GetCompress(m, &fsGetCompressServer{stream})
}

type Fs_GetCompressServer interface {
	Send(*GetCompressResponse) error
	grpc.ServerStream
}

type fsGetCompressServer struct {
	grpc.ServerStream
}

func (x *fsGetCompressServer) Send(m *GetCompressResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _Fs_Update_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(FsServer).Update(&fsUpdateServer{stream})
}

type Fs_UpdateServer interface {
	SendAndClose(*UpdateResponse) error
	Recv() (*UpdateRequest, error)
	grpc.ServerStream
}

type fsUpdateServer struct {
	grpc.ServerStream
}

func (x *fsUpdateServer) SendAndClose(m *UpdateResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *fsUpdateServer) Recv() (*UpdateRequest, error) {
	m := new(UpdateRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
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
		FullMethod: "/pb.Fs/Inspect",
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
		FullMethod: "/pb.Fs/Snapshot",
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
		FullMethod: "/pb.Fs/Reset",
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
		FullMethod: "/pb.Fs/GcProject",
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
		FullMethod: "/pb.Fs/GcRandomProjects",
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
		FullMethod: "/pb.Fs/GcContents",
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
		FullMethod: "/pb.Fs/CloneToProject",
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
	return srv.(FsServer).GetCache(m, &fsGetCacheServer{stream})
}

type Fs_GetCacheServer interface {
	Send(*GetCacheResponse) error
	grpc.ServerStream
}

type fsGetCacheServer struct {
	grpc.ServerStream
}

func (x *fsGetCacheServer) Send(m *GetCacheResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _Fs_CreateCache_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateCacheRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FsServer).CreateCache(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.Fs/CreateCache",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FsServer).CreateCache(ctx, req.(*CreateCacheRequest))
	}
	return interceptor(ctx, in, info, handler)
}

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
		{
			MethodName: "CreateCache",
			Handler:    _Fs_CreateCache_Handler,
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
