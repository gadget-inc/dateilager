import crypto from "crypto";
import { decodeContent, encodeContent } from "../src";
import { grpcClient } from "./util";

describe("grpc client operations", () => {
  afterEach(async () => {
    await grpcClient.deleteProject(1337n);
  });

  it("can create and read an object", async () => {
    await grpcClient.newProject(1337n, []);
    const content = encodeContent("a v1");

    await grpcClient.updateObject(1337n, {
      path: "a",
      mode: 0o755n,
      content: content,
      size: BigInt(content.length),
      deleted: false,
    });

    const result = await grpcClient.getObject(1337n, "a");

    expect(result?.content?.toString()).toBe("a v1");
  });

  it("can create and read multiple object", async () => {
    const projectId = 1337n;
    await grpcClient.newProject(projectId, []);

    const objects = [];
    for (let i = 0; i < 20; i++) {
      const content = crypto.randomBytes(64).toString("hex");
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

    const response = await grpcClient.getObjects(projectId, "");
    const receivedObjects = response.objects
      .map((object) => ({ ...object, content: decodeContent(object.content) }))
      .sort((a, b) => {
        const aNum = parseInt(a.path.split("-")[1]!.slice(0, -4));
        const bNum = parseInt(b.path.split("-")[1]!.slice(0, -4));
        return aNum - bNum;
      });

    expect(receivedObjects).toEqual(objects);
  });

  it("can rollback a project", async () => {
    await grpcClient.newProject(1337n, []);
    const originalValue = encodeContent("a v1");
    const updatedValue = encodeContent("a v2");

    let version = await grpcClient.updateObject(1337n, {
      path: "a",
      mode: 0o755n,
      content: originalValue,
      size: BigInt(originalValue.length),
      deleted: false,
    });

    expect(version).toBe(1n);

    version = await grpcClient.updateObject(1337n, {
      path: "a",
      mode: 0o755n,
      content: updatedValue,
      size: BigInt(updatedValue.length),
      deleted: false,
    });

    expect(version).toBe(2n);

    await grpcClient.rollbackProject(1337n, 1n);

    const result = await grpcClient.getObject(1337n, "a");

    expect(result?.content?.toString()).toBe("a v1");
  });

  it("can GC updated objects", async () => {
    await grpcClient.newProject(1337n, []);

    for (let i = 0; i < 20; i++) {
      const content = encodeContent(`a v${i}`);
      await grpcClient.updateObject(1337n, {
        path: "a",
        mode: 0o755n,
        content: content,
        size: BigInt(content.length),
        deleted: false,
      });
    }

    const result = await grpcClient.gcProject(1337n, 5n, 2n);

    expect(result).toBe(12n);
  });

  it("throws a proper error when recreating the same project", async () => {
    await grpcClient.newProject(1337n, []);

    let errorCaught;
    try {
      await grpcClient.newProject(1337n, []);
      expect(true).toBe(false);
    } catch (error) {
      errorCaught = error;
    }
    expect((errorCaught as Error).message).toBe("project id 1337 already exists");
  });
});
