import { describe, expect, it } from "vitest";
import { billingNano } from "./billing-money";
import { adaptLedgerEntry, normalizeWallet, type LedgerEntry } from "./billing";

describe("exact wallet and ledger API amounts", () => {
  it("prefers exact strings after JSON numeric fields have rounded", () => {
    const payload = JSON.parse(
      '{"balance":9223372036854775807,"balance_nano":"9223372036854775807"}',
    );
    expect(normalizeWallet(payload).balance_nano).toBe("9223372036854775807");
    expect(BigInt(payload.balance)).not.toBe(9223372036854775807n);
  });
  it("keeps signed-minimum ledger debits and balance-after exact", () => {
    const row = {
      id: 1,
      user_id: 1,
      type: "adjustment",
      amount: Number(-9223372036854775808n),
      amount_nano: "-9223372036854775808",
      balance_after: Number(9223372036854775807n),
      balance_after_nano: "9223372036854775807",
      description: "",
      order_id: "",
      trace_id: "",
      created_at: 1793145600,
    } as LedgerEntry;
    expect(adaptLedgerEntry(row)).toMatchObject({
      amount_nano: "-9223372036854775808",
      balance_after_nano: "9223372036854775807",
    });
  });
  it.each([0, 1, -1, Number.MAX_SAFE_INTEGER, Number.MIN_SAFE_INTEGER])(
    "accepts safe legacy integer %s",
    (value) => {
      expect(billingNano(undefined, value)).toBe(String(value));
    },
  );
  it.each([
    Number.MAX_SAFE_INTEGER + 1,
    Number.MIN_SAFE_INTEGER - 1,
    1.5,
    NaN,
    Infinity,
    null,
    "7",
  ])("rejects already imprecise or malformed legacy amount %s", (value) => {
    expect(() => billingNano(undefined, value)).toThrow();
  });
  it.each([
    null,
    1,
    "1.0",
    "+1",
    "01",
    "1e3",
    "9223372036854775808",
    "-9223372036854775809",
    "",
  ])("never falls back after a malformed exact field %s", (value) => {
    expect(() => billingNano(value, 7)).toThrow();
  });
  it("keeps positive ledger signing compatible with exact integer consumers", () => {
    const row = {
      id: 1,
      user_id: 1,
      type: "adjustment",
      amount: 7,
      balance_after: 7,
      description: "",
      order_id: "",
      trace_id: "",
      created_at: 1793145600,
    } as LedgerEntry;
    expect(adaptLedgerEntry(row).amount_nano).toBe("+7");
    expect(billingNano("0", 1)).toBe("0");
  });
});
