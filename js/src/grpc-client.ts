import type { ClientOptions } from "@grpc/grpc-js";
import { ChannelCredentials, credentials, Metadata } from "@grpc/grpc-js";
import type { Span } from "@opentelemetry/api";
import { context as contextAPI, trace as traceAPI } from "@opentelemetry/api";
import { GrpcTransport } from "@protobuf-ts/grpc-transport";
import { RpcError, type ClientStreamingCall, type RpcOptions } from "@protobuf-ts/runtime-rpc";
import { TextDecoder, TextEncoder } from "util";
import { trace, tracer } from "./internal/telemetry";
import type { CloneToProjectResponse, GetUnaryResponse, Objekt, Project, UpdateRequest, UpdateResponse } from "./pb/fs_pb";
import { FsClient } from "./pb/fs_pb.client";
import { ProjectAlreadyExistsError } from "./utils/errors";
export type { Objekt, Project };

/**
 * Options for {@link DateiLagerGrpcClient}.
 */
export interface DateiLagerGrpcClientOptions {
  /**
   * The address of the dateilager server.
   */
  server:
    | string
    | {
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
   *
   * @see ClientOptions
   */
  grpcClientOptions?: ClientOptions;

  /**
   * Options that will be passed to every remote procedure call.
   *
   * @see RpcOptions
   */
  rpcOptions?: RpcOptions | (() => RpcOptions | undefined);
}

/**
 * A client class for interacting with DateiLager's GRPC API.
 *
 * The DateiLager API surface area is as minimal as possible;
 * convenience functions, such as getObject, should be implemented within the client.
 */
export class DateiLagerGrpcClient {
  /** @internal */
  private readonly _client: FsClient;

  /** @internal */
  private readonly _transport: GrpcTransport;

  /** @internal */
  private readonly _rpcOptions: () => RpcOptions | undefined;

  /**
   * The library used to interact with GRPC creates connections lazily, this constructor will not
   * raise an error even if there is no service running at {@link DateiLagerGrpcClientOptions.server server}.
   *
   * @param options Grpc client options.
   */
  public constructor(options: DateiLagerGrpcClientOptions) {
    const tokenFn = typeof options.token === "string" ? () => Promise.resolve(options.token as string) : options.token;

    this._transport = new GrpcTransport({
      host: typeof options.server === "string" ? options.server : `${options.server.host}:${options.server.port}`,
      channelCredentials: credentials.combineChannelCredentials(
        ChannelCredentials.createSsl(),
        credentials.createFromMetadataGenerator((_, callback) => {
          tokenFn()
            .then((token) => {
              const meta = new Metadata();
              meta.add("authorization", `Bearer ${token}`);
              callback(null, meta);
            })
            .catch(callback);
        })
      ),
      clientOptions: {
        "grpc.keepalive_time_ms": 5_000,
        "grpc.keepalive_timeout_ms": 1_000,
        "grpc.keepalive_permit_without_calls": 1,
        ...options.grpcClientOptions,
      },
    });

    this._client = new FsClient(this._transport);

    this._rpcOptions = options.rpcOptions instanceof Function ? options.rpcOptions : () => options.rpcOptions as RpcOptions | undefined;
  }

  /**
   * Close the underlying GRPC client.
   */
  public close(): void {
    this._transport.close();
  }

  /**
   * Create a new project.
   *
   * @param project      The id of the project.
   * @param packPatterns The paths to pack.
   * @param template     The id of the project to start from.
   */
  public async newProject(project: bigint, packPatterns: string[], template?: bigint): Promise<void> {
    try {
      await trace(
        "dateilager-grpc-client.new-project",
        {
          attributes: {
            "dl.project": String(project),
            "dl.pack_patterns": packPatterns,
            "dl.template": String(template),
          },
        },
        () => this._client.newProject({ id: project, packPatterns, template }, this._rpcOptions())
      );
    } catch (error) {
      if (error instanceof RpcError && error.code == "ALREADY_EXISTS") {
        throw new ProjectAlreadyExistsError(`project id ${project} already exists`);
      }
      throw error;
    }
  }

  /**
   * Delete a project.
   *
   * @param project The id of the project.
   */
  public async deleteProject(project: bigint): Promise<void> {
    await trace(
      "dateilager-grpc-client.delete-project",
      {
        attributes: {
          "dl.project": String(project),
        },
      },
      () => this._client.deleteProject({ project }, this._rpcOptions())
    );
  }

