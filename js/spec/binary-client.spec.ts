import * as fs from "fs";
import { encodeContent } from "../src";
import { binaryClient, grpcClient, tmpdir } from "./util";

describe("binary client operations", () => {
  it("can rebuild the file system", async () => {
    const project = 1337n;
    const path = "hello.txt";
    const content = "hello world";

    await grpcClient.newProject(project, []);
    const encodedContent = encodeContent(content);

    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    const dir = tmpdir();
    const result = await binaryClient.rebuild(project, null, dir);
    expect(result?.version).toBe(1n);

    const filepath = `${dir}/${path}`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });

  it("can rebuild the file system with a file pattern", async () => {
    const project = 1337n;
    const path = "hello.txt";
    const content = "hello world";

    await grpcClient.newProject(project, []);
    const encodedContent = encodeContent(content);

    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    const dir = tmpdir();
    const result = await binaryClient.rebuild(project, null, dir, { filePattern: "hello*" });
    expect(result?.version).toBe(1n);
    expect(result?.patternMatch).toBe(true);

    const filepath = `${dir}/${path}`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });

  it("can rebuild the file system with an iff file pattern", async () => {
    const project = 1337n;
    const path = "hello.txt";
    const content = "hello world";

    await grpcClient.newProject(project, []);
    const encodedContent = encodeContent(content);

    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    await grpcClient.updateObject(project, {
      path: "bar.txt",
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    const dir = tmpdir();
    const result = await binaryClient.rebuild(project, null, dir, { filePattern: "hello*", filePatternIff: true });
    expect(result?.version).toBe(2n);
    expect(result?.patternMatch).toBe(false);

    const filepath = `${dir}/bar.txt`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });
});
