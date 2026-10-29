import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({
  state: {
    role: 1,
    session: { accountId: 7 as number | null, group: "default", epoch: 1 },
  },
  get: vi.fn(),
}));
vi.mock("../src/lib/product-preview", () => {
  throw Error("Token reads must not depend on Demo identity");
});
vi.mock("../src/lib/system-access", () => ({
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
  useQueryClient: () => ({}),
}));
vi.mock("../src/lib/api/http", () => ({
  apiGet: (...args: unknown[]) => fake.get(...args),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
  apiDelete: vi.fn(),
}));
import { useTokens } from "../src/lib/api/tokens";
beforeEach(() => {
  fake.state = {
    role: 1,
    session: { accountId: 7, group: "default", epoch: 1 },
  };
  fake.get.mockReset();
});

describe("confirmed API key list transport", () => {
  it("uses every confirmed session dimension and forwards cancellation", async () => {
    const query = useTokens() as any,
      signal = new AbortController().signal;
    expect(query.queryKey).toEqual(["tokens", [1, 7, "default", 1]]);
    expect(query.enabled).toBe(true);
    expect(query.retry).toBe(false);
    expect(query.staleTime).toBe(0);
    const tokens = [{ id: 2, user_id: 7 }];
    fake.get.mockResolvedValue(tokens);
    expect(await query.queryFn({ signal })).toBe(tokens);
    expect(fake.get).toHaveBeenCalledWith("/api/token", signal);
  });
  it("blocks guest and stale manual reads before dispatch", async () => {
    const old = useTokens() as any;
    fake.state.session.epoch++;
    await expect(old.queryFn({})).rejects.toThrow(/session/);
    fake.state.session.accountId = null;
    const guest = useTokens() as any;
    expect(guest.enabled).toBe(false);
    await expect(guest.queryFn({})).rejects.toThrow(/session/);
    expect(fake.get).not.toHaveBeenCalled();
  });
  it.each(["accountId", "epoch", "group", "role"])(
    "discards a delayed result after a change to %s",
    async (field) => {
      const query = useTokens() as any;
      let release!: (value: unknown) => void;
      fake.get.mockImplementation(
        () => new Promise((resolve) => (release = resolve)),
      );
      const pending = query.queryFn({});
      if (field === "accountId") fake.state.session.accountId = 8;
      else if (field === "epoch") fake.state.session.epoch++;
      else if (field === "group") fake.state.session.group = "vip";
      else fake.state.role = 10;
      release([{ id: 2, user_id: 7 }]);
      await expect(pending).rejects.toThrow(/session/);
    },
  );
  it("rejects missing and foreign ownership without retaining a partial list", async () => {
    for (const value of [
      null,
      {},
      [{ id: 2 }],
      [
        { id: 2, user_id: 7 },
        { id: 3, user_id: 8 },
      ],
    ]) {
      fake.get.mockResolvedValue(value);
      await expect((useTokens() as any).queryFn({})).rejects.toThrow(
        /ownership/,
      );
    }
  });
});
