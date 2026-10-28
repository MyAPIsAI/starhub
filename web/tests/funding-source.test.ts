import { describe, expect, it } from "vitest";
import { fundingSource, fundingSourceLabel } from "../src/lib/funding-source";
import {
  adaptLedgerEntry,
  adaptRechargeOrder,
  type LedgerEntry,
  type RechargeOrder,
} from "../src/lib/api/billing";
import {
  serializeLedgerCsv,
  serializeRechargeCsv,
} from "../src/lib/billing-export";
const ledger = (source: unknown, eligible = true) =>
  ({
    id: 1,
    user_id: 7,
    type: "recharge",
    amount: 9007199254740992,
    amount_nano: "9007199254740993",
    balance_after: 9007199254740992,
    balance_after_nano: "9007199254740993",
    description: "Stored credit",
    order_id: "1",
    trace_id: "",
    created_at: 1793232000,
    funding_source: source,
    api_quota_eligible: eligible,
  }) as LedgerEntry;
const order = (source: unknown, eligible = true) =>
  ({
    id: 1,
    user_id: 7,
    order_number: "ord_1",
    amount: 10_000_000_000,
    amount_nano: 10_000_000_000,
    amount_usd: "10.00",
    status: "paid",
    channel: "card",
    failure_reason: "",
    late_payment: false,
    created_at: "2026-10-29T00:00:00Z",
    expires_at: "2026-10-29T00:30:00Z",
    paid_at: "2026-10-29T00:01:00Z",
    funding_source: source,
    api_quota_eligible: eligible,
  }) as RechargeOrder;
describe("funding source presentation and exports", () => {
  it("recognizes only declared simulation and never certifies missing or invented provenance", () => {
    expect(fundingSource("simulation")).toBe("simulation");
    for (const value of [
      undefined,
      null,
      "",
      "legacy_unverified",
      "verified_provider",
      "trusted",
      "SIMULATION",
      { trusted: true },
      true,
    ]) {
      expect(fundingSource(value)).toBe("legacy_unverified");
      expect(adaptLedgerEntry(ledger(value)).funding_source).toBe(
        "legacy_unverified",
      );
      expect(adaptRechargeOrder(order(value)).funding_source).toBe(
        "legacy_unverified",
      );
    }
  });
  it("preserves exact amounts while current adapters never expose quota conversion", () => {
    for (const source of [
      "simulation",
      "legacy_unverified",
      "verified_provider",
    ]) {
      const entry = adaptLedgerEntry(ledger(source)),
        paid = adaptRechargeOrder(order(source));
      expect(entry.api_quota_eligible).toBe(false);
      expect(paid.api_quota_eligible).toBe(false);
      expect(entry.amount_nano).toBe("+9007199254740993");
      expect(entry.balance_after_nano).toBe("9007199254740993");
      expect(paid.status).toBe("paid");
      expect(paid.amount_nano).toBe("10000000000");
    }
  });
  it("localizes honest source labels", () => {
    const zh = (zh: string) => zh,
      en = (_: string, en: string) => en,
      ar = (_: string, _en: string, ar: string) => ar;
    expect(fundingSourceLabel("simulation", zh)).toBe("支付模拟");
    expect(fundingSourceLabel("simulation", en)).toBe("Payment simulation");
    expect(fundingSourceLabel(undefined, ar)).toBe("مصدر تاريخي غير متحقق منه");
    expect(fundingSourceLabel("verified_provider", en)).toBe(
      "Legacy source unverified",
    );
  });
  it("appends canonical provenance and false conversion to localized CSV without changing money", () => {
    const entry = adaptLedgerEntry(ledger("simulation"));
    for (const language of ["zh", "en", "ar"] as const) {
      const csv = serializeLedgerCsv([entry], language);
      expect(csv).toContain('"9007199.254740993","9007199254740993"');
      expect(csv).toContain(',"simulation","false"\r\n');
      const legacy = serializeRechargeCsv(
        [adaptRechargeOrder(order(undefined))],
        language,
      );
      expect(legacy).toContain(',"legacy_unverified","false"\r\n');
      expect(legacy).toContain('"10.000000000","10000000000"');
    }
  });
});
