import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  BILLING_STORAGE_KEY,
  listenBillingStorage,
  parseBillingSnapshot,
  readBillingStorage,
  validateRechargeAmount,
  writeBillingStorage,
} from "../src/lib/billing";
import { useProductPreview } from "../src/lib/product-preview";
import { sumNano } from "../src/lib/money";
import { filterExportRequests } from "../src/lib/request-export";

function memoryStorage() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
}

function createOrder(amount = "50.00") {
  const result = useProductPreview.getState().createRechargeOrder(amount);
  if (!result.ok) throw new Error(result.error);
  return result.order;
}

function expectReconciled() {
  const state = useProductPreview.getState();
  expect(sumNano(state.ledger.map((entry) => entry.amount_nano))).toBe(
    state.balance,
  );
  expect(BigInt(state.opening)).toBe(
    BigInt(state.balance) +
      BigInt(sumNano(state.requests.map((row) => row.cost_nano))),
  );
  let running = 0n;
  for (const entry of [...state.ledger].reverse()) {
    running += BigInt(entry.amount_nano);
    expect(entry.balance_after_nano).toBe(running.toString());
  }
}

describe("recharge money boundaries", () => {
  it.each([
    ["", "required"],
    ["  ", "required"],
    ["NaN", "format"],
    ["Infinity", "format"],
    ["1e3", "format"],
    ["10.001", "format"],
    ["-10", "format"],
    ["10,000", "format"],
    ["10.00 USD", "format"],
    ["0", "min"],
    ["9.99", "min"],
    ["10000.01", "max"],
  ])("rejects %s without rounding or guessing", (input, error) => {
    expect(validateRechargeAmount(input)).toEqual({ ok: false, error });
  });

  it("accepts inclusive limits and converts cents exactly", () => {
    expect(validateRechargeAmount(" 0010.01 ")).toEqual({
      ok: true,
      amount_usd: "10.01",
      amount_nano: "10010000000",
    });
    expect(validateRechargeAmount("10")).toEqual({
      ok: true,
      amount_usd: "10.00",
      amount_nano: "10000000000",
    });
    expect(validateRechargeAmount("10000.00")).toEqual({
      ok: true,
      amount_usd: "10000.00",
      amount_nano: "10000000000000",
    });
  });
});

