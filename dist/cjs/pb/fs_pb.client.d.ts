import type { RpcTransport } from "@protobuf-ts/runtime-rpc";
import type { ServiceInfo } from "@protobuf-ts/runtime-rpc";
import type { GetCacheResponse } from "./fs_pb";
import type { GetCacheRequest } from "./fs_pb";
import type { CloneToProjectResponse } from "./fs_pb";
import type { CloneToProjectRequest } from "./fs_pb";
import type { GcContentsResponse } from "./fs_pb";
import type { GcContentsRequest } from "./fs_pb";
import type { GcRandomProjectsResponse } from "./fs_pb";
import type { GcRandomProjectsRequest } from "./fs_pb";
import type { GcProjectResponse } from "./fs_pb";
import type { GcProjectRequest } from "./fs_pb";
import type { ResetResponse } from "./fs_pb";
import type { ResetRequest } from "./fs_pb";
import type { SnapshotResponse } from "./fs_pb";
import type { SnapshotRequest } from "./fs_pb";
import type { InspectResponse } from "./fs_pb";
import type { InspectRequest } from "./fs_pb";
import type { RollbackResponse } from "./fs_pb";
import type { RollbackRequest } from "./fs_pb";
import type { UpdateResponse } from "./fs_pb";
import type { UpdateRequest } from "./fs_pb";
import type { ClientStreamingCall } from "@protobuf-ts/runtime-rpc";
import type { GetUnaryResponse } from "./fs_pb";
import type { GetUnaryRequest } from "./fs_pb";
import type { GetCompressResponse } from "./fs_pb";
import type { GetCompressRequest } from "./fs_pb";
import type { GetResponse } from "./fs_pb";
import type { GetRequest } from "./fs_pb";
import type { ServerStreamingCall } from "@protobuf-ts/runtime-rpc";
import type { ListProjectsResponse } from "./fs_pb";
import type { ListProjectsRequest } from "./fs_pb";
import type { DeleteProjectResponse } from "./fs_pb";
import type { DeleteProjectRequest } from "./fs_pb";
import type { NewProjectResponse } from "./fs_pb";
import type { NewProjectRequest } from "./fs_pb";
import type { UnaryCall } from "@protobuf-ts/runtime-rpc";
import type { RpcOptions } from "@protobuf-ts/runtime-rpc";
/**
 * @generated from protobuf service pb.Fs
 */
