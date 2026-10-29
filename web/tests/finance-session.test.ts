import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({
  state: {
    role: 5,
    session: { accountId: 7 as number | null, group: "default", epoch: 1 },
  },
  get: vi.fn(),
  put: vi.fn(),
  invalidate: vi.fn(),
}));
vi.mock("../src/lib/product-preview", () => {
  throw Error("Finance requests must not import Demo state");
});
vi.mock("../src/lib/system-access", () => ({
  ROLE_USER: 1,
  ROLE_FINANCE: 5,
  ROLE_ROOT: 100,
  canReadFinance: (role: number) => [5, 10, 100].includes(role),
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
import { useFinanceSummary, useSetFinanceRole } from "../src/lib/api/finance";
const summary = {
  since: 100,
  until: 200,
  wallet_balance_nano: "18446744073709551614",
  paid_recharge_nano: "0",
  ledger_credits_nano: "0",
  ledger_debits_nano: "0",
  api_collected_nano: "0",
  api_unfunded_nano: "0",
  paid_recharge_count: 0,
  paid_orders_without_timestamp: 0,
  api_request_count: 0,
  api_success_count: 0,
  api_failed_count: 0,
  api_cancelled_count: 0,
  wallet_and_api_quota_linked: false,
};
const input = { id: 9, enabled: true, expected_role: 1 };
beforeEach(() => {
  fake.state = {
    role: 5,
    session: { accountId: 7, group: "default", epoch: 1 },
  };
  fake.get.mockReset();
  fake.put.mockReset();
  fake.invalidate.mockReset();
});
describe("confirmed finance session transport", () => {
  it("uses all server identity dimensions and validates the reporting period", async () => {
    const query = useFinanceSummary(30) as any;
    expect(query.queryKey).toEqual([
      "finance-summary",
      [1, 7, "default", 5],
      30,
    ]);
    expect(query.enabled).toBe(true);
    expect(query.retry).toBe(false);
    fake.get.mockResolvedValue(summary);
    expect((await query.queryFn({})).wallet_balance_nano).toBe(
      summary.wallet_balance_nano,
    );
    for (const days of [0, 91, 1.5, NaN, Infinity]) {
      const invalid = useFinanceSummary(days) as any;
      expect(invalid.enabled).toBe(false);
      await expect(invalid.queryFn({})).rejects.toThrow(/days/);
    }
    expect(fake.get).toHaveBeenCalledTimes(1);
    fake.state.session.accountId = 8;
    expect((useFinanceSummary(30) as any).queryKey).not.toEqual(query.queryKey);
    fake.state.session.group = "vip";
    expect((useFinanceSummary(30) as any).queryKey).toEqual([
      "finance-summary",
      [1, 8, "vip", 5],
      30,
    ]);
  });
  it("prevents guest, unknown-role and stale manual reads before dispatch", async () => {
    const old = useFinanceSummary(30) as any;
    fake.state.session.epoch++;
    await expect(old.queryFn({})).rejects.toThrow(/session/);
    for (const role of [0, 1, 11, 99, 101]) {
      fake.state.role = role;
      const query = useFinanceSummary(30) as any;
      expect(query.enabled).toBe(false);
      await expect(query.queryFn({})).rejects.toThrow(/session/);
    }
    fake.state.role = 100;
    fake.state.session.accountId = null;
    const guest = useFinanceSummary(30) as any;
    expect(guest.enabled).toBe(false);
    await expect(guest.queryFn({})).rejects.toThrow(/session/);
    expect(fake.get).not.toHaveBeenCalled();
  });
  it("discards a delayed summary after a same-role account switch", async () => {
    const query = useFinanceSummary(30) as any;
    let release!: (value: unknown) => void;
    fake.get.mockImplementation(
      () => new Promise((resolve) => (release = resolve)),
    );
    const result = query.queryFn({});
    fake.state.session.accountId = 8;
    release(summary);
    await expect(result).rejects.toThrow(/session/);
  });
  it("allows only confirmed Root to dispatch an exact guarded role change", async () => {
    for (const role of [1, 5, 10, 99, 101]) {
      fake.state.role = role;
      await expect(
        (useSetFinanceRole() as any).mutationFn(input),
      ).rejects.toThrow(/session/);
    }
    fake.state.role = 100;
    fake.state.session.accountId = null;
    await expect(
      (useSetFinanceRole() as any).mutationFn(input),
    ).rejects.toThrow(/session/);
    expect(fake.put).not.toHaveBeenCalled();
    fake.state.session.accountId = 7;
    const mutation = useSetFinanceRole() as any;
    expect(mutation.retry).toBe(false);
    for (const change of [
      { ...input, id: 0 },
      { ...input, id: 1.5 },
      { ...input, id: Number.MAX_SAFE_INTEGER + 1 },
      { ...input, expected_role: 10 },
      { ...input, enabled: "true" },
    ])
      await expect(mutation.mutationFn(change)).rejects.toThrow(/Invalid/);
    expect(fake.put).not.toHaveBeenCalled();
    fake.put.mockResolvedValue(undefined);
    await mutation.mutationFn(input);
    expect(fake.put).toHaveBeenCalledWith("/api/user/9/finance-role", {
      enabled: true,
      expected_role: 1,
    });
    mutation.onSettled();
    for (const key of ["admin-users", "admin-audit", "finance-summary"])
      expect(fake.invalidate).toHaveBeenCalledWith({ queryKey: [key] });
  });
  it("rejects stale Root writes, late results and unsupported acknowledgements", async () => {
    fake.state.role = 100;
    const old = useSetFinanceRole() as any;
    fake.state.session.epoch++;
    await expect(old.mutationFn(input)).rejects.toThrow(/session/);
    expect(fake.put).not.toHaveBeenCalled();
    const current = useSetFinanceRole() as any;
    let release!: (value: unknown) => void;
    fake.put.mockImplementation(
      () => new Promise((resolve) => (release = resolve)),
    );
    const result = current.mutationFn(input);
    fake.state.role = 10;
    release(null);
    await expect(result).rejects.toThrow(/session/);
    fake.state.role = 100;
    fake.put.mockResolvedValue({ enabled: true });
    await expect(
      (useSetFinanceRole() as any).mutationFn(input),
    ).rejects.toThrow(/acknowledgement/);
  });
});
