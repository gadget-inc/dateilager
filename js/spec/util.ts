import crypto from "crypto";
import * as fs from "fs";
import path from "path";
import { DateiLagerBinaryClient, DateiLagerGrpcClient, encodeContent } from "../src";

export const devAdminToken =
  "v2.public.eyJzdWIiOiJhZG1pbiJ9yt40HNkcyOUtDeFa_WPS6vi0WiE4zWngDGJLh17TuYvssTudCbOdQEkVDRD-mSNTXLgSRDXUkO-AaEr4ZLO4BQ";

export const grpcClient = new DateiLagerGrpcClient({
  server: "localhost:5051",
  token: devAdminToken,
});

export const binaryClient = new DateiLagerBinaryClient({
  server: {
    host: "localhost",
  },
  token: devAdminToken,
  command: path.join(__dirname, "..", "..", "bin", "client"),
});

export function tmpdir(): string {
  const name = Date.now();
  const dir = path.resolve(__dirname, "..", "..", "tmp", "tests", "js", name.toString());
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

export async function buildTestFiles(size: number, count: number, projectId: bigint, offset?: number) {
  if (offset == undefined) {
    offset = 0;
  }
  const objects = [];
  for (let i = offset; i < count + offset; i++) {
    const content = crypto.randomBytes(size).toString("hex");
    objects.push({
      path: `file-${i}.txt`,
      mode: 0o755n,
      content: content,
      size: BigInt(content.length),
      deleted: false,
    });
  }

  const stream = grpcClient.updateObjects(projectId);
  await Promise.all(objects.map((object) => stream.send({ ...object, content: encodeContent(object.content) })));
  await stream.complete();

  return objects;
}
