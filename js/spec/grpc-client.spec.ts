import { DateiLagerGrpcClient, encodeContent } from "../src";
import type { Project } from "../src/pb/fs_pb";

const devAdminToken =
  "v2.public.eyJzdWIiOiJhZG1pbiIsImlhdCI6IjIwMjEtMTAtMTVUMTE6MjA6MDAuMDM0WiJ9WtEey8KfQQRy21xoHq1C5KQatEevk8RxS47k4bRfMwVCPHumZmVuk6ADcfDHTmSnMtEGfFXdxnYOhRP6Clb_Dw";

describe("grpc client operations", () => {
  const client = new DateiLagerGrpcClient({ server: "localhost:5051", token: devAdminToken });

  let snapshot: Project[];

  beforeEach(async () => {
    snapshot = await client.snapshotInDevOrTests();
  });

  afterEach(async () => {
    await client.resetToSnapshotInDevOrTests(snapshot);
  });

  afterAll(() => {
    client.close();
  });

  test("create and read an object", async () => {
    await client.newProject(1337n, []);
    const content = encodeContent("a v1");

    await client.updateObject(1337n, {
      path: "a",
      mode: 0o755n,
      content: content,
      size: BigInt(content.length),
      deleted: false,
    });

    const result = await client.getObject(1337n, "a");

    expect(result?.content?.toString()).toBe("a v1");
  });
});
