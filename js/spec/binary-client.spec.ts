import crypto from "crypto";
import * as fs from "fs";
import { Client } from "pg";
import { encodeContent } from "../src";
import { binaryClient, grpcClient, tmpdir } from "./util";

beforeEach(async () => {
  const client = new Client({
    user: "postgres",
    password: "password",
    host: "127.0.0.1",
    database: "dl",
    port: 5432,
  });

  await client.connect();
  const statement = "truncate dl.objects; truncate dl.contents; truncate dl.projects; truncate dl.cache_versions";

  await client.query(statement);
  await client.end();
});

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

  it("can rebuild the file system with a file matcher", async () => {
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
    const result = await binaryClient.rebuild(project, null, dir, { matchInclude: "hello*" });
    expect(result?.version).toBe(1n);
    expect(result?.fileMatch).toBe(true);

    const filepath = `${dir}/${path}`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });

  it("can rebuild the file system with a file matcher exclude pattern", async () => {
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
    const result = await binaryClient.rebuild(project, null, dir, { matchExclude: "bar*" });
    expect(result?.version).toBe(2n);
    expect(result?.fileMatch).toBe(false);

    const filepath = `${dir}/bar.txt`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });

  it("can gc random-projects and return the count cleaned up", async () => {
    const result = await binaryClient.gcRandomProjects(90, 2, -1, { timeout: 90 });

    expect(JSON.stringify(result)).toMatch('{"count":0}');
    expect(result.count).toStrictEqual(0);
  });

  it("can gc a specific project and return the count cleaned up", async () => {
    const result = await binaryClient.gcProject(1, 2, -1, { timeout: 90 });

    expect(JSON.stringify(result)).toMatch('{"count":0}');
    expect(result.count).toStrictEqual(0);
  });

  it("can gc contents and successfully return the count of contents cleaned up", async () => {
    const result = await binaryClient.gcContents(90, { timeout: 90 });

    expect(JSON.stringify(result)).toMatch('{"count":0}');
    expect(result.count).toStrictEqual(0);
  });

  it("can timeout while rebuilding the file system", async () => {
    const project = 1337n;

    await grpcClient.newProject(project, []);

    const stream = grpcClient.updateObjects(project);
    for (let i = 0; i < 1000; i++) {
      const content = encodeContent(crypto.randomBytes(512 * 1024).toString("hex"));
      await stream.send({
        path: `hello-${i}.txt`,
        mode: 0o755n,
        content: content,
        size: BigInt(content.length),
        deleted: false,
      });
    }
    await stream.complete();

    const dir = tmpdir();
    const rebuildPromise = binaryClient.rebuild(project, null, dir, { timeout: 1 });
    await expect(rebuildPromise).rejects.toThrow(/context deadline exceeded/);
  }, 20_000);
});

describe("Gadget file match tests", () => {
  const writeFilesWithMatcher = async (project: bigint, include: string, exclude: string, paths: string[]): Promise<boolean> => {
    const content = "example";
    const encodedContent = encodeContent(content);

    await grpcClient.newProject(project, []);

    const stream = grpcClient.updateObjects(project);
    for (const path of paths) {
      await stream.send({
        path,
        mode: 0o755n,
        content: encodedContent,
        size: BigInt(encodedContent.length),
        deleted: false,
      });
    }
    await stream.complete();

    const dir = tmpdir();
    const result = await binaryClient.rebuild(project, null, dir, { matchInclude: include, matchExclude: exclude });

    fs.rmSync(dir, { recursive: true });

    return result.fileMatch;
  };

  it("can support Gadget's pattern", async () => {
    let result = await writeFilesWithMatcher(5n, "frontend/**", "frontend/vite.config.[jt]s", [
      "frontend/example.js",
      "frontend/other.css",
      "frontend/third.jsx",
    ]);
    expect(result).toBe(true);

    result = await writeFilesWithMatcher(6n, "frontend/**", "frontend/vite.config.[jt]s", [
      "frontend/example.js",
      "frontend/other.css",
      "frontend/third.jsx",
      "frontend/vite.config.js",
    ]);
    expect(result).toBe(false);

    result = await writeFilesWithMatcher(7n, "frontend/**", "frontend/vite.config.[jt]s", [
      "frontend/example.js",
      "frontend/other.css",
      "frontend/third.jsx",
      "frontend/vite.config.ts",
    ]);
    expect(result).toBe(false);

    result = await writeFilesWithMatcher(8n, "frontend/**", "frontend/vite.config.[jt]s", [
      "frontend/example.js",
      "frontend/other.css",
      "model/effect.js",
    ]);
    expect(result).toBe(false);
  });
});
