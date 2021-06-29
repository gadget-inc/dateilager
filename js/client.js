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

  buildGetRequest(paths, isPrefix, withContent) {
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
    return this.client.get(this.buildGetRequest([path], false, true));
  }

  listObjects(path) {
    return this.client.get(this.buildGetRequest([path], true, true));
  }
}

const client = new DateiLagerClient("localhost", 5051, 1);
const stream = client.listObjects("");

stream.on("data", function (data) {
  const object = data.object;
  console.log("-- data --");
  console.log("version: " + data.version);
  console.log(
    "path: " + object.path + ", size: " + object.size + ", mode: " + object.mode
  );
  console.log("content: '" + object.content.toString("utf-8") + "'");
});

stream.on("error", function (error) {
  console.log("-- error --");
  console.log(error);
});
