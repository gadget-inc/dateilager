import type { ClientOptions } from "@grpc/grpc-js";
import { ChannelCredentials, credentials, Metadata } from "@grpc/grpc-js";
import type { Span } from "@opentelemetry/api";
import { context as contextAPI, trace as traceAPI } from "@opentelemetry/api";
import { GrpcTransport } from "@protobuf-ts/grpc-transport";
import type { ClientStreamingCall, RpcOptions } from "@protobuf-ts/runtime-rpc";
import { TextDecoder, TextEncoder } from "util";
import { trace, tracer } from "./internal/telemetry";
import type { CloneToProjectResponse, Objekt, Project, UpdateRequest, UpdateResponse } from "./pb/fs_pb";
import { FsClient } from "./pb/fs_pb.client";

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
      clientOptions: options.grpcClientOptions,
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
                withContent: true,
                withHash: false,
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
   * Get an object.
   *
   * @param project The id of the project.
   * @param path    The path of the object.
   * @returns         The object at the given path or undefined if it does not exist.
   */
  public async getObject(project: bigint, path: string): Promise<Objekt | undefined> {
    return await trace(
      "dateilager-grpc-client.get-object",
      {
        attributes: {
          "dl.project": String(project),
          "dl.path": path,
        },
      },
      async () => {
        const call = this._client.get(
          {
            project,
            queries: [
              {
                path,
                isPrefix: false,
                withContent: true,
                ignores: [],
                withHash: false,
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
   * @param project The id of the project.
   * @returns         An {@link UpdateInputStream} to send objects to update.
   */
  public updateObjects(project: bigint): UpdateInputStream {
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

    return new UpdateInputStream(project, call, span);
  }

  /**
   * Update an object.
   *
   * @param project The id of the project.
   * @param obj     The object to update.
   * @returns         The latest project version or `null` if something went wrong.
   */
  public async updateObject(project: bigint, obj: Objekt): Promise<bigint | null> {
    const stream = this.updateObjects(project);
    await stream.send(obj);
    return await stream.complete();
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
   * Clones the `source`projects changes (from `fromVersion` up to `toVersion`) to the `target` project. This method assumes that it is always a one way clone from source to target, it does not take into account the changes that have occurred in the `target` project.
   *
   * @param source      The source project.
   * @param target      The target project.
   * @param fromVersion Start version of the source project.
   * @param toVersion   Stop version of the source project.
   * @returns             The new version number of hte target project
   */
  public async cloneToProject(source: bigint, target: bigint, fromVersion: bigint, toVersion: bigint): Promise<CloneToProjectResponse> {
    return await trace(
      "dateilager-grpc-client.clone-to-project",
      {
        attributes: {
          "dl.source": String(source),
          "dl.target": String(target),
          "dl.fromVersion": String(fromVersion),
          "dl.toVersion": String(toVersion),
        },
      },
      async () => {
        const call = await this._client.cloneToProject({ source, target, fromVersion, toVersion }, this._rpcOptions());

        return call.response;
      }
    );
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
  private readonly _call: ClientStreamingCall<UpdateRequest, UpdateResponse>;

  /** @internal */
  private readonly _span: Span;

  public constructor(project: bigint, call: ClientStreamingCall<UpdateRequest, UpdateResponse>, span: Span) {
    this._project = project;
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
