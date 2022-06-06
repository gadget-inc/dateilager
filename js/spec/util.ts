import * as fs from "fs";
import path from "path";
import { DateiLagerBinaryClient, DateiLagerGrpcClient } from "../src";

export const devAdminToken =
  "v2.public.eyJzdWIiOiJhZG1pbiIsImlhdCI6IjIwMjEtMTAtMTVUMTE6MjA6MDAuMDM0WiJ9WtEey8KfQQRy21xoHq1C5KQatEevk8RxS47k4bRfMwVCPHumZmVuk6ADcfDHTmSnMtEGfFXdxnYOhRP6Clb_Dw";

export const grpcClient = new DateiLagerGrpcClient({
  server: "localhost:5051",
  token: devAdminToken,
});

export const binaryClient = new DateiLagerBinaryClient({
  server: "localhost:5051",
  token: devAdminToken,
  command: path.join(__dirname, "..", "..", "bin", "client"),
});

export function tmpdir(): string {
  const name = Date.now();
  const dir = path.resolve(__dirname, "..", "..", "tmp", "tests", "js", name.toString());
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}
