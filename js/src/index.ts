import { Readable, Transform, Writable } from "stream";
import * as util from "util";

import * as grpc from "@grpc/grpc-js";

import { GetRequest, ObjectQuery, UpdateRequest } from "./fs_pb";
import * as pb from "./fs_pb";
import { FsClient } from "./fs_grpc_pb";

interface Object {
  path: string;
  mode: number;
  deleted: boolean;
  content: string;
}

function objectToPb(source: Object): pb.Object {
  const encoder = new util.TextEncoder();

  const obj = new pb.Object();
  obj.setPath(source.path);
  obj.setMode(source.mode);
  obj.setDeleted(source.deleted);
  obj.setContent(encoder.encode(source.content));

  return obj;
}

type UpdateCallback = {
  (err: Error | null, version: number): void;
};

class DateiLagerClient {
  project: number;
  client: any;

  constructor(host: string, port: number, project: number) {
    this.project = project;
    this.client = new FsClient(
      host + ":" + port,
      grpc.credentials.createInsecure()
    );
  }

  _buildGetRequest(
    paths: string[],
    isPrefix: boolean,
    withContent: boolean
  ): GetRequest {
    const queries = paths.map((path) => {
      const query = new ObjectQuery();
      query.setPath(path);
      query.setIsPrefix(isPrefix);
      query.setWithContent(withContent);
      return query;
    });

    const request = new GetRequest();
    request.setProject(this.project);
    request.setQueriesList(queries);

    return request;
  }

  _get(path: string, isPrefix: boolean, withContent: boolean): Readable {
    type TransformCallback = {
      (err: Error | null, update?: Object): void;
    };

    const decoder = new util.TextDecoder();

    const transformer = new Transform({
      objectMode: true,
      transform: (resp: pb.GetResponse, _: string, next: TransformCallback) => {
        const object = resp.getObject();
        if (!object) {
          return next(new Error("missing object"));
        }

        next(null, {
          path: object.getPath(),
          mode: object.getMode(),
          deleted: object.getDeleted(),
          content: decoder.decode(object.getContent_asU8()),
        });
      },
    });

    const stream = this.client.get(
      this._buildGetRequest([path], isPrefix, withContent)
    );

    return stream.pipe(transformer);
  }

  async getObject(path: string): Promise<Object> {
    const stream = this._get(path, false, true);
    for await (const object of stream) {
      return object;
    }
    throw new Error("object at path " + path + " not found");
  }

  async *listObjects(path: string) {
    const stream = this._get(path, true, true);
    for await (const object of stream) {
      yield object;
    }
  }

  updateObjects(): [Writable, Promise<number>] {
    type TransformCallback = {
      (err: Error | null, update: UpdateRequest): void;
    };

    const transformer = new Transform({
      objectMode: true,
      transform: (chunk: Object, _: string, next: TransformCallback) => {
        const update = new UpdateRequest();
        update.setProject(this.project);
        update.setObject(objectToPb(chunk));
        next(null, update);
      },
    });

    const promise = new Promise<number>((resolve, reject) => {
      transformer.pipe(
        this.client.update((err: Error, version: number) => {
          if (err) {
            reject(err);
          } else {
            resolve(version);
          }
        })
      );
    });

    return [transformer, promise];
  }

  async pack(path: string): Promise<number> {
    const pack = util.promisify(this.client.pack);
    return pack(path);
  }
}

async function main() {
  const client = new DateiLagerClient("localhost", 5051, 1);

  const object = await client.getObject("a");
  console.log("[getObject] path: " + object.path);
  console.log("[getObject] content:\n" + object.content);

  for await (const object of client.listObjects("")) {
    console.log("[listObjects] path: " + object.path);
    console.log("[listObjects] content:\n" + object.content);
  }

  const [updateStream, promise] = client.updateObjects();

  updateStream.write({
    path: "a",
    mode: 0o755,
    content: "foo bar",
  });

  updateStream.end();

  const version = await promise;
  console.log("[updateObject] version: " + version);

  for await (const object of client.listObjects("")) {
    console.log("[listObjects] path: " + object.path);
    console.log("[listObjects] content:\n" + object.content);
  }
}

main();

export { DateiLagerClient };
