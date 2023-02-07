import * as fs from "fs";
import { encodeContent } from "../src";
import { binaryClient, grpcClient, tmpdir } from "./util";

describe("binary client operations", () => {
  const path = "hello.txt";
  let dir: string;
  const content = "hello world";
  const encodedContent = encodeContent(content);

  let project: bigint;
  let counter = 1;

  beforeEach(async () => {
    dir = tmpdir();
    counter += 1;
    project = BigInt(counter);
    await grpcClient.newProject(project, []);
  });

  it("can rebuild the file system", async () => {
    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    const result = await binaryClient.rebuild(project, null, dir);
    expect(result.version).toBeDefined();
    expect(result.updates).toBeUndefined();
    expect(result.globsMatched).toBeUndefined();

    const filepath = `${dir}/${path}`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });

  it("can rebuild the file system and list changed files", async () => {
    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    const result = await binaryClient.rebuild(project, null, dir, { listUpdated: true });
    expect(result.version).toBeDefined();
    expect(result.updates).toEqual([
      {
        operation: "ADD",
        path: "hello.txt",
      },
    ]);
  });

  it("can rebuild the file system and check changed files against a list of globs", async () => {
    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    const result = await binaryClient.rebuild(project, null, dir, { checkGlobs: ["hello.*", "goodbye.*"] });
    expect(result.version).toBeDefined();
    expect(result.globsMatched).toEqual(true);
  });

  it("can rebuild the file system and check changed files against a list of globs where none match", async () => {
    await grpcClient.updateObject(project, {
      path,
      mode: 0o755n,
      content: encodedContent,
      size: BigInt(encodedContent.length),
      deleted: false,
    });

    const result = await binaryClient.rebuild(project, null, dir, { checkGlobs: ["foo/**/bar", "baz"] });
    expect(result.version).toBeDefined();
    expect(result.globsMatched).toEqual(false);
  });

  it("can update the file system", async () => {
    await binaryClient.rebuild(project, null, dir);

    const filepath = `${dir}/${path}`;
    fs.writeFileSync(filepath, "hello world", { encoding: "utf8" });

    const result = await binaryClient.update(project, dir);
    expect(result).toBeTruthy();
    expect(result!.version).toBe(1n);
    expect(result!.updates).toBeFalsy();
  });

  it("can update the file system and return updates in a list", async () => {
    await binaryClient.rebuild(project, null, dir);

    fs.writeFileSync(`${dir}/a`, "a content", { encoding: "utf8" });
    fs.writeFileSync(`${dir}/b`, "b content", { encoding: "utf8" });

    const result = await binaryClient.update(project, dir, { listUpdated: true });
    expect(result).toBeTruthy();
    expect(result!.version).toBe(1n);
    expect(sortUpdates(result!.updates!)).toEqual(
      sortUpdates([
        {
          operation: "ADD",
          path: "a",
        },
        {
          operation: "ADD",
          path: "b",
        },
      ])
    );
  });

  it("can update the file system checking file globs for matches", async () => {
    await binaryClient.rebuild(project, null, dir);

    fs.writeFileSync(`${dir}/a`, "a content", { encoding: "utf8" });
    fs.writeFileSync(`${dir}/b`, "b content", { encoding: "utf8" });

    const result = await binaryClient.update(project, dir, { checkGlobs: ["a"] });
    expect(result).toBeTruthy();
    expect(result!.version).toBe(1n);
    expect(result!.globsMatched).toBeTruthy();
  });

  it("can update the file system and log updates when a directory turns into a file", async () => {
    await binaryClient.rebuild(project, null, dir);

    fs.mkdirSync(`${dir}/a`);

    let result = await binaryClient.update(project, dir, { listUpdated: true });
    expect(result).toBeTruthy();
    expect(result!.version).toBe(1n);
    expect(result!.updates!.sort()).toEqual([
      {
        operation: "ADD",
        path: "a/",
      },
    ]);

    fs.rmdirSync(`${dir}/a`);
    fs.writeFileSync(`${dir}/a`, "a content", { encoding: "utf8" });

    result = await binaryClient.update(project, dir, { listUpdated: true });
    expect(result).toBeTruthy();
    expect(result!.version).toBe(2n);
    expect(sortUpdates(result!.updates!)).toEqual(
      sortUpdates([
        {
          operation: "REMOVE",
          path: "a/",
        },
        {
          operation: "ADD",
          path: "a",
        },
      ])
    );
  });

  it("updates where nothing change return a result", async () => {
    await binaryClient.rebuild(project, null, dir);
    const result = await binaryClient.update(project, dir, { listUpdated: true });
    expect(result).toBeTruthy();
    expect(result!.version).toEqual(0n);
    expect(result!.updates!).toEqual([]);
  });
});

const sortUpdates = (updates: { path: string; operation: string }[]) => {
  return updates.sort((a, b) => {
    const aString = JSON.stringify(a);
    const bString = JSON.stringify(b);
    if (aString < bString) {
      return -1;
    }
    if (aString > bString) {
      return 1;
    }
    return 0;
  });
};