  /**
   * List objects.
   *
   * @param project The id of the project.
   * @param path    The path to list objects under.
   * @param ignores The paths under {@link path} to ignore.
   * @param from    The project version to start from.
   * @param to      The project version to end at.
   * @returns         A stream of objects.
   * @yields           An object from the stream.
   * @example
   * for await (const object of client.listObjects(1n, "")) {
   *   console.log("[listObjects] path: " + object.path);
   *   console.log("[listObjects] content:\n" + object.content);
   * }
   */
  public async *listObjects(
    project: bigint,
    path: string,
    ignores: string[] = [],
    from?: bigint,
    to?: bigint
  ): AsyncGenerator<Objekt, void> {
    const parentContext = contextAPI.active();
    const span = tracer.startSpan(
      "dateilager-grpc-client.list-objects",
      {
        attributes: {
          "dl.project": String(project),
          "dl.path": path,
          "dl.ignores": ignores,
          "dl.from_version": String(from),
          "dl.to_version": String(to),
        },
      },
      parentContext
    );

    try {
      const call = contextAPI.with(traceAPI.setSpan(parentContext, span), () =>
        this._client.get(
          {
            project,
            fromVersion: from,
            toVersion: to,
            queries: [
              {
                path,
                ignores,
                isPrefix: true,
              },
            ],
          },
          this._rpcOptions()
        )
      );

      for await (const response of call.responses) {
        if (response.object) {
          yield response.object;
        }
      }

      await call.status;
    } finally {
      span.end();
    }
  }

  /**
   * Get objects.
   *
   * @param project The id of the project.
   * @param path    The path to get objects under.
   * @param ignores The paths under {@link path} to ignore.
   * @param from    The project version to start from.
   * @param to      The project version to end at.
   * @returns         All the objects under {@link path}.
   * @example
   * const response = await client.getObjects(1n, "");
   * for (const object of response.objects) {
   *   console.log("[getObjects] path: " + object.path);
   *   console.log("[getObjects] content:\n" + object.content);
   * }
   */
  public async getObjects(project: bigint, path: string, ignores: string[] = [], from?: bigint, to?: bigint): Promise<GetUnaryResponse> {
    return await trace(
      "dateilager-grpc-client.get-unary",
      {
        attributes: {
          "dl.project": String(project),
          "dl.path": path,
          "dl.ignores": ignores,
          "dl.from_version": String(from),
          "dl.to_version": String(to),
        },
      },
      async () => {
        const call = this._client.getUnary(
          { project, fromVersion: from, toVersion: to, queries: [{ path, ignores, isPrefix: true }] },
          this._rpcOptions()
        );
        return await call.response;
      }
    );
  }

  /**
   * Get an object.
   *
   * @param project The id of the project.
   * @param path    The path of the object.
   * @param from    The project version to start from.
   * @param to      The project version to end at.
   * @returns         The object at the given path or undefined if it does not exist.
   */
  public async getObject(project: bigint, path: string, from?: bigint, to?: bigint): Promise<Objekt | undefined> {
    return await trace(
      "dateilager-grpc-client.get-object",
      {
        attributes: {
          "dl.project": String(project),
          "dl.path": path,
          "dl.from_version": String(from),
          "dl.to_version": String(to),
        },
      },
      async () => {
        const call = this._client.get(
          {
            project,
            fromVersion: from,
            toVersion: to,
            queries: [
              {
                path,
                isPrefix: false,
                ignores: [],
              },
            ],
          },
          this._rpcOptions()
        );

        let object: Objekt | undefined;
        for await (const response of call.responses) {
          object = response.object;
        }

        await call.status;

        return object;
      }
    );
  }

  /**
   * Update objects.
   *
   * @param project  The id of the project.
   * @param isStaged If the update will be staged and committed later.
   * @returns          An {@link UpdateInputStream} to send objects to update.
   */
  public updateObjects(project: bigint, isStaged: boolean): UpdateInputStream {
    const parentContext = contextAPI.active();
    const span = tracer.startSpan(
      "dateilager-grpc-client.update-objects",
      {
        attributes: {
          "dl.project": String(project),
        },
      },
      parentContext
    );

    const call = contextAPI.with(traceAPI.setSpan(parentContext, span), () => this._client.update(this._rpcOptions()));

    return new UpdateInputStream(project, isStaged, call, span);
  }

  /**
   * Update an object.
   *
   * @param project The id of the project.
   * @param obj     The object to update.
   * @returns         The latest project version or `null` if something went wrong.
   */
  public async updateObject(project: bigint, obj: Objekt): Promise<bigint | null> {
    const stream = this.updateObjects(project, false);
    await stream.send(obj);
    return await stream.complete();
  }

  /**
   * Commit a staged update.
   *
   * @param project The id of the project.
   * @param version The version to commit.
   * @returns         The latest project version or `null` if something went wrong.
   */
  public async commitUpdate(project: bigint, version: bigint): Promise<void> {
    await this._client.commitUpdate({ project, version }, this._rpcOptions());
  }

  /**
   * Rollback a project.
   *
   * @param project The id of the project.
   * @param version The version to rollback to.
   */
  public async rollbackProject(project: bigint, version: bigint): Promise<void> {
    await this._client.rollback({ project, version }, this._rpcOptions());
  }

