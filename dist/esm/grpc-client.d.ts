import type { ClientOptions } from "@grpc/grpc-js";
import type { Span } from "@opentelemetry/api";
import { type ClientStreamingCall, type RpcOptions } from "@protobuf-ts/runtime-rpc";
import type { CloneToProjectResponse, GetUnaryResponse, Objekt, Project, UpdateRequest, UpdateResponse } from "./pb/fs_pb";
export type { Objekt, Project };
/**
 * Options for {@link DateiLagerGrpcClient}.
 */
export interface DateiLagerGrpcClientOptions {
    /**
     * The address of the dateilager server.
     */
    server: string | {
        /**
         * The host of the dateilager server.
         */
        host: string;
        /**
         * The port of the dateilager server.
         */
        port: number;
    };
    /**
     * The token that will be sent as authorization metadata to the dateilager server.
     */
    token: string | (() => Promise<string>);
    /**
     * Options that will be passed to the underlying grpc client constructor.
     * @see ClientOptions
     */
    grpcClientOptions?: ClientOptions;
    /**
     * Options that will be passed to every remote procedure call.
     * @see RpcOptions
     */
    rpcOptions?: RpcOptions | (() => RpcOptions | undefined);
}
/** Options for calls that list objects */
export interface ListObjectsOptions {
    /** The project version to start from. If not provided, will send all objects in the latest version. */
    from?: bigint;
    /** The project version to end at. If not provided, will send all objects in the latest version. */
    to?: bigint;
    /** Don't send objects that are under these given paths. Acts in tandem with subpaths, where objects will only be sent if they pass both the include / exclude filters. */
    ignores?: string[];
    /** Only send objects that live at one of these subpaths in the response. Acts in tandem with ignores, where objects will only be sent if they pass both the include / exclude filters. */
    subpaths?: string[];
    /** The maximum file size to send content for. If object contents are larger than this, the object's metadata will still be sent but the contents will be omitted */
    maxContentSendSize?: bigint;
}
/**
 * A client class for interacting with DateiLager's GRPC API.
 *
 * The DateiLager API surface area is as minimal as possible;
 * convenience functions, such as getObject, should be implemented within the client.
 */
