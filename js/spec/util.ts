import * as fs from "fs";
import path from "path";
import { DateiLagerBinaryClient, DateiLagerGrpcClient } from "../src";

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
