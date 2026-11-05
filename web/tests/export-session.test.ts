import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({
  state: {
    role: 1,
    session: {
      accountId: 7 as number | null,
      group: "default" as string | null,
      epoch: 1,
    },
  },
  get: vi.fn(),
  post: vi.fn(),
  invalidate: vi.fn(),
}));
vi.mock("../src/lib/product-preview", () => {
  throw Error("Billing requests must not import the Demo store");
});
vi.mock("../src/lib/system-access", () => ({
  sessionQueryIdentity: (s: typeof fake.state) => [
    s.session.epoch,
    s.session.accountId,
    s.session.group,
    s.role,
  ],
  useSessionQueryIdentity: () => [
    fake.state.session.epoch,
    fake.state.session.accountId,
    fake.state.session.group,
    fake.state.role,
  ],
  useSystemAccess: Object.assign(
    (selector: (s: typeof fake.state) => unknown) => selector(fake.state),
    { getState: () => fake.state },
  ),
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => options,
  useMutation: (options: unknown) => options,
  useQueryClient: () => ({ invalidateQueries: fake.invalidate }),
}));
vi.mock("../src/lib/api/http", () => ({
  apiGet: (...args: unknown[]) => fake.get(...args),
  apiPost: (...args: unknown[]) => fake.post(...args),
}));

import {
  useExports,
  useCreateExport,
  downloadExport,
} from "../src/lib/api/exports";
beforeEach(() => {
  fake.state = {
    role: 1,
    session: { accountId: 7, group: "default", epoch: 1 },
  };
  fake.get.mockReset();
  fake.post.mockReset();
  fake.invalidate.mockReset().mockResolvedValue(undefined);
});
describe("owner-bound export jobs", () => {
  it("keys export history by the confirmed session and polls processing jobs", () => {
    const query = useExports() as any;
    expect(query.queryKey).toEqual(["exports", [1, 7, "default", 1]]);
    expect(
      query.refetchInterval({ state: { data: [{ status: "processing" }] } }),
    ).toBe(1000);
    expect(
      query.refetchInterval({ state: { data: [{ status: "completed" }] } }),
    ).toBe(false);
  });
  it("rejects stale exports after switching accounts", async () => {
    const query = useExports() as any;
    let finish!: (v: unknown) => void;
    fake.get.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const read = query.queryFn({ signal: undefined });
    fake.state.session.accountId = 8;
    fake.state.session.epoch = 2;
    finish([{ user_id: 7 }]);
    await expect(read).rejects.toThrow("account changed");
  });
  it("rejects foreign history and mutation responses", async () => {
    fake.get.mockResolvedValue([{ user_id: 8 }]);
    await expect(
      (useExports() as any).queryFn({ signal: undefined }),
    ).rejects.toThrow("another account");
    fake.post.mockResolvedValue({ user_id: 8 });
    await expect(
      (useCreateExport() as any).mutationFn({ type: "ledger", range_days: 31 }),
    ).rejects.toThrow("another account");
  });
  it("requires an opaque URL without a numeric download fallback", async () => {
    await expect(downloadExport(1, "x.csv")).rejects.toThrow("Invalid export");
    await expect(
      downloadExport(1, "x.csv", "/api/export/1/download"),
    ).rejects.toThrow("Invalid export");
  });
  it("does not download arbitrary locations or use a signed-out identity", async () => {
    await expect(
      downloadExport(1, "x.csv", "https://outside.example/x"),
    ).rejects.toThrow("Invalid export");
    fake.state.session.accountId = null;
    await expect(downloadExport(1, "x.csv")).rejects.toThrow("account changed");
  });
});