export declare class DateiLagerGrpcClient {
    /**
     * The library used to interact with GRPC creates connections lazily, this constructor will not
     * raise an error even if there is no service running at {@link DateiLagerGrpcClientOptions.server server}.
     * @param options Grpc client options.
     */
    constructor(options: DateiLagerGrpcClientOptions);
    /**
     * Close the underlying GRPC client.
     */
    close(): void;
    /**
     * Create a new project.
     * @param project      The id of the project.
     * @param packPatterns The paths to pack.
     * @param template     The id of the project to start from.
     */
    newProject(project: bigint, packPatterns: string[], template?: bigint): Promise<void>;
    /**
     * Delete a project.
     * @param project The id of the project.
     */
    deleteProject(project: bigint): Promise<void>;
    /**
     * List objects.
     * @param  project The id of the project.
     * @param  path    The path to list objects under.
     * @param  options The options for the list objects call to limit which objects are returned.
     * @returns        A stream of objects.
     * @yields           An object from the stream.
     * @example
     * for await (const object of client.listObjects(1n, "")) {
     *   console.log("[listObjects] path: " + object.path);
     *   console.log("[listObjects] content:\n" + object.content);
     * }
     */
    listObjects(project: bigint, path: string, options?: ListObjectsOptions): AsyncGenerator<Objekt, void>;
    /**
     * Get objects.
     * @param project The id of the project.
     * @param path    The path to get objects under.
     * @param options The options for the get objects call to limit which objcets are returned.
     * @returns       All the objects under {@link path}.
     * @example
     * const response = await client.getObjects(1n, "");
     * for (const object of response.objects) {
     *   console.log("[getObjects] path: " + object.path);
     *   console.log("[getObjects] content:\n" + object.content);
     * }
     */
    getObjects(project: bigint, path: string, options?: ListObjectsOptions): Promise<GetUnaryResponse>;
    /**
     * Get an object.
     * @param project The id of the project.
     * @param path    The path of the object.
     * @param from    The project version to start from.
     * @param to      The project version to end at.
     * @returns       The object at the given path or undefined if it does not exist.
     */
    getObject(project: bigint, path: string, from?: bigint, to?: bigint): Promise<Objekt | undefined>;
    /**
     * Update objects.
     * @param project The id of the project.
     * @returns       An {@link UpdateInputStream} to send objects to update.
     */
    updateObjects(project: bigint): UpdateInputStream;
    /**
     * Update an object.
     * @param project The id of the project.
     * @param obj     The object to update.
     * @returns       The latest project version or `null` if something went wrong.
     */
    updateObject(project: bigint, obj: Objekt): Promise<bigint | null>;
    /**
     * Rollback a project.
     * @param project The id of the project.
     * @param version The version to rollback to.
     */
    rollbackProject(project: bigint, version: bigint): Promise<void>;
    /**
     * Snapshot the current state of the dateilager server.
     * @returns All the projects on the dateilager server.
     * @throws If the dateilager server's DL_ENV environment variable is PROD.
     * @see DateiLagerGrpcClient.resetToSnapshotInDevOrTests
     */
    snapshotInDevOrTests(): Promise<Project[]>;
    /**
     * Reset the given projects to their respective versions and delete any remaining projects.
     * If no projects are provided, delete all projects.
     * @param projects The projects to reset.
     * @throws If the dateilager server's DL_ENV environment variable is PROD.
     * @see DateiLagerGrpcClient.snapshotInDevOrTests
     */
    resetToSnapshotInDevOrTests(projects: Project[]): Promise<void>;
    /**
     * Clones the `source` projects changes (from `fromVersion` up to `toVersion`) to the `target` project.
     * This method assumes that it is always a one way clone from source to target, it does not take into account
     * the changes that have occurred in the `target` project.
     * @param source  The source project.
     * @param target  The target project.
     * @param version The version of the source project to clone up to.
     * @returns       The new version number of the target project.
     */
    cloneToProject(source: bigint, target: bigint, version: bigint): Promise<CloneToProjectResponse>;
    /**
     * GC project.
     * @param project The project to GC.
     * @param keep    The amount of versions since the latest that should be kept.
     * @param from    The starting version to GC from.
     * @returns       The amount of objects that were GC'd.
     */
    gcProject(project: bigint, keep: bigint, from?: bigint): Promise<bigint>;
    /**
     * GC random projects.
     * @param sample The percentage of projects to sample from.
     * @param keep   The amount of versions since the latest that should be kept.
     * @param from   The starting version to GC from.
     * @returns      The amount of objects that were GC'd.
     */
    gcRandomProjects(sample: number, keep: bigint, from?: bigint): Promise<bigint>;
    /**
     * GC contents.
     * @param sample The percentage of projects to sample from.
     * @returns      The amount of objects that were GC'd.
     */
    gcContents(sample: number): Promise<bigint>;
}
/**
 * Used to send a stream of objects to update.
 * @see DateiLagerGrpcClient.updateObjects
 */
declare class UpdateInputStream {
    constructor(project: bigint, call: ClientStreamingCall<UpdateRequest, UpdateResponse>, span: Span);
    /**
     * Send an object to update.
     * @param obj The object to update.
     */
    send(obj: Objekt): Promise<void>;
    /**
     * Complete the update request.
     * @returns The latest project version or `null` if something went wrong.
     */
    complete(): Promise<bigint | null>;
}
/**
 * Encode string object contents as an array of bytes.
 * @param content The string to encode.
 * @returns       The encoded content as an array of bytes.
 */
export declare function encodeContent(content: string): Uint8Array;
/**
 * Decode an array of bytes as an object's string contents.
 * @param bytes The array of bytes to decode.
 * @returns     The bytes decoded into a string.
 */
export declare function decodeContent(bytes: Uint8Array | undefined): string;
