import { encodeContent } from "../src";
import { grpcClient } from "./util";

describe("grpc client operations", () => {
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
});