describe("local recharge lifecycle and ledger", () => {
  let storage: ReturnType<typeof memoryStorage>;
  beforeEach(() => {
    storage = memoryStorage();
    vi.stubGlobal("localStorage", storage);
    useProductPreview.setState(useProductPreview.getInitialState(), true);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("creates and starts payment without credit, then credits a provider order once", () => {
    const before = useProductPreview.getState();
    const order = createOrder("10.01");
    expect(order.status).toBe("pending");
    expect(Date.parse(order.expires_at) - Date.parse(order.created_at)).toBe(
      1800000,
    );
    expect(
      useProductPreview.getState().beginRechargePayment(order.id),
    ).toMatchObject({ ok: true, order: { status: "processing" } });
    expect(useProductPreview.getState().balance).toBe(before.balance);
    expect(useProductPreview.getState().ledger).toBe(before.ledger);
    expect(
      useProductPreview.getState().resolveRechargeOrder(order.id, "success"),
    ).toMatchObject({
      ok: true,
      order: { status: "paid", late_payment: false },
    });
    expect(
      useProductPreview.getState().resolveRechargeOrder(order.id, "success"),
    ).toMatchObject({ ok: true, alreadyProcessed: true });
    expect(
      useProductPreview.getState().resolveRechargeOrder(order.id, "failed"),
    ).toMatchObject({ ok: true, order: { status: "paid" } });
    const after = useProductPreview.getState();
    expect(BigInt(after.balance) - BigInt(before.balance)).toBe(10010000000n);
    expect(
      after.ledger.filter(
        (entry) =>
          entry.idempotency_key === `payment:${order.provider_order_id}`,
      ),
    ).toHaveLength(1);
    expectReconciled();
  });

  it("expires processing orders and credits late success once", () => {
    const before = useProductPreview.getState().balance;
    const order = createOrder();
    useProductPreview.getState().beginRechargePayment(order.id);
    const now = Date.parse(order.expires_at) + 1;
    useProductPreview.getState().expireRechargeOrders(now);
    expect(useProductPreview.getState().rechargeOrders[0].status).toBe(
      "expired",
    );
    expect(useProductPreview.getState().balance).toBe(before);
    expect(useProductPreview.getState().beginRechargePayment(order.id)).toEqual(
      { ok: false, error: "order_not_payable" },
    );
    expect(
      useProductPreview
        .getState()
        .resolveRechargeOrder(order.id, "success", { now }),
    ).toMatchObject({
      ok: true,
      order: { status: "paid", late_payment: true },
    });
    useProductPreview
      .getState()
      .resolveRechargeOrder(order.id, "success", { now });
    expect(BigInt(useProductPreview.getState().balance) - BigInt(before)).toBe(
      50000000000n,
    );
    expectReconciled();
  });

  it("does not charge failures or cancellations; cancelled orders accept confirmed late success", () => {
    const before = useProductPreview.getState().balance;
    const failed = createOrder();
    useProductPreview.getState().resolveRechargeOrder(failed.id, "failed");
    expect(
      useProductPreview.getState().resolveRechargeOrder(failed.id, "success"),
    ).toEqual({ ok: false, error: "new_order_required" });
    const cancelled = createOrder();
    useProductPreview
      .getState()
      .resolveRechargeOrder(cancelled.id, "cancelled");
    expect(useProductPreview.getState().balance).toBe(before);
    expect(
      useProductPreview
        .getState()
        .resolveRechargeOrder(cancelled.id, "success"),
    ).toMatchObject({
      ok: true,
      order: { status: "paid", late_payment: true },
    });
    const retry = createOrder();
    expect(retry.provider_order_id).not.toBe(failed.provider_order_id);
    expectReconciled();
  });

  it("guards the legacy entry point and retains exact reconciliation through demo changes and calls", () => {
    expect(() => useProductPreview.getState().rechargeAccount(9.99)).toThrow(
      "min",
    );
    expect(() =>
      useProductPreview.getState().rechargeAccount(Infinity),
    ).toThrow("format");
    useProductPreview.getState().rechargeAccount(10.01);
    useProductPreview.getState().recordCall("pk_1", false);
    const before = useProductPreview.getState();
    useProductPreview.getState().recordCall("pk_1", true);
    expect(useProductPreview.getState().ledger).toBe(before.ledger);
    for (const mode of ["zero", "low", "normal"] as const) {
      useProductPreview.getState().demoSetBalanceState(mode);
      expectReconciled();
    }
    const ids = useProductPreview
      .getState()
      .ledger.map((entry) => entry.idempotency_key)
      .filter(Boolean);
    expect(new Set(ids).size).toBe(ids.length);
    useProductPreview.getState().demoSetAccountState("new");
    expectReconciled();
    useProductPreview.getState().demoSetAccountState("mature");
    expectReconciled();
  });

  it("restores orders, balances and local call metadata after a reload, without secrets", () => {
    const order = createOrder("73.21");
    useProductPreview.getState().beginRechargePayment(order.id);
    useProductPreview.getState().recordCall("pk_1", false);
    const raw = storage.getItem(BILLING_STORAGE_KEY)!;
    const before = useProductPreview.getState();
    expect(raw).not.toContain("keySecret");
    expect(raw).not.toContain("sk-xy-live");
    useProductPreview.setState(useProductPreview.getInitialState(), true);
    storage.setItem(BILLING_STORAGE_KEY, raw);
    expect(useProductPreview.getState().syncBillingStorage()).toBe(true);
    expect(useProductPreview.getState().balance).toBe(before.balance);
    expect(useProductPreview.getState().rechargeOrders[0]).toEqual(
      before.rechargeOrders[0],
    );
    expect(useProductPreview.getState().requests).toHaveLength(
      before.requests.length,
    );
    useProductPreview.getState().resolveRechargeOrder(order.id, "success");
    const paid = storage.getItem(BILLING_STORAGE_KEY)!;
    useProductPreview.setState(useProductPreview.getInitialState(), true);
    storage.setItem(BILLING_STORAGE_KEY, paid);
    expect(useProductPreview.getState().syncBillingStorage()).toBe(true);
    expect(
      useProductPreview.getState().resolveRechargeOrder(order.id, "success"),
    ).toMatchObject({ alreadyProcessed: true });
    expectReconciled();
  });

  it("rejects corrupt snapshots atomically and tolerates unavailable storage", () => {
    createOrder();
    const raw = storage.getItem(BILLING_STORAGE_KEY)!;
    expect(parseBillingSnapshot(raw)).not.toBeNull();
    const broken = JSON.parse(raw);
    broken.balance = "999999999999";
    expect(parseBillingSnapshot(JSON.stringify(broken))).toBeNull();
    broken.balance = useProductPreview.getState().balance;
    broken.rechargeOrders[0].amount_usd = "NaN";
    expect(parseBillingSnapshot(JSON.stringify(broken))).toBeNull();
    storage.setItem(BILLING_STORAGE_KEY, "{invalid json");
    const before = useProductPreview.getState();
    expect(useProductPreview.getState().syncBillingStorage()).toBe(false);
    expect(useProductPreview.getState()).toBe(before);
    const unavailable = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("quota");
      },
    };
    expect(readBillingStorage(unavailable)).toBeNull();
    expect(writeBillingStorage(parseBillingSnapshot(raw)!, unavailable)).toBe(
      false,
    );
    vi.stubGlobal("localStorage", undefined);
    expect(readBillingStorage()).toBeNull();
    expect(writeBillingStorage(parseBillingSnapshot(raw)!)).toBe(false);
  });

  it("expires restored pending orders without crediting them", () => {
    const now = Date.now();
    vi.spyOn(Date, "now").mockReturnValue(now - 1800001);
    const order = createOrder();
    vi.restoreAllMocks();
    expect(useProductPreview.getState().syncBillingStorage()).toBe(true);
    useProductPreview.getState().expireRechargeOrders();
    expect(
      useProductPreview
        .getState()
        .rechargeOrders.find((item) => item.id === order.id)?.status,
    ).toBe("expired");
    expectReconciled();
  });

  it("does not publish or persist unchanged expiry checks", () => {
    const order = createOrder();
    const save = vi.spyOn(storage, "setItem");
    const before = useProductPreview.getState();
    before.expireRechargeOrders(Date.parse(order.expires_at) - 1);
    expect(useProductPreview.getState()).toBe(before);
    expect(save).not.toHaveBeenCalled();
    before.expireRechargeOrders(Date.parse(order.expires_at));
    expect(save).toHaveBeenCalledTimes(1);
    const expired = useProductPreview.getState();
    expired.expireRechargeOrders(Date.parse(order.expires_at) + 1000);
    expect(useProductPreview.getState()).toBe(expired);
    expect(save).toHaveBeenCalledTimes(1);
  });

  it("receives valid snapshots from another tab without echoing storage writes", () => {
    const target = new EventTarget();
    const stop = listenBillingStorage(
      () => useProductPreview.getState().syncBillingStorage(),
      target,
    );
    const stale = useProductPreview.getState();
    const order = createOrder("82.17");
    useProductPreview.getState().resolveRechargeOrder(order.id, "success");
    const external = storage.getItem(BILLING_STORAGE_KEY)!;
    const expectedBalance = useProductPreview.getState().balance;
    useProductPreview.setState(stale, true);
    storage.setItem(BILLING_STORAGE_KEY, external);
    const save = vi.spyOn(storage, "setItem");
    target.dispatchEvent(
      Object.assign(new Event("storage"), { key: "unrelated" }),
    );
    expect(useProductPreview.getState().balance).toBe(stale.balance);
    target.dispatchEvent(
      Object.assign(new Event("storage"), { key: BILLING_STORAGE_KEY }),
    );
    expect(useProductPreview.getState().balance).toBe(expectedBalance);
    expect(useProductPreview.getState().rechargeOrders[0].status).toBe("paid");
    expect(save).not.toHaveBeenCalled();
    const received = useProductPreview.getState();
    target.dispatchEvent(
      Object.assign(new Event("storage"), { key: BILLING_STORAGE_KEY }),
    );
    expect(useProductPreview.getState()).toBe(received);
    expect(save).not.toHaveBeenCalled();
    expectReconciled();
    stop();
    useProductPreview.setState(stale, true);
    storage.setItem(BILLING_STORAGE_KEY, external);
    target.dispatchEvent(
      Object.assign(new Event("storage"), { key: BILLING_STORAGE_KEY }),
    );
    expect(useProductPreview.getState().balance).toBe(stale.balance);
  });

  it("refreshes an old tab before expiry so it cannot overwrite another tab's deposit", () => {
    const expiring = createOrder();
    const stale = useProductPreview.getState();
    const deposit = createOrder("123.45");
    useProductPreview.getState().resolveRechargeOrder(deposit.id, "success");
    const external = storage.getItem(BILLING_STORAGE_KEY)!;
    const expectedBalance = useProductPreview.getState().balance;
    useProductPreview.setState(stale, true);
    storage.setItem(BILLING_STORAGE_KEY, external);
    const save = vi.spyOn(storage, "setItem");
    useProductPreview
      .getState()
      .expireRechargeOrders(Date.parse(expiring.expires_at) + 1);
    expect(useProductPreview.getState().balance).toBe(expectedBalance);
    expect(
      useProductPreview
        .getState()
        .rechargeOrders.find((item) => item.id === expiring.id)?.status,
    ).toBe("expired");
    expect(
      useProductPreview
        .getState()
        .rechargeOrders.find((item) => item.id === deposit.id)?.status,
    ).toBe("paid");
    expect(readBillingStorage()?.balance).toBe(expectedBalance);
    expect(save).toHaveBeenCalledTimes(1);
    expectReconciled();
  });

  it("refreshes an old payment callback before checking whether the deposit was already posted", () => {
    const order = createOrder();
    const stale = useProductPreview.getState();
    useProductPreview.getState().resolveRechargeOrder(order.id, "success");
    const external = storage.getItem(BILLING_STORAGE_KEY)!;
    const expectedBalance = useProductPreview.getState().balance;
    useProductPreview.setState(stale, true);
    storage.setItem(BILLING_STORAGE_KEY, external);
    expect(
      useProductPreview.getState().resolveRechargeOrder(order.id, "success"),
    ).toMatchObject({ ok: true, alreadyProcessed: true });
    expect(useProductPreview.getState().balance).toBe(expectedBalance);
    expect(
      useProductPreview
        .getState()
        .ledger.filter((entry) => entry.order_id === order.id),
    ).toHaveLength(1);
    expectReconciled();
  });

  it("exports at most 31 inclusive Dubai dates and counts only matching ledger rows", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-30T20:30:00Z")); // October 1 in Dubai.
    const times = [
      "2026-08-31T19:59:59.999Z",
      "2026-08-31T20:00:00Z",
      "2026-10-01T19:59:59.999Z",
      "2026-10-01T20:00:00Z",
    ];
    useProductPreview.setState({
      ledger: useProductPreview
        .getState()
        .ledger.map((entry, index) => ({ ...entry, time: times[index] })),
    });
    const full = useProductPreview.getState().requestExport("ledger", 31);
    expect(full).toMatchObject({
      startDate: "2026-09-01",
      endDate: "2026-10-01",
      rangeDays: 31,
      rowCount: 2,
      filename: "ledger_2026-10-01.csv",
    });
    const today = useProductPreview.getState().requestExport("ledger", 1);
    expect(today).toMatchObject({
      startDate: "2026-10-01",
      endDate: "2026-10-01",
      rowCount: 1,
    });
    expect(Date.parse(today.expiresAt) - Date.parse(today.createdAt)).toBe(
      7 * 86400000,
    );
    for (const days of [0, -1, 32, 1.5, NaN, Infinity]) {
      expect(() =>
        useProductPreview.getState().requestExport("ledger", days),
      ).toThrow(RangeError);
    }
    const calls = useProductPreview.getState().requestExport("requests", 7);
    expect(calls).toMatchObject({
      startDate: "2026-09-24",
      endDate: "2026-09-30",
      rangeDays: 7,
      rowCount: filterExportRequests(useProductPreview.getState().requests, calls).length,
    });
  });
});
