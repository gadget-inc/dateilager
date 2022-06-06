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
    await binaryClient.rebuild(project, null, dir);

    const filepath = `${dir}/${path}`;
    expect(fs.existsSync(filepath)).toBe(true);

    const fileContent = fs.readFileSync(filepath).toString();
    expect(fileContent).toBe(content);
  });
});
