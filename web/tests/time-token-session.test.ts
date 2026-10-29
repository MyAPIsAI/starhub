import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({
  state: {
    role: 10,
    session: { accountId: 7 as number | null, group: "default", epoch: 1 },
  },
  get: vi.fn(),
  put: vi.fn(),
  invalidate: vi.fn(),
}));
vi.mock("../src/lib/product-preview", () => {
  throw Error("Independent server prices must not import Demo state");
});
vi.mock("../src/lib/system-access", () => ({
  isAdmin: (role: number) => role === 10 || role === 100,
  sessionQueryIdentity: (state: typeof fake.state) => [
    state.session.epoch,
    state.session.accountId,
    state.session.group,
    state.role,
  ],
  useSessionQueryIdentity: () => [
    fake.state.session.epoch,
    fake.state.session.accountId,
    fake.state.session.group,
    fake.state.role,
  ],
  useSystemAccess: { getState: () => fake.state },
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => options,
  useMutation: (options: unknown) => options,
  useQueryClient: () => ({ invalidateQueries: fake.invalidate }),
}));
vi.mock("../src/lib/api/http", () => ({
  apiGet: (...args: unknown[]) => fake.get(...args),
  apiPut: (...args: unknown[]) => fake.put(...args),
}));
import {
  useServerTokenTimePricing,
  useSaveServerTokenTimePricing,
} from "../src/lib/api/time-token-pricing";
const ack = (version = 3) => ({
  version,
  loaded_version: version,
  revision_guards: true,
  schema: "weekly_token_prices_v1",
  billing_active: true,
  token_only: true,
  asynchronous_tasks: false,
  multiplier_behavior: "stacked",
});
beforeEach(() => {
  fake.state = {
    role: 10,
    session: { accountId: 7, group: "default", epoch: 1 },
  };
  fake.get.mockReset();
  fake.put.mockReset();
  fake.invalidate.mockReset();
});
describe("independent time pricing confirmed-session transport", () => {
  it("isolates the complete server session and guards manual private reads", async () => {
    const query = useServerTokenTimePricing(true) as any;
    expect(query.queryKey).toEqual([
      "time-token-pricing",
      [1, 7, "default", 10],
    ]);
    fake.get.mockResolvedValue({ ...ack(), rules: {} });
    expect((await query.queryFn({ signal: undefined })).rules).toEqual({});
    for (const role of [1, 5, 99, 101]) {
      fake.state.role = role;
      const denied = useServerTokenTimePricing(true) as any;
      expect(denied.enabled).toBe(false);
      await expect(denied.queryFn({})).rejects.toThrow(/session/);
    }
    fake.state.role = 100;
    fake.state.session.accountId = null;
    const guest = useServerTokenTimePricing(true) as any;
    expect(guest.enabled).toBe(false);
    await expect(guest.queryFn({})).rejects.toThrow(/session/);
    expect(fake.get).toHaveBeenCalledTimes(1);
  });
  it("rejects late reads and stale mutations after account, group or epoch changes", async () => {
    const query = useServerTokenTimePricing(true) as any,
      mutation = useSaveServerTokenTimePricing() as any;
    let release!: (value: unknown) => void;
    fake.get.mockImplementation(
      () => new Promise((resolve) => (release = resolve)),
    );
    const result = query.queryFn({});
    fake.state.session.group = "vip";
    release({ ...ack(), rules: {} });
    await expect(result).rejects.toThrow(/session/);
    await expect(
      mutation.mutationFn({ expected_version: 3, rules: {} }),
    ).rejects.toThrow(/session/);
    expect(fake.put).not.toHaveBeenCalled();
    const current = useSaveServerTokenTimePricing() as any;
    fake.put.mockImplementation(
      () => new Promise((resolve) => (release = resolve)),
    );
    const saving = current.mutationFn({ expected_version: 3, rules: {} });
    fake.state.session.epoch++;
    release(ack(4));
    await expect(saving).rejects.toThrow(/session/);
  });
  it("checks revisions and capabilities without retries and invalidates affected views", async () => {
    const mutation = useSaveServerTokenTimePricing() as any;
    expect(mutation.retry).toBe(false);
    for (const version of [-1, 1.5, Number.MAX_SAFE_INTEGER, NaN])
      await expect(
        mutation.mutationFn({ expected_version: version, rules: {} }),
      ).rejects.toThrow(/revision/);
    expect(fake.put).not.toHaveBeenCalled();
    for (const response of [
      { ...ack(4), billing_active: false },
      { ...ack(4), asynchronous_tasks: true },
      ack(6),
      ack(2),
    ]) {
      fake.put.mockResolvedValue(response);
      await expect(
        mutation.mutationFn({ expected_version: 3, rules: {} }),
      ).rejects.toThrow();
    }
    fake.put.mockResolvedValue(ack(4));
    expect(
      await mutation.mutationFn({ expected_version: 3, rules: {} }),
    ).toEqual({ version: 4, loaded_version: 4 });
    expect(fake.put).toHaveBeenLastCalledWith(
      "/api/option/time-token-pricing",
      { expected_version: 3, rules: {} },
    );
    mutation.onSettled();
    for (const key of [
      "time-token-pricing",
      "time-pricing",
      "server-pricing",
      "options",
      "option-history",
      "plaza-model",
      "resource-publication-context",
    ])
      expect(fake.invalidate).toHaveBeenCalledWith({ queryKey: [key] });
  });
});