  /**
   * Snapshot the current state of the dateilager server.
   *
   * @returns All the projects on the dateilager server.
   * @throws If the dateilager server's DL_ENV environment variable is PROD.
   * @see DateiLagerGrpcClient.resetToSnapshotInDevOrTests
   */
  public async snapshotInDevOrTests(): Promise<Project[]> {
    const call = await this._client.snapshot({}, this._rpcOptions());
    return call.response.projects;
  }

  /**
   * Reset the given projects to their respective versions and delete any remaining projects.
   * If no projects are provided, delete all projects.
   *
   * @param projects The projects to reset.
   * @throws If the dateilager server's DL_ENV environment variable is PROD.
   * @see DateiLagerGrpcClient.snapshotInDevOrTests
   */
  public async resetToSnapshotInDevOrTests(projects: Project[]): Promise<void> {
    await this._client.reset({ projects }, this._rpcOptions());
  }

  /**
   * Clones the `source` projects changes (from `fromVersion` up to `toVersion`) to the `target` project.
   * This method assumes that it is always a one way clone from source to target, it does not take into account
   * the changes that have occurred in the `target` project.
   *
   * @param source  The source project.
   * @param target  The target project.
   * @param version The version of the source project to clone up to.
   * @returns         The new version number of the target project.
   */
  public async cloneToProject(source: bigint, target: bigint, version: bigint): Promise<CloneToProjectResponse> {
    return await trace(
      "dateilager-grpc-client.clone-to-project",
      {
        attributes: {
          "dl.source": String(source),
          "dl.target": String(target),
          "dl.version": String(version),
        },
      },
      async () => {
        const call = await this._client.cloneToProject({ source, target, version }, this._rpcOptions());
        return call.response;
      }
    );
  }

  /**
   * GC project.
   *
   * @param project The project to GC.
   * @param keep    The amount of versions since the latest that should be kept.
   * @param from    The starting version to GC from.
   * @returns         The amount of objects that were GC'd.
   */
  public async gcProject(project: bigint, keep: bigint, from?: bigint): Promise<bigint> {
    const call = await this._client.gcProject({
      project: project,
      keepVersions: keep,
      fromVersion: from,
    });
    return call.response.count;
  }

  /**
   * GC random projects.
   *
   * @param sample The percentage of projects to sample from.
   * @param keep   The amount of versions since the latest that should be kept.
   * @param from   The starting version to GC from.
   * @returns        The amount of objects that were GC'd.
   */
  public async gcRandomProjects(sample: number, keep: bigint, from?: bigint): Promise<bigint> {
    const call = await this._client.gcRandomProjects({
      sample: sample,
      keepVersions: keep,
      fromVersion: from,
    });
    return call.response.count;
  }

  /**
   * GC contents.
   *
   * @param sample The percentage of projects to sample from.
   * @returns        The amount of objects that were GC'd.
   */
  public async gcContents(sample: number): Promise<bigint> {
    const call = await this._client.gcContents({
      sample: sample,
    });
    return call.response.count;
  }
}

/**
 * Used to send a stream of objects to update.
 *
 * @see DateiLagerGrpcClient.updateObjects
 */
class UpdateInputStream {
  /** @internal */
  private readonly _project: bigint;

  /** @internal */
  private readonly _isStaged: boolean;

  /** @internal */
  private readonly _call: ClientStreamingCall<UpdateRequest, UpdateResponse>;

  /** @internal */
  private readonly _span: Span;

  public constructor(project: bigint, isStaged: boolean, call: ClientStreamingCall<UpdateRequest, UpdateResponse>, span: Span) {
    this._project = project;
    this._isStaged = isStaged;
    this._call = call;
    this._span = span;
  }

  /**
   * Send an object to update.
   *
   * @param obj The object to update.
   */
  public async send(obj: Objekt): Promise<void> {
    try {
      await this._call.requests.send({
        project: this._project,
        object: obj,
        isStaged: this._isStaged,
      });
    } catch (err) {
      this._span.end();
      throw err;
    }
  }

  /**
   * Complete the update request.
   *
   * @returns The latest project version or `null` if something went wrong.
   */
  public async complete(): Promise<bigint | null> {
    try {
      await this._call.requests.complete();
      const response = await this._call.response;
      return response.version != -1n ? response.version : null;
    } finally {
      this._span.end();
    }
  }
}

const encoder = new TextEncoder();

/**
 * Encode string object contents as an array of bytes.
 *
 * @param content The string to encode.
 * @returns         The encoded content as an array of bytes.
 */
export function encodeContent(content: string): Uint8Array {
  return encoder.encode(content);
}

const decoder = new TextDecoder();

/**
 * Decode an array of bytes as an object's string contents.
 *
 * @param bytes The array of bytes to decode.
 * @returns       The bytes decoded into a string.
 */
export function decodeContent(bytes: Uint8Array | undefined): string {
  return decoder.decode(bytes);
}
