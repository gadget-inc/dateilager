import { ServiceType } from "@protobuf-ts/runtime-rpc";
import type { BinaryWriteOptions } from "@protobuf-ts/runtime";
import type { IBinaryWriter } from "@protobuf-ts/runtime";
import type { BinaryReadOptions } from "@protobuf-ts/runtime";
import type { IBinaryReader } from "@protobuf-ts/runtime";
import type { PartialMessage } from "@protobuf-ts/runtime";
import { MessageType } from "@protobuf-ts/runtime";
/**
 * @generated from protobuf message pb.NewProjectRequest
 */
export interface NewProjectRequest {
    /**
     * @generated from protobuf field: int64 id = 1;
     */
    id: bigint;
    /**
     * @generated from protobuf field: optional int64 template = 2;
     */
    template?: bigint;
    /**
     * @generated from protobuf field: repeated string pack_patterns = 3;
     */
    packPatterns: string[];
}
/**
 * @generated from protobuf message pb.NewProjectResponse
 */
export interface NewProjectResponse {
}
/**
 * @generated from protobuf message pb.DeleteProjectRequest
 */
export interface DeleteProjectRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
}
/**
 * @generated from protobuf message pb.DeleteProjectResponse
 */
export interface DeleteProjectResponse {
}
/**
 * @generated from protobuf message pb.Project
 */
export interface Project {
    /**
     * @generated from protobuf field: int64 id = 1;
     */
    id: bigint;
    /**
     * @generated from protobuf field: int64 version = 2;
     */
    version: bigint;
}
/**
 * @generated from protobuf message pb.ListProjectsRequest
 */
export interface ListProjectsRequest {
}
/**
 * @generated from protobuf message pb.ListProjectsResponse
 */
export interface ListProjectsResponse {
    /**
     * @generated from protobuf field: repeated pb.Project projects = 1;
     */
    projects: Project[];
}
/**
 * Typescript does not support creating a new Object class
 *
 * @generated from protobuf message pb.Objekt
 */
export interface Objekt {
    /**
     * @generated from protobuf field: string path = 1;
     */
    path: string;
    /**
     * @generated from protobuf field: int64 mode = 2;
     */
    mode: bigint;
    /**
     * @generated from protobuf field: int64 size = 3;
     */
    size: bigint;
    /**
     * @generated from protobuf field: bool deleted = 4;
     */
    deleted: boolean;
    /**
     * @generated from protobuf field: optional bytes content = 5;
     */
    content?: Uint8Array;
}
/**
 * @generated from protobuf message pb.ObjectQuery
 */
export interface ObjectQuery {
    /**
     * @generated from protobuf field: string path = 1;
     */
    path: string;
    /**
     * @generated from protobuf field: bool is_prefix = 2;
     */
    isPrefix: boolean;
    /**
     * @generated from protobuf field: repeated string ignores = 4;
     */
    ignores: string[];
    /**
     * @generated from protobuf field: repeated string subpaths = 5;
     */
    subpaths: string[];
}
/**
 * @generated from protobuf message pb.GetRequest
 */
export interface GetRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
    /**
     * @generated from protobuf field: optional int64 from_version = 2;
     */
    fromVersion?: bigint;
    /**
     * @generated from protobuf field: optional int64 to_version = 3;
     */
    toVersion?: bigint;
    /**
     * @generated from protobuf field: repeated pb.ObjectQuery queries = 4;
     */
    queries: ObjectQuery[];
}
/**
 * @generated from protobuf message pb.GetResponse
 */
export interface GetResponse {
    /**
     * @generated from protobuf field: int64 version = 1;
     */
    version: bigint;
    /**
     * @generated from protobuf field: pb.Objekt object = 2;
     */
    object?: Objekt;
}
/**
 * @generated from protobuf message pb.GetCompressRequest
 */