export interface IFsClient {
    /**
     * @generated from protobuf rpc: NewProject(pb.NewProjectRequest) returns (pb.NewProjectResponse);
     */
    newProject(input: NewProjectRequest, options?: RpcOptions): UnaryCall<NewProjectRequest, NewProjectResponse>;
    /**
     * @generated from protobuf rpc: DeleteProject(pb.DeleteProjectRequest) returns (pb.DeleteProjectResponse);
     */
    deleteProject(input: DeleteProjectRequest, options?: RpcOptions): UnaryCall<DeleteProjectRequest, DeleteProjectResponse>;
    /**
     * @generated from protobuf rpc: ListProjects(pb.ListProjectsRequest) returns (pb.ListProjectsResponse);
     */
    listProjects(input: ListProjectsRequest, options?: RpcOptions): UnaryCall<ListProjectsRequest, ListProjectsResponse>;
    /**
     * @generated from protobuf rpc: Get(pb.GetRequest) returns (stream pb.GetResponse);
     */
    get(input: GetRequest, options?: RpcOptions): ServerStreamingCall<GetRequest, GetResponse>;
    /**
     * @generated from protobuf rpc: GetCompress(pb.GetCompressRequest) returns (stream pb.GetCompressResponse);
     */
    getCompress(input: GetCompressRequest, options?: RpcOptions): ServerStreamingCall<GetCompressRequest, GetCompressResponse>;
    /**
     * @generated from protobuf rpc: GetUnary(pb.GetUnaryRequest) returns (pb.GetUnaryResponse);
     */
    getUnary(input: GetUnaryRequest, options?: RpcOptions): UnaryCall<GetUnaryRequest, GetUnaryResponse>;
    /**
     * @generated from protobuf rpc: Update(stream pb.UpdateRequest) returns (pb.UpdateResponse);
     */
    update(options?: RpcOptions): ClientStreamingCall<UpdateRequest, UpdateResponse>;
    /**
     * @generated from protobuf rpc: Rollback(pb.RollbackRequest) returns (pb.RollbackResponse);
     */
    rollback(input: RollbackRequest, options?: RpcOptions): UnaryCall<RollbackRequest, RollbackResponse>;
    /**
     * @generated from protobuf rpc: Inspect(pb.InspectRequest) returns (pb.InspectResponse);
     */
    inspect(input: InspectRequest, options?: RpcOptions): UnaryCall<InspectRequest, InspectResponse>;
    /**
     * @generated from protobuf rpc: Snapshot(pb.SnapshotRequest) returns (pb.SnapshotResponse);
     */
    snapshot(input: SnapshotRequest, options?: RpcOptions): UnaryCall<SnapshotRequest, SnapshotResponse>;
    /**
     * @generated from protobuf rpc: Reset(pb.ResetRequest) returns (pb.ResetResponse);
     */
    reset(input: ResetRequest, options?: RpcOptions): UnaryCall<ResetRequest, ResetResponse>;
    /**
     * @generated from protobuf rpc: GcProject(pb.GcProjectRequest) returns (pb.GcProjectResponse);
     */
    gcProject(input: GcProjectRequest, options?: RpcOptions): UnaryCall<GcProjectRequest, GcProjectResponse>;
    /**
     * @generated from protobuf rpc: GcRandomProjects(pb.GcRandomProjectsRequest) returns (pb.GcRandomProjectsResponse);
     */
    gcRandomProjects(input: GcRandomProjectsRequest, options?: RpcOptions): UnaryCall<GcRandomProjectsRequest, GcRandomProjectsResponse>;
    /**
     * @generated from protobuf rpc: GcContents(pb.GcContentsRequest) returns (pb.GcContentsResponse);
     */
    gcContents(input: GcContentsRequest, options?: RpcOptions): UnaryCall<GcContentsRequest, GcContentsResponse>;
    /**
     * @generated from protobuf rpc: CloneToProject(pb.CloneToProjectRequest) returns (pb.CloneToProjectResponse);
     */
    cloneToProject(input: CloneToProjectRequest, options?: RpcOptions): UnaryCall<CloneToProjectRequest, CloneToProjectResponse>;
    /**
     * @generated from protobuf rpc: GetCache(pb.GetCacheRequest) returns (stream pb.GetCacheResponse);
     */
    getCache(input: GetCacheRequest, options?: RpcOptions): ServerStreamingCall<GetCacheRequest, GetCacheResponse>;
}
/**
 * @generated from protobuf service pb.Fs
 */
