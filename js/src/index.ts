import { ChannelCredentials } from "@grpc/grpc-js";
import { GrpcTransport } from "@protobuf-ts/grpc-transport";
import { ClientStreamingCall } from "@protobuf-ts/runtime-rpc";
import { TextEncoder } from "util";
import { Objekt, ProjectSnapshot, UpdateRequest, UpdateResponse } from "./fs";
import { FsClient } from "./fs.client";

const MB = 1024 * 1024;

class UpdateInputStream {
  project: bigint;
  call: ClientStreamingCall<UpdateRequest, UpdateResponse>;

  constructor(project: bigint, call: ClientStreamingCall<UpdateRequest, UpdateResponse>) {
    this.project = project;
    this.call = call;
  }

  send(obj: Objekt): Promise<void> {
    return this.call.requests.send({
      project: this.project,
      object: obj,
    });
  }

  async complete(): Promise<bigint> {
    await this.call.requests.complete();
    const response = await this.call.response;
    return response.version;
  }
}

/**
 * Encode string object contents as an array of bytes.
 */
export function encodeContent(content: string): Uint8Array {
  const encoder = new TextEncoder();
  return encoder.encode(content);
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

  constructor(host: string, port: number) {
    const transport = new GrpcTransport({
      host: host + ":" + port,
      channelCredentials: ChannelCredentials.createInsecure(),
      clientOptions: {
        'grpc.max_send_message_length': 50 * MB,
        'grpc.max_receive_message_length': 50 * MB,
      }
    });
    this.client = new FsClient(transport);
  }

  _options() {
    return {
      deadline: new Date().setSeconds(new Date().getSeconds() + 2),
    };
  }

  async newProject(project: bigint) {
    await this.client.newProject({ id: project }, this._options());
  }

  async *listObjects(project: bigint, path: string) {
    const call = this.client.get(
      {
        project: project,
        queries: [
          {
            path: path,
            isPrefix: true,
            withContent: true,
          },
        ],
      },
      this._options()
    );

    for await (const response of call.responses) {
      yield response.object;
    }
  }

  async getObject(project: bigint, path: string): Promise<Objekt | undefined> {
    const call = this.client.get(
      {
        project: project,
        queries: [
          {
            path: path,
            isPrefix: false,
            withContent: true,
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
    const call = this.client.update(this._options());
    return new UpdateInputStream(project, call);
  }

  async updateObject(project: bigint, obj: Objekt): Promise<bigint> {
    const stream = this.updateObjects(project);
    await stream.send(obj);
    return await stream.complete();
  }

  async snapshotInDevOrTests(): Promise<ProjectSnapshot[]> {
    const response = await this.client.snapshot({}, this._options());
    return response.response.projects;
  }

  async resetToSnapshotInDevOrTests(projects: ProjectSnapshot[]): Promise<void> {
    await this.client.reset({ projects: projects }, this._options());
  }

  async resetAllInDevOrTests(): Promise<void> {
    await this.client.reset({ projects: [] }, this._options());
  }
}