export interface GetCompressRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
    /**
     * @generated from protobuf field: optional int64 from_version = 2;
     */
    fromVersion?: bigint;
    /**
     * @generated from protobuf field: optional int64 to_version = 3;
     */
    toVersion?: bigint;
    /**
     * @generated from protobuf field: repeated pb.ObjectQuery queries = 5;
     */
    queries: ObjectQuery[];
    /**
     * @generated from protobuf field: repeated int64 available_cache_versions = 6;
     */
    availableCacheVersions: bigint[];
}
/**
 * @generated from protobuf message pb.GetCompressResponse
 */
export interface GetCompressResponse {
    /**
     * @generated from protobuf field: int64 version = 1;
     */
    version: bigint;
    /**
     * @generated from protobuf field: pb.GetCompressResponse.Format format = 2;
     */
    format: GetCompressResponse_Format;
    /**
     * @generated from protobuf field: bytes bytes = 3;
     */
    bytes: Uint8Array;
    /**
     * @generated from protobuf field: optional string pack_path = 4;
     */
    packPath?: string;
}
/**
 * @generated from protobuf enum pb.GetCompressResponse.Format
 */
export declare enum GetCompressResponse_Format {
    /**
     * @generated from protobuf enum value: S2_TAR = 0;
     */
    S2_TAR = 0
}
/**
 * @generated from protobuf message pb.GetUnaryRequest
 */
export interface GetUnaryRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
    /**
     * @generated from protobuf field: optional int64 from_version = 2;
     */
    fromVersion?: bigint;
    /**
     * @generated from protobuf field: optional int64 to_version = 3;
     */
    toVersion?: bigint;
    /**
     * @generated from protobuf field: repeated pb.ObjectQuery queries = 4;
     */
    queries: ObjectQuery[];
    /**
     * @generated from protobuf field: optional int64 max_content_send_size = 5;
     */
    maxContentSendSize?: bigint;
}
/**
 * @generated from protobuf message pb.GetUnaryResponse
 */
export interface GetUnaryResponse {
    /**
     * @generated from protobuf field: int64 version = 1;
     */
    version: bigint;
    /**
     * @generated from protobuf field: repeated pb.Objekt objects = 2;
     */
    objects: Objekt[];
}
/**
 * @generated from protobuf message pb.UpdateRequest
 */
export interface UpdateRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
    /**
     * @generated from protobuf field: pb.Objekt object = 2;
     */
    object?: Objekt;
}
/**
 * @generated from protobuf message pb.UpdateResponse
 */
export interface UpdateResponse {
    /**
     * @generated from protobuf field: int64 version = 1;
     */
    version: bigint;
}
/**
 * @generated from protobuf message pb.RollbackRequest
 */
export interface RollbackRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
    /**
     * @generated from protobuf field: int64 version = 2;
     */
    version: bigint;
}
/**
 * @generated from protobuf message pb.RollbackResponse
 */
export interface RollbackResponse {
}
/**
 * @generated from protobuf message pb.InspectRequest
 */
export interface InspectRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
}
/**
 * @generated from protobuf message pb.InspectResponse
 */
export interface InspectResponse {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
    /**
     * @generated from protobuf field: int64 latest_version = 2;
     */
    latestVersion: bigint;
    /**
     * @generated from protobuf field: int64 live_objects_count = 3;
     */
    liveObjectsCount: bigint;
    /**
     * @generated from protobuf field: int64 total_objects_count = 4;
     */
    totalObjectsCount: bigint;
}
/**
 * @generated from protobuf message pb.SnapshotRequest
 */
export interface SnapshotRequest {
}
/**
 * @generated from protobuf message pb.SnapshotResponse
 */
export interface SnapshotResponse {
    /**
     * @generated from protobuf field: repeated pb.Project projects = 1;
     */
    projects: Project[];
}
/**
 * @generated from protobuf message pb.ResetRequest
 */
export interface ResetRequest {
    /**
     * @generated from protobuf field: repeated pb.Project projects = 1;
     */
    projects: Project[];
}
/**
 * @generated from protobuf message pb.ResetResponse
 */
export interface ResetResponse {
}
/**
 * @generated from protobuf message pb.GcProjectRequest
 */