export declare class FsClient implements IFsClient, ServiceInfo {
    private readonly _transport;
    typeName: string;
    methods: import("@protobuf-ts/runtime-rpc").MethodInfo<any, any>[];
    options: {
        [extensionName: string]: import("@protobuf-ts/runtime").JsonValue;
    };
    constructor(_transport: RpcTransport);
    /**
     * @generated from protobuf rpc: NewProject(pb.NewProjectRequest) returns (pb.NewProjectResponse);
     */
    newProject(input: NewProjectRequest, options?: RpcOptions): UnaryCall<NewProjectRequest, NewProjectResponse>;
    /**
     * @generated from protobuf rpc: DeleteProject(pb.DeleteProjectRequest) returns (pb.DeleteProjectResponse);
     */
    deleteProject(input: DeleteProjectRequest, options?: RpcOptions): UnaryCall<DeleteProjectRequest, DeleteProjectResponse>;
    /**
     * @generated from protobuf rpc: ListProjects(pb.ListProjectsRequest) returns (pb.ListProjectsResponse);
     */
    listProjects(input: ListProjectsRequest, options?: RpcOptions): UnaryCall<ListProjectsRequest, ListProjectsResponse>;
    /**
     * @generated from protobuf rpc: Get(pb.GetRequest) returns (stream pb.GetResponse);
     */
    get(input: GetRequest, options?: RpcOptions): ServerStreamingCall<GetRequest, GetResponse>;
    /**
     * @generated from protobuf rpc: GetCompress(pb.GetCompressRequest) returns (stream pb.GetCompressResponse);
     */
    getCompress(input: GetCompressRequest, options?: RpcOptions): ServerStreamingCall<GetCompressRequest, GetCompressResponse>;
    /**
     * @generated from protobuf rpc: GetUnary(pb.GetUnaryRequest) returns (pb.GetUnaryResponse);
     */
    getUnary(input: GetUnaryRequest, options?: RpcOptions): UnaryCall<GetUnaryRequest, GetUnaryResponse>;
    /**
     * @generated from protobuf rpc: Update(stream pb.UpdateRequest) returns (pb.UpdateResponse);
     */
    update(options?: RpcOptions): ClientStreamingCall<UpdateRequest, UpdateResponse>;
    /**
     * @generated from protobuf rpc: Rollback(pb.RollbackRequest) returns (pb.RollbackResponse);
     */
    rollback(input: RollbackRequest, options?: RpcOptions): UnaryCall<RollbackRequest, RollbackResponse>;
    /**
     * @generated from protobuf rpc: Inspect(pb.InspectRequest) returns (pb.InspectResponse);
     */
    inspect(input: InspectRequest, options?: RpcOptions): UnaryCall<InspectRequest, InspectResponse>;
    /**
     * @generated from protobuf rpc: Snapshot(pb.SnapshotRequest) returns (pb.SnapshotResponse);
     */
    snapshot(input: SnapshotRequest, options?: RpcOptions): UnaryCall<SnapshotRequest, SnapshotResponse>;
    /**
     * @generated from protobuf rpc: Reset(pb.ResetRequest) returns (pb.ResetResponse);
     */
    reset(input: ResetRequest, options?: RpcOptions): UnaryCall<ResetRequest, ResetResponse>;
    /**
     * @generated from protobuf rpc: GcProject(pb.GcProjectRequest) returns (pb.GcProjectResponse);
     */
    gcProject(input: GcProjectRequest, options?: RpcOptions): UnaryCall<GcProjectRequest, GcProjectResponse>;
    /**
     * @generated from protobuf rpc: GcRandomProjects(pb.GcRandomProjectsRequest) returns (pb.GcRandomProjectsResponse);
     */
    gcRandomProjects(input: GcRandomProjectsRequest, options?: RpcOptions): UnaryCall<GcRandomProjectsRequest, GcRandomProjectsResponse>;
    /**
     * @generated from protobuf rpc: GcContents(pb.GcContentsRequest) returns (pb.GcContentsResponse);
     */
    gcContents(input: GcContentsRequest, options?: RpcOptions): UnaryCall<GcContentsRequest, GcContentsResponse>;
    /**
     * @generated from protobuf rpc: CloneToProject(pb.CloneToProjectRequest) returns (pb.CloneToProjectResponse);
     */
    cloneToProject(input: CloneToProjectRequest, options?: RpcOptions): UnaryCall<CloneToProjectRequest, CloneToProjectResponse>;
    /**
     * @generated from protobuf rpc: GetCache(pb.GetCacheRequest) returns (stream pb.GetCacheResponse);
     */
    getCache(input: GetCacheRequest, options?: RpcOptions): ServerStreamingCall<GetCacheRequest, GetCacheResponse>;
}
