"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.decodeContent = exports.encodeContent = exports.DateiLagerGrpcClient = void 0;
const grpc_js_1 = require("@grpc/grpc-js");
const api_1 = require("@opentelemetry/api");
const grpc_transport_1 = require("@protobuf-ts/grpc-transport");
const runtime_rpc_1 = require("@protobuf-ts/runtime-rpc");
const util_1 = require("util");
const telemetry_1 = require("./internal/telemetry");
const fs_pb_client_1 = require("./pb/fs_pb.client");
const errors_1 = require("./utils/errors");
/**
 * A client class for interacting with DateiLager's GRPC API.
 *
 * The DateiLager API surface area is as minimal as possible;
 * convenience functions, such as getObject, should be implemented within the client.
 */
class DateiLagerGrpcClient {
    /**
     * The library used to interact with GRPC creates connections lazily, this constructor will not
     * raise an error even if there is no service running at {@link DateiLagerGrpcClientOptions.server server}.
     * @param options Grpc client options.
     */
    constructor(options) {
        /** @internal */
        Object.defineProperty(this, "_client", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        /** @internal */
        Object.defineProperty(this, "_transport", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        /** @internal */
        Object.defineProperty(this, "_rpcOptions", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        const tokenFn = typeof options.token === "string" ? () => Promise.resolve(options.token) : options.token;
        this._transport = new grpc_transport_1.GrpcTransport({
            host: typeof options.server === "string" ? options.server : `${options.server.host}:${options.server.port}`,
            channelCredentials: grpc_js_1.credentials.combineChannelCredentials(grpc_js_1.ChannelCredentials.createSsl(), grpc_js_1.credentials.createFromMetadataGenerator((_, callback) => {
                tokenFn()
                    .then((token) => {
                    const meta = new grpc_js_1.Metadata();
                    meta.add("authorization", `Bearer ${token}`);
                    callback(null, meta);
                })
                    .catch(callback);
            })),
            clientOptions: {
                "grpc.keepalive_time_ms": 5000,
                "grpc.keepalive_timeout_ms": 1000,
                "grpc.keepalive_permit_without_calls": 1,
                ...options.grpcClientOptions,
            },
        });
        this._client = new fs_pb_client_1.FsClient(this._transport);
        this._rpcOptions = options.rpcOptions instanceof Function ? options.rpcOptions : () => options.rpcOptions;
    }
    /**
     * Close the underlying GRPC client.
     */
    close() {
        this._transport.close();
    }
    /**
     * Create a new project.
     * @param project      The id of the project.
     * @param packPatterns The paths to pack.
     * @param template     The id of the project to start from.
     */
    async newProject(project, packPatterns, template) {
        try {
            await (0, telemetry_1.trace)("dateilager-grpc-client.new-project", {
                attributes: {
                    "dl.project": String(project),
                    "dl.pack_patterns": packPatterns,
                    "dl.template": String(template),
                },
            }, () => this._client.newProject({ id: project, packPatterns, template }, this._rpcOptions()));
        }
        catch (error) {
            if (error instanceof runtime_rpc_1.RpcError && error.code == "ALREADY_EXISTS") {
                throw new errors_1.ProjectAlreadyExistsError(`project id ${project} already exists`);
            }
            throw error;
        }
    }
    /**
     * Delete a project.
     * @param project The id of the project.
     */
    async deleteProject(project) {
        await (0, telemetry_1.trace)("dateilager-grpc-client.delete-project", {
            attributes: {
                "dl.project": String(project),
            },
        }, () => this._client.deleteProject({ project }, this._rpcOptions()));
    }
    /**
     * List objects.
     * @param  project The id of the project.
     * @param  path    The path to list objects under.
     * @param  ignores The paths under {@link path} to ignore.
     * @param  from    The project version to start from.
     * @param  to      The project version to end at.
     * @returns        A stream of objects.
     * @yields           An object from the stream.
     * @example
     * for await (const object of client.listObjects(1n, "")) {
     *   console.log("[listObjects] path: " + object.path);
     *   console.log("[listObjects] content:\n" + object.content);
     * }
     */
    async *listObjects(project, path, ignores = [], from, to) {
        const parentContext = api_1.context.active();
        const span = telemetry_1.tracer.startSpan("dateilager-grpc-client.list-objects", {
            attributes: {
                "dl.project": String(project),
                "dl.path": path,
                "dl.ignores": ignores,
                "dl.from_version": String(from),
                "dl.to_version": String(to),
            },
        }, parentContext);
        try {
            const call = api_1.context.with(api_1.trace.setSpan(parentContext, span), () => this._client.get({
                project,
                fromVersion: from,
                toVersion: to,
                queries: [
                    {
                        path,
                        ignores,
                        isPrefix: true,
                        subpaths: [],
                    },
                ],
            }, this._rpcOptions()));
            for await (const response of call.responses) {
                if (response.object) {
                    yield response.object;
                }
            }
            await call.status;
        }
        finally {
            span.end();
        }
    }
    /**
     * Get objects.
     * @param project The id of the project.
     * @param path    The path to get objects under.
     * @param ignores The paths under {@link path} to ignore.
     * @param from    The project version to start from.
     * @param to      The project version to end at.
     * @param maxSize The maximum file size at which the content of the file is sent for.
     * @returns       All the objects under {@link path}.
     * @example
     * const response = await client.getObjects(1n, "");
     * for (const object of response.objects) {
     *   console.log("[getObjects] path: " + object.path);
     *   console.log("[getObjects] content:\n" + object.content);
     * }
     */
    async getObjects(project, path, ignores = [], from, to, maxSize) {
        return await (0, telemetry_1.trace)("dateilager-grpc-client.get-unary", {
            attributes: {
                "dl.project": String(project),
                "dl.path": path,
                "dl.ignores": ignores,
                "dl.from_version": String(from),
                "dl.to_version": String(to),
                "dl.max_content_send_size": String(maxSize),
            },
        }, async () => {
            const call = this._client.getUnary({
                project,
                fromVersion: from,
                toVersion: to,
                queries: [{ path, ignores, isPrefix: true, subpaths: [] }],
                maxContentSendSize: maxSize,
            }, this._rpcOptions());
            return await call.response;
        });
    }
    /**
     * Get an object.
     * @param project The id of the project.
     * @param path    The path of the object.
     * @param from    The project version to start from.
     * @param to      The project version to end at.
     * @returns       The object at the given path or undefined if it does not exist.
     */
    async getObject(project, path, from, to) {
        return await (0, telemetry_1.trace)("dateilager-grpc-client.get-object", {
            attributes: {
                "dl.project": String(project),
                "dl.path": path,
                "dl.from_version": String(from),
                "dl.to_version": String(to),
            },
        }, async () => {
            const call = this._client.get({
                project,
                fromVersion: from,
                toVersion: to,
                queries: [
                    {
                        path,
                        isPrefix: false,
                        ignores: [],
                        subpaths: [],
                    },
                ],
            }, this._rpcOptions());
            let object;
            for await (const response of call.responses) {
                object = response.object;
            }
            await call.status;
            return object;
        });
    }
    /**
     * Update objects.
     * @param project The id of the project.
     * @returns       An {@link UpdateInputStream} to send objects to update.
     */
    updateObjects(project) {
        const parentContext = api_1.context.active();
        const span = telemetry_1.tracer.startSpan("dateilager-grpc-client.update-objects", {
            attributes: {
                "dl.project": String(project),
            },
        }, parentContext);
        const call = api_1.context.with(api_1.trace.setSpan(parentContext, span), () => this._client.update(this._rpcOptions()));
        return new UpdateInputStream(project, call, span);
    }
    /**
     * Update an object.
     * @param project The id of the project.
     * @param obj     The object to update.
     * @returns       The latest project version or `null` if something went wrong.
     */
    async updateObject(project, obj) {
        const stream = this.updateObjects(project);
        await stream.send(obj);
        return await stream.complete();
    }
    /**
     * Rollback a project.
     * @param project The id of the project.
     * @param version The version to rollback to.
     */
    async rollbackProject(project, version) {
        await this._client.rollback({ project, version }, this._rpcOptions());
    }
    /**
     * Snapshot the current state of the dateilager server.
     * @returns All the projects on the dateilager server.
     * @throws If the dateilager server's DL_ENV environment variable is PROD.
     * @see DateiLagerGrpcClient.resetToSnapshotInDevOrTests
     */
    async snapshotInDevOrTests() {
        const call = await this._client.snapshot({}, this._rpcOptions());
        return call.response.projects;
    }
    /**
     * Reset the given projects to their respective versions and delete any remaining projects.
     * If no projects are provided, delete all projects.
     * @param projects The projects to reset.
     * @throws If the dateilager server's DL_ENV environment variable is PROD.
     * @see DateiLagerGrpcClient.snapshotInDevOrTests
     */
    async resetToSnapshotInDevOrTests(projects) {
        await this._client.reset({ projects }, this._rpcOptions());
    }
    /**
     * Clones the `source` projects changes (from `fromVersion` up to `toVersion`) to the `target` project.
     * This method assumes that it is always a one way clone from source to target, it does not take into account
     * the changes that have occurred in the `target` project.
     * @param source  The source project.
     * @param target  The target project.
     * @param version The version of the source project to clone up to.
     * @returns       The new version number of the target project.
     */
    async cloneToProject(source, target, version) {
        return await (0, telemetry_1.trace)("dateilager-grpc-client.clone-to-project", {
            attributes: {
                "dl.source": String(source),
                "dl.target": String(target),
                "dl.version": String(version),
            },
        }, async () => {
            const call = await this._client.cloneToProject({ source, target, version }, this._rpcOptions());
            return call.response;
        });
    }
    /**
     * GC project.
     * @param project The project to GC.
     * @param keep    The amount of versions since the latest that should be kept.
     * @param from    The starting version to GC from.
     * @returns       The amount of objects that were GC'd.
     */
    async gcProject(project, keep, from) {
        const call = await this._client.gcProject({
            project: project,
            keepVersions: keep,
            fromVersion: from,
        });
        return call.response.count;
    }
    /**
     * GC random projects.
     * @param sample The percentage of projects to sample from.
     * @param keep   The amount of versions since the latest that should be kept.
     * @param from   The starting version to GC from.
     * @returns      The amount of objects that were GC'd.
     */
    async gcRandomProjects(sample, keep, from) {
        const call = await this._client.gcRandomProjects({
            sample: sample,
            keepVersions: keep,
            fromVersion: from,
        });
        return call.response.count;
    }
    /**
     * GC contents.
     * @param sample The percentage of projects to sample from.
     * @returns      The amount of objects that were GC'd.
     */
    async gcContents(sample) {
        const call = await this._client.gcContents({
            sample: sample,
        });
        return call.response.count;
    }
}
exports.DateiLagerGrpcClient = DateiLagerGrpcClient;
/**
 * Used to send a stream of objects to update.
 * @see DateiLagerGrpcClient.updateObjects
 */
class UpdateInputStream {
    constructor(project, call, span) {
        /** @internal */
        Object.defineProperty(this, "_project", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        /** @internal */
        Object.defineProperty(this, "_call", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        /** @internal */
        Object.defineProperty(this, "_span", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        this._project = project;
        this._call = call;
        this._span = span;
    }
    /**
     * Send an object to update.
     * @param obj The object to update.
     */
    async send(obj) {
        try {
            await this._call.requests.send({
                project: this._project,
                object: obj,
            });
        }
        catch (err) {
            this._span.end();
            throw err;
        }
    }
    /**
     * Complete the update request.
     * @returns The latest project version or `null` if something went wrong.
     */
    async complete() {
        try {
            await this._call.requests.complete();
            const response = await this._call.response;
            return response.version != -1n ? response.version : null;
        }
        finally {
            this._span.end();
        }
    }
}
const encoder = new util_1.TextEncoder();
/**
 * Encode string object contents as an array of bytes.
 * @param content The string to encode.
 * @returns       The encoded content as an array of bytes.
 */
function encodeContent(content) {
    return encoder.encode(content);
}
exports.encodeContent = encodeContent;
const decoder = new util_1.TextDecoder();
/**
 * Decode an array of bytes as an object's string contents.
 * @param bytes The array of bytes to decode.
 * @returns     The bytes decoded into a string.
 */
function decodeContent(bytes) {
    return decoder.decode(bytes);
}
exports.decodeContent = decodeContent;
//# sourceMappingURL=grpc-client.js.map