export interface GcProjectRequest {
    /**
     * @generated from protobuf field: int64 project = 1;
     */
    project: bigint;
    /**
     * @generated from protobuf field: int64 keep_versions = 2;
     */
    keepVersions: bigint;
    /**
     * @generated from protobuf field: optional int64 from_version = 3;
     */
    fromVersion?: bigint;
}
/**
 * @generated from protobuf message pb.GcProjectResponse
 */
export interface GcProjectResponse {
    /**
     * @generated from protobuf field: int64 count = 1;
     */
    count: bigint;
    /**
     * @generated from protobuf field: int64 project = 2;
     */
    project: bigint;
}
/**
 * @generated from protobuf message pb.GcRandomProjectsRequest
 */
export interface GcRandomProjectsRequest {
    /**
     * @generated from protobuf field: float sample = 1;
     */
    sample: number;
    /**
     * @generated from protobuf field: int64 keep_versions = 2;
     */
    keepVersions: bigint;
    /**
     * @generated from protobuf field: optional int64 from_version = 3;
     */
    fromVersion?: bigint;
}
/**
 * @generated from protobuf message pb.GcRandomProjectsResponse
 */
export interface GcRandomProjectsResponse {
    /**
     * @generated from protobuf field: int64 count = 1;
     */
    count: bigint;
    /**
     * @generated from protobuf field: repeated int64 projects = 2;
     */
    projects: bigint[];
}
/**
 * @generated from protobuf message pb.GcContentsRequest
 */
export interface GcContentsRequest {
    /**
     * @generated from protobuf field: float sample = 1;
     */
    sample: number;
}
/**
 * @generated from protobuf message pb.GcContentsResponse
 */
export interface GcContentsResponse {
    /**
     * @generated from protobuf field: int64 count = 1;
     */
    count: bigint;
}
/**
 * @generated from protobuf message pb.CloneToProjectRequest
 */
export interface CloneToProjectRequest {
    /**
     * @generated from protobuf field: int64 source = 1;
     */
    source: bigint;
    /**
     * @generated from protobuf field: int64 version = 2;
     */
    version: bigint;
    /**
     * @generated from protobuf field: int64 target = 3;
     */
    target: bigint;
}
/**
 * @generated from protobuf message pb.CloneToProjectResponse
 */
export interface CloneToProjectResponse {
    /**
     * @generated from protobuf field: int64 latest_version = 1;
     */
    latestVersion: bigint;
}
/**
 * @generated from protobuf message pb.GetCacheRequest
 */
export interface GetCacheRequest {
    /**
     * @generated from protobuf field: optional int64 version = 1;
     */
    version?: bigint;
}
/**
 * @generated from protobuf message pb.GetCacheResponse
 */
export interface GetCacheResponse {
    /**
     * @generated from protobuf field: int64 version = 1;
     */
    version: bigint;
    /**
     * @generated from protobuf field: pb.GetCacheResponse.Format format = 2;
     */
    format: GetCacheResponse_Format;
    /**
     * @generated from protobuf field: bytes bytes = 3;
     */
    bytes: Uint8Array;
    /**
     * @generated from protobuf field: bytes hash = 4;
     */
    hash: Uint8Array;
}
/**
 * @generated from protobuf enum pb.GetCacheResponse.Format
 */
