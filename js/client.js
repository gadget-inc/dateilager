import { Transform } from "stream";

import * as grpc from "@grpc/grpc-js";
import * as protoLoader from "@grpc/proto-loader";

const GRPC_OPTIONS = {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
};

class DateiLagerClient {
  FS_PROTO_PATH = "../internal/pb/fs.proto";

  constructor(host, port, project) {
    const fs = grpc.loadPackageDefinition(
      protoLoader.loadSync(this.FS_PROTO_PATH, GRPC_OPTIONS)
    ).pb;

    this.project = project;
    this.client = new fs.Fs(
      host + ":" + port,
      grpc.credentials.createInsecure()
    );
  }

  _buildGetRequest(paths, isPrefix, withContent) {
    const queries = paths.map((path) => {
      return {
        path: path,
        is_prefix: isPrefix,
        with_content: withContent,
      };
    });

    return {
      project: this.project,
      queries: queries,
    };
  }

  getObject(path) {
    return this.client.get(this._buildGetRequest([path], false, true));
  }

  listObjects(path) {
    return this.client.get(this._buildGetRequest([path], true, true));
  }

  updateObjects(callback) {
    const transformer = new Transform({
      objectMode: true,
      transform: (chunk, _encoding, next) => {
        next(null, {
          project: this.project,
          object: {
            path: chunk.path,
            mode: chunk.mode,
            size: chunk.content.length,
            deleted: Boolean(chunk.isDeleted),
            content: chunk.content,
          },
        });
      },
    });

    transformer.pipe(this.client.update(callback));
    return transformer;
  }

  pack(path, callback) {
    this.client.pack(
      {
        project: this.project,
        path: path,
      },
      function (err, response) {
        if (err) {
          return callback(err);
        }
        callback(null, response.version);
      }
    );
  }
}

export { DateiLagerClient }
