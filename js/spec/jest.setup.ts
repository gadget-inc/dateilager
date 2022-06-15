import { grpcClient } from "./util";

beforeEach(async () => {
  await grpcClient.resetToSnapshotInDevOrTests([]);
});

afterEach(async () => {});

afterAll(() => {
  grpcClient.close();
});
