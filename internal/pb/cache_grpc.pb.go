// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.29.3
// source: internal/pb/cache.proto

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
	Cached_PopulateDiskCache_FullMethodName = "/pb.Cached/PopulateDiskCache"
)

// CachedClient is the client API for Cached service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type CachedClient interface {
	PopulateDiskCache(ctx context.Context, in *PopulateDiskCacheRequest, opts ...grpc.CallOption) (*PopulateDiskCacheResponse, error)
}

type cachedClient struct {
	cc grpc.ClientConnInterface
}

func NewCachedClient(cc grpc.ClientConnInterface) CachedClient {
	return &cachedClient{cc}
}

func (c *cachedClient) PopulateDiskCache(ctx context.Context, in *PopulateDiskCacheRequest, opts ...grpc.CallOption) (*PopulateDiskCacheResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(PopulateDiskCacheResponse)
	err := c.cc.Invoke(ctx, Cached_PopulateDiskCache_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CachedServer is the server API for Cached service.
// All implementations must embed UnimplementedCachedServer
// for forward compatibility.
type CachedServer interface {
	PopulateDiskCache(context.Context, *PopulateDiskCacheRequest) (*PopulateDiskCacheResponse, error)
	mustEmbedUnimplementedCachedServer()
}

// UnimplementedCachedServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedCachedServer struct{}

func (UnimplementedCachedServer) PopulateDiskCache(context.Context, *PopulateDiskCacheRequest) (*PopulateDiskCacheResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PopulateDiskCache not implemented")
}
func (UnimplementedCachedServer) mustEmbedUnimplementedCachedServer() {}
func (UnimplementedCachedServer) testEmbeddedByValue()                {}

// UnsafeCachedServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to CachedServer will
// result in compilation errors.
type UnsafeCachedServer interface {
	mustEmbedUnimplementedCachedServer()
}

func RegisterCachedServer(s grpc.ServiceRegistrar, srv CachedServer) {
	// If the following call pancis, it indicates UnimplementedCachedServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Cached_ServiceDesc, srv)
}

func _Cached_PopulateDiskCache_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PopulateDiskCacheRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CachedServer).PopulateDiskCache(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Cached_PopulateDiskCache_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CachedServer).PopulateDiskCache(ctx, req.(*PopulateDiskCacheRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Cached_ServiceDesc is the grpc.ServiceDesc for Cached service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Cached_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "pb.Cached",
	HandlerType: (*CachedServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "PopulateDiskCache",
			Handler:    _Cached_PopulateDiskCache_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "internal/pb/cache.proto",
}
