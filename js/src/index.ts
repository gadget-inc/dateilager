import { ChannelCredentials } from "@grpc/grpc-js";
import { GrpcTransport } from "@protobuf-ts/grpc-transport";
import { ClientStreamingCall } from "@protobuf-ts/runtime-rpc";
import { Objekt, UpdateRequest, UpdateResponse } from "./fs";
import { FsClient } from "./fs.client";

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

export class DateiLagerClient {
  client: FsClient;

  constructor(host: string, port: number) {
    const transport = new GrpcTransport({
      host: host + ":" + port,
      channelCredentials: ChannelCredentials.createInsecure(),
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

  async resetAllInDevOrTests(): Promise<void> {
    await this.client.reset(this._options());
  }
}
