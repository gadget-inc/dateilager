import { ChannelCredentials } from "@grpc/grpc-js";
import { GrpcTransport } from "@protobuf-ts/grpc-transport";
import { FsClient } from "./fs.client";
import { Object, UpdateRequest, UpdateResponse } from "./fs";
import { ClientStreamingCall, RpcInputStream } from "@protobuf-ts/runtime-rpc";

class UpdateInputStream {
  project: bigint
  call: ClientStreamingCall<UpdateRequest, UpdateResponse>

  constructor(project: bigint, call: ClientStreamingCall<UpdateRequest, UpdateResponse>) {
    this.project = project
    this.call = call
  }

  send(obj: Object): Promise<void> {
    return this.call.requests.send({
      project: this.project,
      object: obj
    })
  }

  async complete(): Promise<bigint> {
    await this.call.requests.complete()
    const response = await this.call.response
    return response.version
  }
}

class DateiLagerClient {
  client: FsClient;

  constructor(host: string, port: number) {
    const transport = new GrpcTransport({
      host: host + ":" + port,
      channelCredentials: ChannelCredentials.createInsecure(),
    });
    this.client = new FsClient(transport);
  }

  async newProject(project: bigint) {
    await this.client.newProject({id: project})
  }

  async *listObjects(project: bigint, path: string) {
    const call = this.client.get({
      project: project,
      queries: [
        {
          path: path,
          isPrefix: true,
          withContent: true
        }
      ]
    })

    for await (const response of call.responses) {
      yield response.object
    }
  }

  async getObject(project: bigint, path: string): Promise<Object | undefined> {
    const call = this.client.get({
      project: project,
      queries: [
        {
          path: path,
          isPrefix: false,
          withContent: true
        }
      ]
    })

    for await (const response of call.responses) {
      return response.object
    }
  }

  updateObjects(project: bigint): UpdateInputStream {
    const call = this.client.update();
    return new UpdateInputStream(project, call)
  }

  updateObject(project: bigint, obj: Object): Promise<bigint> {
    const stream = this.updateObjects(project)
    stream.send(obj)
    return stream.complete()
  }
}


async function main() {
  const dl = new DateiLagerClient("localhost", 5051)

  for await (const obj of dl.listObjects(1n, "")) {
    console.log("object.path: " + obj?.path)
    console.log("object.content: " + obj?.content)
  }
}

main()
