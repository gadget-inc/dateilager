import { ChannelCredentials, credentials, Metadata } from "@grpc/grpc-js";
import type { Span } from "@opentelemetry/api";
import { context, propagation, trace } from "@opentelemetry/api";
import { GrpcTransport } from "@protobuf-ts/grpc-transport";
import type { ClientStreamingCall, RpcOptions } from "@protobuf-ts/runtime-rpc";
import pMemoize from "p-memoize";
import { TextDecoder, TextEncoder } from "util";
import type { Objekt, Project, UpdateRequest, UpdateResponse } from "./fs";
import { FsClient } from "./fs.client";
import { traced, tracer } from "./telemetry";

const MB = 1024 * 1024;

class UpdateInputStream {
  constructor(private project: bigint, private call: ClientStreamingCall<UpdateRequest, UpdateResponse>, private span: Span) {}

  send(obj: Objekt): Promise<void> {
    try {
      return this.call.requests.send({
        project: this.project,
        object: obj,
      });
    } catch (err) {
      this.span.end();
      throw err;
    }
  }

  async complete(): Promise<bigint | null> {
    try {
      await this.call.requests.complete();
      const response = await this.call.response;
      return response.version != -1n ? response.version : null;
    } finally {
      this.span.end();
    }
  }
}

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/**
 * Encode string object contents as an array of bytes.
 */
export function encodeContent(content: string): Uint8Array {
  return encoder.encode(content);
}

/**
 * Decode an array of bytes as an object's string contents.
 */
export function decodeContent(bytes: Uint8Array | undefined): string {
  return decoder.decode(bytes);
}

/**
 * A client class for interacting with DateiLager's GRPC API
 *
 * The library used to interact with GRPC creates connections lazily, this constructor will not
 * raise an error even if there is no service running at host:port.
 *
 * The DateiLager API surface area is as minimal as possible, convenience functions, such as getObject,
 * should be implemented within the client.
 */
export class DateiLagerClient {
  client: FsClient;
  transport: GrpcTransport;

  constructor(host: string, port: number, tokenFn: () => Promise<string>, rootCert?: Buffer) {
    const memoizedTokenFn = pMemoize(tokenFn);

    const tokenMetaGenerator = (_params: any, callback: (err: Error | null, meta: Metadata) => void) => {
      memoizedTokenFn()
        .then((token) => {
          const meta = new Metadata();
          meta.add("authorization", `Bearer ${token}`);
          callback(null, meta);
        })
        .catch((error) => {
          throw error;
        });
    };

    const creds = credentials.combineChannelCredentials(
      ChannelCredentials.createSsl(rootCert),
      credentials.createFromMetadataGenerator(tokenMetaGenerator)
    );

    this.transport = new GrpcTransport({
      host: host + ":" + port,
      channelCredentials: creds,
      clientOptions: {
        "grpc.max_send_message_length": 100 * MB,
        "grpc.max_receive_message_length": 100 * MB,
      },
    });

    this.client = new FsClient(this.transport);
  }

  _options(): RpcOptions {
    const meta = {};
    propagation.inject(context.active(), meta);

    return {
      deadline: new Date().setSeconds(new Date().getSeconds() + 2),
      meta,
    };
  }

  close() {
    this.transport.close();
  }

  @traced("dateilager-grpc-client.new-project", {
    record: (project, pack_patterns, template) => ({
      project,
      pack_patterns,
      template,
    }),
  })
  async newProject(project: bigint, packPatterns: string[], template?: bigint) {
    await this.client.newProject({ id: project, packPatterns: packPatterns, template: template }, this._options());
  }

  @traced("dateilager-grpc-client.delete-project", {
    record: (project) => ({ project }),
  })
  async deleteProject(project: bigint): Promise<void> {
    await this.client.deleteProject({ project }, this._options());
  }

  async *listObjects(project: bigint, path: string, ignores: string[] = [], from?: bigint, to?: bigint) {
    const parentContext = context.active();
    const span = tracer.startSpan(
      "dateilager-grpc-client.list-objects",
      {
        attributes: {
          "dl.project": project.toString(),
          "dl.path": path,
          "dl.ignores": ignores,
          "dl.from_version": String(from),
          "dl.to_version": String(to),
        },
      },
      parentContext
    );

    try {
      const call = context.with(trace.setSpan(parentContext, span), () =>
        this.client.get(
          {
            project: project,
            fromVersion: from,
            toVersion: to,
            queries: [
              {
                path: path,
                isPrefix: true,
                withContent: true,
                ignores: ignores,
              },
            ],
          },
          this._options()
        )
      );

      for await (const response of call.responses) {
        if (response.object) {
          yield response.object;
        }
      }
    } finally {
      span.end();
    }
  }

  @traced("dateilager-grpc-client.get-object", {
    record: (project, path) => ({ project, path }),
  })
  async getObject(project: bigint, path: string): Promise<Objekt | undefined> {
    const call = this.client.get(
      {
        project: project,
        queries: [
          {
            path: path,
            isPrefix: false,
            withContent: true,
            ignores: [],
          },
        ],
      },
      this._options()
    );

    for await (const response of call.responses) {
      return response.object;
    }
  }

  updateObjects(project: bigint): UpdateInputStream {
    const parentContext = context.active();
    const span = tracer.startSpan(
      "dateilager-grpc-client.update-objects",
      {
        attributes: {
          "dl.project": project.toString(10),
        },
      },
      parentContext
    );

    const call = context.with(trace.setSpan(parentContext, span), () => this.client.update(this._options()));
    return new UpdateInputStream(project, call, span);
  }

  async updateObject(project: bigint, obj: Objekt): Promise<bigint | null> {
    const stream = this.updateObjects(project);
    await stream.send(obj);
    return await stream.complete();
  }

  async snapshotInDevOrTests(): Promise<Project[]> {
    const response = await this.client.snapshot({}, this._options());
    return response.response.projects;
  }

  async resetToSnapshotInDevOrTests(projects: Project[]): Promise<void> {
    await this.client.reset({ projects: projects }, this._options());
  }

  async resetToK8SSnapshot(): Promise<void> {
    await this.client.reset({ projects: [{ id: 0n, version: 1n }] }, this._options());
  }

  async resetAllInDevOrTests(): Promise<void> {
    await this.client.reset({ projects: [] }, this._options());
  }
}
