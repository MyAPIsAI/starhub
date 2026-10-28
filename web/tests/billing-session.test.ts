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
  useWallet,
  useLedger,
  useRechargeOrders,
  useRechargeOrder,
  useCreateRechargeOrder,
  useBeginRechargePayment,
  useResolveRechargeOrder,
} from "../src/lib/api/billing";
const queries = () =>
  [useWallet(), useLedger(), useRechargeOrders(), useRechargeOrder(9)] as any[];
const mutations = () =>
  [
    useCreateRechargeOrder(),
    useBeginRechargePayment(),
    useResolveRechargeOrder(),
  ] as any[];
const args = [{ amount_usd: "10.00" }, 9, { id: 9, outcome: "success" }];
beforeEach(() => {
  fake.state = {
    role: 1,
    session: { accountId: 7, group: "default", epoch: 1 },
  };
  fake.get.mockReset();
  fake.post.mockReset();
  fake.invalidate.mockReset().mockResolvedValue(undefined);
});

describe("real-session billing requests", () => {
  it("keys all four reads by server epoch, account, group and exact role", () => {
    const before = queries();
    for (const q of before) {
      expect(q.queryKey[1]).toEqual([1, 7, "default", 1]);
      expect(q.enabled).toBe(true);
      expect(q.retry).toBe(false);
    }
    for (const change of [
      { session: { accountId: 8, group: "default", epoch: 1 } },
      { session: { accountId: 7, group: "vip", epoch: 1 } },
      { session: { accountId: 7, group: "default", epoch: 2 } },
      { role: 5 },
    ]) {
      const original = fake.state;
      fake.state = { ...fake.state, ...change };
      for (const [i, q] of queries().entries())
        expect(q.queryKey).not.toEqual(before[i].queryKey);
      fake.state = original;
    }
  });
  it("blocks guest reads even when a caller manually refetches a disabled query", async () => {
    fake.state.session.accountId = null;
    for (const q of queries()) {
      expect(q.enabled).toBe(false);
      await expect(q.queryFn({})).rejects.toThrow("confirmed unchanged");
    }
    expect(fake.get).not.toHaveBeenCalled();
    for (const [i, m] of mutations().entries()) {
      expect(m.retry).toBe(false);
      await expect(m.mutationFn(args[i])).rejects.toThrow(
        "confirmed unchanged",
      );
    }
    expect(fake.post).not.toHaveBeenCalled();
  });
  it("rejects captured reads and mutations after same-role identity changes before dispatch", async () => {
    const reads = queries(),
      writes = mutations();
    fake.state.session = { accountId: 8, group: "default", epoch: 2 };
    for (const q of reads)
      await expect(q.queryFn({})).rejects.toThrow("confirmed unchanged");
    for (const [i, m] of writes.entries())
      await expect(m.mutationFn(args[i])).rejects.toThrow(
        "confirmed unchanged",
      );
    expect(fake.get).not.toHaveBeenCalled();
    expect(fake.post).not.toHaveBeenCalled();
  });
  it("does not expose late old-session responses or continue old-order polling", async () => {
    for (const index of [0, 1, 2, 3]) {
      let resolve!: (value: unknown) => void;
      fake.get.mockImplementation(
        () =>
          new Promise((r) => {
            resolve = r;
          }),
      );
      const q = queries()[index],
        pending = q.queryFn({});
      fake.state.session = {
        ...fake.state.session,
        epoch: fake.state.session.epoch + 1,
      };
      resolve(
        index === 0
          ? { balance: 123 }
          : index === 1
            ? []
            : index === 2
              ? []
              : { status: "pending" },
      );
      await expect(pending).rejects.toThrow("session changed");
      if (index === 3)
        expect(
          q.refetchInterval({ state: { data: { status: "pending" } } }),
        ).toBe(false);
    }
  });
  it("marks a late write unconfirmed and invalidates authoritative reads on settlement", async () => {
    for (const [index, m] of mutations().entries()) {
      fake.state.session.epoch = 1;
      let resolve!: (value: unknown) => void;
      fake.post.mockImplementation(
        () =>
          new Promise((r) => {
            resolve = r;
          }),
      );
      const pending = m.mutationFn(args[index]);
      fake.state.session = {
        ...fake.state.session,
        epoch: fake.state.session.epoch + 1,
      };
      resolve({ id: 9, status: "paid" });
      await expect(pending).rejects.toThrow();
      m.onSettled();
    }
    expect(fake.invalidate).toHaveBeenCalledWith({ queryKey: ["wallet"] });
    expect(fake.invalidate).toHaveBeenCalledWith({ queryKey: ["ledger"] });
    expect(fake.invalidate).toHaveBeenCalledWith({ queryKey: ["recharge"] });
  });
  it("keeps exact wallet amounts and forwards cancellation under the current scope", async () => {
    const controller = new AbortController();
    fake.get.mockResolvedValue({
      balance: 9007199254740992,
      balance_nano: "9007199254740993",
    });
    expect(
      (await (useWallet() as any).queryFn({ signal: controller.signal }))
        .balance_nano,
    ).toBe("9007199254740993");
    expect(fake.get).toHaveBeenCalledWith("/api/wallet", controller.signal);
    const order = useRechargeOrder(9) as any;
    expect(
      order.refetchInterval({ state: { data: { status: "pending" } } }),
    ).toBe(5000);
    expect(order.refetchInterval({ state: { data: { status: "paid" } } })).toBe(
      false,
    );
  });
  it("refuses unsafe or fractional order identifiers before querying or writing", async () => {
    for (const id of [0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1, NaN]) {
      const query = useRechargeOrder(id) as any;
      expect(query.enabled).toBe(false);
      await expect(query.queryFn({})).rejects.toThrow("identifier");
      await expect(
        (useBeginRechargePayment() as any).mutationFn(id),
      ).rejects.toThrow("identifier");
      await expect(
        (useResolveRechargeOrder() as any).mutationFn({
          id,
          outcome: "success",
        }),
      ).rejects.toThrow("identifier");
    }
    expect(fake.get).not.toHaveBeenCalled();
    expect(fake.post).not.toHaveBeenCalled();
  });
});
