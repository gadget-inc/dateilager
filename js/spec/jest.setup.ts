import type { Project } from "../src";
import { grpcClient } from "./util";

let snapshot: Project[];

beforeEach(async () => {
  snapshot = await grpcClient.snapshotInDevOrTests();
});

afterEach(async () => {
  await grpcClient.resetToSnapshotInDevOrTests(snapshot);
});

afterAll(() => {
  grpcClient.close();
});