export declare enum GetCacheResponse_Format {
    /**
     * @generated from protobuf enum value: S2_TAR = 0;
     */
    S2_TAR = 0
}
declare class NewProjectRequest$Type extends MessageType<NewProjectRequest> {
    constructor();
    create(value?: PartialMessage<NewProjectRequest>): NewProjectRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: NewProjectRequest): NewProjectRequest;
    internalBinaryWrite(message: NewProjectRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.NewProjectRequest
 */
export declare const NewProjectRequest: NewProjectRequest$Type;
declare class NewProjectResponse$Type extends MessageType<NewProjectResponse> {
    constructor();
    create(value?: PartialMessage<NewProjectResponse>): NewProjectResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: NewProjectResponse): NewProjectResponse;
    internalBinaryWrite(message: NewProjectResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.NewProjectResponse
 */
export declare const NewProjectResponse: NewProjectResponse$Type;
declare class DeleteProjectRequest$Type extends MessageType<DeleteProjectRequest> {
    constructor();
    create(value?: PartialMessage<DeleteProjectRequest>): DeleteProjectRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: DeleteProjectRequest): DeleteProjectRequest;
    internalBinaryWrite(message: DeleteProjectRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.DeleteProjectRequest
 */
export declare const DeleteProjectRequest: DeleteProjectRequest$Type;
declare class DeleteProjectResponse$Type extends MessageType<DeleteProjectResponse> {
    constructor();
    create(value?: PartialMessage<DeleteProjectResponse>): DeleteProjectResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: DeleteProjectResponse): DeleteProjectResponse;
    internalBinaryWrite(message: DeleteProjectResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.DeleteProjectResponse
 */
export declare const DeleteProjectResponse: DeleteProjectResponse$Type;
declare class Project$Type extends MessageType<Project> {
    constructor();
    create(value?: PartialMessage<Project>): Project;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: Project): Project;
    internalBinaryWrite(message: Project, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.Project
 */
export declare const Project: Project$Type;
declare class ListProjectsRequest$Type extends MessageType<ListProjectsRequest> {
    constructor();
    create(value?: PartialMessage<ListProjectsRequest>): ListProjectsRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: ListProjectsRequest): ListProjectsRequest;
    internalBinaryWrite(message: ListProjectsRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.ListProjectsRequest
 */
export declare const ListProjectsRequest: ListProjectsRequest$Type;
declare class ListProjectsResponse$Type extends MessageType<ListProjectsResponse> {
    constructor();
    create(value?: PartialMessage<ListProjectsResponse>): ListProjectsResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: ListProjectsResponse): ListProjectsResponse;
    internalBinaryWrite(message: ListProjectsResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.ListProjectsResponse
 */
export declare const ListProjectsResponse: ListProjectsResponse$Type;
declare class Objekt$Type extends MessageType<Objekt> {
    constructor();
    create(value?: PartialMessage<Objekt>): Objekt;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: Objekt): Objekt;
    internalBinaryWrite(message: Objekt, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.Objekt
 */
export declare const Objekt: Objekt$Type;
declare class ObjectQuery$Type extends MessageType<ObjectQuery> {
    constructor();
    create(value?: PartialMessage<ObjectQuery>): ObjectQuery;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: ObjectQuery): ObjectQuery;
    internalBinaryWrite(message: ObjectQuery, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.ObjectQuery
 */
export declare const ObjectQuery: ObjectQuery$Type;
declare class GetRequest$Type extends MessageType<GetRequest> {
    constructor();
    create(value?: PartialMessage<GetRequest>): GetRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetRequest): GetRequest;
    internalBinaryWrite(message: GetRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetRequest
 */
export declare const GetRequest: GetRequest$Type;
declare class GetResponse$Type extends MessageType<GetResponse> {
    constructor();
    create(value?: PartialMessage<GetResponse>): GetResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetResponse): GetResponse;
    internalBinaryWrite(message: GetResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetResponse
 */
export declare const GetResponse: GetResponse$Type;
declare class GetCompressRequest$Type extends MessageType<GetCompressRequest> {
    constructor();
    create(value?: PartialMessage<GetCompressRequest>): GetCompressRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetCompressRequest): GetCompressRequest;
    internalBinaryWrite(message: GetCompressRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetCompressRequest
 */
export declare const GetCompressRequest: GetCompressRequest$Type;
declare class GetCompressResponse$Type extends MessageType<GetCompressResponse> {
    constructor();
    create(value?: PartialMessage<GetCompressResponse>): GetCompressResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetCompressResponse): GetCompressResponse;
    internalBinaryWrite(message: GetCompressResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetCompressResponse
 */
export declare const GetCompressResponse: GetCompressResponse$Type;
declare class GetUnaryRequest$Type extends MessageType<GetUnaryRequest> {
    constructor();
    create(value?: PartialMessage<GetUnaryRequest>): GetUnaryRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetUnaryRequest): GetUnaryRequest;
    internalBinaryWrite(message: GetUnaryRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetUnaryRequest
 */
export declare const GetUnaryRequest: GetUnaryRequest$Type;
declare class GetUnaryResponse$Type extends MessageType<GetUnaryResponse> {
    constructor();
    create(value?: PartialMessage<GetUnaryResponse>): GetUnaryResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetUnaryResponse): GetUnaryResponse;
    internalBinaryWrite(message: GetUnaryResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetUnaryResponse
 */
export declare const GetUnaryResponse: GetUnaryResponse$Type;
declare class UpdateRequest$Type extends MessageType<UpdateRequest> {
    constructor();
    create(value?: PartialMessage<UpdateRequest>): UpdateRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: UpdateRequest): UpdateRequest;
    internalBinaryWrite(message: UpdateRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.UpdateRequest
 */
export declare const UpdateRequest: UpdateRequest$Type;
declare class UpdateResponse$Type extends MessageType<UpdateResponse> {
    constructor();
    create(value?: PartialMessage<UpdateResponse>): UpdateResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: UpdateResponse): UpdateResponse;
    internalBinaryWrite(message: UpdateResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.UpdateResponse
 */
export declare const UpdateResponse: UpdateResponse$Type;
declare class RollbackRequest$Type extends MessageType<RollbackRequest> {
    constructor();
    create(value?: PartialMessage<RollbackRequest>): RollbackRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: RollbackRequest): RollbackRequest;
    internalBinaryWrite(message: RollbackRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.RollbackRequest
 */
export declare const RollbackRequest: RollbackRequest$Type;
declare class RollbackResponse$Type extends MessageType<RollbackResponse> {
    constructor();
    create(value?: PartialMessage<RollbackResponse>): RollbackResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: RollbackResponse): RollbackResponse;
    internalBinaryWrite(message: RollbackResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.RollbackResponse
 */
export declare const RollbackResponse: RollbackResponse$Type;
declare class InspectRequest$Type extends MessageType<InspectRequest> {
    constructor();
    create(value?: PartialMessage<InspectRequest>): InspectRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: InspectRequest): InspectRequest;
    internalBinaryWrite(message: InspectRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.InspectRequest
 */
export declare const InspectRequest: InspectRequest$Type;
declare class InspectResponse$Type extends MessageType<InspectResponse> {
    constructor();
    create(value?: PartialMessage<InspectResponse>): InspectResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: InspectResponse): InspectResponse;
    internalBinaryWrite(message: InspectResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.InspectResponse
 */
export declare const InspectResponse: InspectResponse$Type;
declare class SnapshotRequest$Type extends MessageType<SnapshotRequest> {
    constructor();
    create(value?: PartialMessage<SnapshotRequest>): SnapshotRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: SnapshotRequest): SnapshotRequest;
    internalBinaryWrite(message: SnapshotRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.SnapshotRequest
 */
export declare const SnapshotRequest: SnapshotRequest$Type;
declare class SnapshotResponse$Type extends MessageType<SnapshotResponse> {
    constructor();
    create(value?: PartialMessage<SnapshotResponse>): SnapshotResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: SnapshotResponse): SnapshotResponse;
    internalBinaryWrite(message: SnapshotResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.SnapshotResponse
 */
export declare const SnapshotResponse: SnapshotResponse$Type;
declare class ResetRequest$Type extends MessageType<ResetRequest> {
    constructor();
    create(value?: PartialMessage<ResetRequest>): ResetRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: ResetRequest): ResetRequest;
    internalBinaryWrite(message: ResetRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.ResetRequest
 */
export declare const ResetRequest: ResetRequest$Type;
declare class ResetResponse$Type extends MessageType<ResetResponse> {
    constructor();
    create(value?: PartialMessage<ResetResponse>): ResetResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: ResetResponse): ResetResponse;
    internalBinaryWrite(message: ResetResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.ResetResponse
 */
export declare const ResetResponse: ResetResponse$Type;
declare class GcProjectRequest$Type extends MessageType<GcProjectRequest> {
    constructor();
    create(value?: PartialMessage<GcProjectRequest>): GcProjectRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GcProjectRequest): GcProjectRequest;
    internalBinaryWrite(message: GcProjectRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GcProjectRequest
 */
export declare const GcProjectRequest: GcProjectRequest$Type;
declare class GcProjectResponse$Type extends MessageType<GcProjectResponse> {
    constructor();
    create(value?: PartialMessage<GcProjectResponse>): GcProjectResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GcProjectResponse): GcProjectResponse;
    internalBinaryWrite(message: GcProjectResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GcProjectResponse
 */
export declare const GcProjectResponse: GcProjectResponse$Type;
declare class GcRandomProjectsRequest$Type extends MessageType<GcRandomProjectsRequest> {
    constructor();
    create(value?: PartialMessage<GcRandomProjectsRequest>): GcRandomProjectsRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GcRandomProjectsRequest): GcRandomProjectsRequest;
    internalBinaryWrite(message: GcRandomProjectsRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GcRandomProjectsRequest
 */
export declare const GcRandomProjectsRequest: GcRandomProjectsRequest$Type;
declare class GcRandomProjectsResponse$Type extends MessageType<GcRandomProjectsResponse> {
    constructor();
    create(value?: PartialMessage<GcRandomProjectsResponse>): GcRandomProjectsResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GcRandomProjectsResponse): GcRandomProjectsResponse;
    internalBinaryWrite(message: GcRandomProjectsResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GcRandomProjectsResponse
 */
export declare const GcRandomProjectsResponse: GcRandomProjectsResponse$Type;
declare class GcContentsRequest$Type extends MessageType<GcContentsRequest> {
    constructor();
    create(value?: PartialMessage<GcContentsRequest>): GcContentsRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GcContentsRequest): GcContentsRequest;
    internalBinaryWrite(message: GcContentsRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GcContentsRequest
 */
export declare const GcContentsRequest: GcContentsRequest$Type;
declare class GcContentsResponse$Type extends MessageType<GcContentsResponse> {
    constructor();
    create(value?: PartialMessage<GcContentsResponse>): GcContentsResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GcContentsResponse): GcContentsResponse;
    internalBinaryWrite(message: GcContentsResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GcContentsResponse
 */
export declare const GcContentsResponse: GcContentsResponse$Type;
declare class CloneToProjectRequest$Type extends MessageType<CloneToProjectRequest> {
    constructor();
    create(value?: PartialMessage<CloneToProjectRequest>): CloneToProjectRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: CloneToProjectRequest): CloneToProjectRequest;
    internalBinaryWrite(message: CloneToProjectRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.CloneToProjectRequest
 */
export declare const CloneToProjectRequest: CloneToProjectRequest$Type;
declare class CloneToProjectResponse$Type extends MessageType<CloneToProjectResponse> {
    constructor();
    create(value?: PartialMessage<CloneToProjectResponse>): CloneToProjectResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: CloneToProjectResponse): CloneToProjectResponse;
    internalBinaryWrite(message: CloneToProjectResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.CloneToProjectResponse
 */
export declare const CloneToProjectResponse: CloneToProjectResponse$Type;
declare class GetCacheRequest$Type extends MessageType<GetCacheRequest> {
    constructor();
    create(value?: PartialMessage<GetCacheRequest>): GetCacheRequest;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetCacheRequest): GetCacheRequest;
    internalBinaryWrite(message: GetCacheRequest, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetCacheRequest
 */
export declare const GetCacheRequest: GetCacheRequest$Type;
declare class GetCacheResponse$Type extends MessageType<GetCacheResponse> {
    constructor();
    create(value?: PartialMessage<GetCacheResponse>): GetCacheResponse;
    internalBinaryRead(reader: IBinaryReader, length: number, options: BinaryReadOptions, target?: GetCacheResponse): GetCacheResponse;
    internalBinaryWrite(message: GetCacheResponse, writer: IBinaryWriter, options: BinaryWriteOptions): IBinaryWriter;
}
/**
 * @generated MessageType for protobuf message pb.GetCacheResponse
 */
export declare const GetCacheResponse: GetCacheResponse$Type;
/**
 * @generated ServiceType for protobuf service pb.Fs
 */
export declare const Fs: ServiceType;
export {};
