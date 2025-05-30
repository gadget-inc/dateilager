import { decodeContent, encodeContent } from "../src";
import { buildTestFiles, grpcClient } from "./util";

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

  it("can create and read multiple objects", async () => {
    const projectId = 1337n;
    await grpcClient.newProject(projectId, []);

    const objects = await buildTestFiles(64, 20, projectId);

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

  it("can read only objects with some subpaths", async () => {
    const projectId = 1337n;
    await grpcClient.newProject(projectId, []);

    await grpcClient.updateObject(1337n, {
      path: "foo/a.txt",
      mode: 0o755n,
      content: encodeContent("a v1"),
      size: BigInt(1),
      deleted: false,
    });

    await grpcClient.updateObject(1337n, {
      path: "bar/a.txt",
      mode: 0o755n,
      content: encodeContent("a v1"),
      size: BigInt(1),
      deleted: false,
    });

    const response = await grpcClient.getObjects(projectId, "", { subpaths: ["foo"] });
    const receivedObjects = response.objects
      .map((object) => ({ ...object, content: decodeContent(object.content) }))
      .sort((a, b) => {
        const aNum = parseInt(a.path.split("-")[1]!.slice(0, -4));
        const bNum = parseInt(b.path.split("-")[1]!.slice(0, -4));
        return aNum - bNum;
      });

    expect(receivedObjects).toEqual([
      {
        path: "foo/a.txt",
        mode: 0o755n,
        content: "a v1",
        size: BigInt(1),
        deleted: false,
      },
    ]);
  });

  it("can ignore objects in a subpath", async () => {
    const projectId = 1337n;
    await grpcClient.newProject(projectId, []);

    await grpcClient.updateObject(1337n, {
      path: "foo/a.txt",
      mode: 0o755n,
      content: encodeContent("a v1"),
      size: BigInt(1),
      deleted: false,
    });

    await grpcClient.updateObject(1337n, {
      path: "bar/a.txt",
      mode: 0o755n,
      content: encodeContent("a v1"),
      size: BigInt(1),
      deleted: false,
    });

    const response = await grpcClient.getObjects(projectId, "", { ignores: ["bar"] });
    const receivedObjects = response.objects
      .map((object) => ({ ...object, content: decodeContent(object.content) }))
      .sort((a, b) => {
        const aNum = parseInt(a.path.split("-")[1]!.slice(0, -4));
        const bNum = parseInt(b.path.split("-")[1]!.slice(0, -4));
        return aNum - bNum;
      });

    expect(receivedObjects).toEqual([
      {
        path: "foo/a.txt",
        mode: 0o755n,
        content: "a v1",
        size: BigInt(1),
        deleted: false,
      },
    ]);
  });

  it("can create and read multiple objects with content size limit set", async () => {
    const projectId = 1337n;
    await grpcClient.newProject(projectId, []);

    const objects = await buildTestFiles(64, 10, projectId);

    const response = await grpcClient.getObjects(projectId, "", { maxContentSendSize: 256n });
    const receivedObjects = response.objects
      .map((object) => ({ ...object, content: decodeContent(object.content) }))
      .sort((a, b) => {
        const aNum = parseInt(a.path.split("-")[1]!.slice(0, -4));
        const bNum = parseInt(b.path.split("-")[1]!.slice(0, -4));
        return aNum - bNum;
      });

    expect(receivedObjects).toEqual(objects);
  });

  it("doesn't return content for objects over limit with size limit set", async () => {
    const projectId = 1337n;
    await grpcClient.newProject(projectId, []);

    const smallObjects = await buildTestFiles(32, 10, projectId);
    const largeObjects = await buildTestFiles(64, 10, projectId, 10);

    largeObjects.forEach((o) => {
      o.content = "";
    });

    const objects = smallObjects.concat(largeObjects);

    const response = await grpcClient.getObjects(projectId, "", { maxContentSendSize: 100n });
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
