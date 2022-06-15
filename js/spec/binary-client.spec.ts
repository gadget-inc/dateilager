import * as fs from "fs";
import { encodeContent } from "../src";
import { binaryClient, grpcClient, tmpdir } from "./util";

describe("binary client operations", () => {
  const project = 1337n;
  const path = "hello.txt";
  let dir: string;

  beforeEach(async () => {
    dir = tmpdir();
    await grpcClient.newProject(project, []);
  });

  it("can rebuild the file system", async () => {
    const content = "hello world";
    const encodedContent = encodeContent(content);

    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    await binaryClient.rebuild(project, null, dir);

    const filepath = `${dir}/${path}`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });

  it("can update the file system", async () => {
    await binaryClient.rebuild(project, null, dir);

    const filepath = `${dir}/${path}`;
    fs.writeFileSync(filepath, "hello world", { encoding: "utf8" });

    const result = await binaryClient.update(project, dir);
    expect(result).toBeTruthy();
    expect(result!.version).toBe(1n);
    expect(result!.updates).toEqual([
      {
        operation: "ADD",
        path: path,
      },
    ]);
  });

  it("updates where nothing change return a result", async () => {
    await binaryClient.rebuild(project, null, dir);
    const result = await binaryClient.update(project, dir);
    expect(result).toBeTruthy();
    expect(result!.version).toEqual(0n);
    expect(result!.updates).toEqual([]);
  });
});
