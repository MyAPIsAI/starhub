import { describe, expect, it } from "vitest";
import type { LedgerEntry, RechargeOrder } from "../src/lib/product-preview";
import {
  billingDate,
  validateBillingRange,
  filterLedger,
  filterRechargeOrders,
  serializeLedgerCsv,
  serializeRechargeCsv,
} from "../src/lib/billing-export";

const ledger = (overrides: Partial<LedgerEntry> = {}): LedgerEntry => ({
  id: "tx_1",
  time: "2026-09-01T09:00:00+04:00",
  type: "recharge",
  amount_nano: "+10000000000",
  balance_after_nano: "10000000000",
  description: "Card payment",
  ...overrides,
});

const order = (overrides: Partial<RechargeOrder> = {}): RechargeOrder => ({
  id: "ord_1",
  created_at: "2026-09-01T09:00:00+04:00",
  expires_at: "2026-09-01T09:30:00+04:00",
  status: "pending",
  amount_nano: "10000000000",
  amount_usd: "10.00",
  currency: "USD",
  channel: "card",
  provider_order_id: "provider_1",
  ...overrides,
});

describe("billing calendar ranges", () => {
  it("validates actual calendar dates, including century leap-year rules", () => {
    for (const date of ["2024-02-29", "2000-02-29", "2026-04-30"]) {
      expect(validateBillingRange(date, date)).toBe(true);
    }
    for (const date of [
      "2025-02-29",
      "1900-02-29",
      "2026-04-31",
      "2026-13-01",
      "2026-00-10",
      "2026-01-00",
      "2026-2-01",
      "",
      "2026-02-01T00:00:00Z",
    ]) {
      expect(validateBillingRange(date, date)).toBe(false);
    }
  });

  it("counts both endpoints and allows up to 31 days across months and years", () => {
    expect(validateBillingRange("2026-01-31", "2026-03-02")).toBe(true);
    expect(validateBillingRange("2026-01-31", "2026-03-03")).toBe(false);
    expect(validateBillingRange("2025-12-20", "2026-01-19")).toBe(true);
    expect(validateBillingRange("2025-12-20", "2026-01-20")).toBe(false);
    expect(validateBillingRange("2026-09-02", "2026-09-01")).toBe(false);
  });

  it("uses Dubai dates and rejects invalid or ambiguous local timestamps", () => {
    expect(billingDate("2026-08-31T19:59:59.999Z")).toBe("2026-08-31");
    expect(billingDate("2026-08-31T20:00:00Z")).toBe("2026-09-01");
    expect(billingDate("2026-09-01T00:30:00+05:00")).toBe("2026-08-31");
    expect(billingDate("2026-02-30T00:00:00Z")).toBe("");
    expect(billingDate("2026-09-01T25:00:00Z")).toBe("");
    expect(billingDate("2026-09-01T09:00:00")).toBe("");
    expect(billingDate("not-a-date")).toBe("");
  });
});

describe("billing filters", () => {
  it("includes a Dubai day from midnight through the last millisecond and sorts by instant", () => {
    const rows = [
      ledger({ id: "before", time: "2026-08-31T19:59:59.999Z" }),
      ledger({ id: "start", time: "2026-08-31T20:00:00Z" }),
      ledger({ id: "end", time: "2026-09-01T19:59:59.999Z" }),
      ledger({ id: "after", time: "2026-09-02T00:00:00+04:00" }),
      ledger({ id: "middle", time: "2026-09-01T12:00:00+08:00" }),
      ledger({ id: "invalid", time: "invalid" }),
    ];
    const originalOrder = rows.map((row) => row.id);
    expect(
      filterLedger(rows, { from: "2026-09-01", to: "2026-09-01" }).map(
        (row) => row.id,
      ),
    ).toEqual(["end", "middle", "start"]);
    expect(rows.map((row) => row.id)).toEqual(originalOrder);
    expect(
      filterLedger(rows, { from: "2026-02-30", to: "2026-03-01" }),
    ).toEqual([]);
  });

  it("combines ledger type with case-insensitive searches of related identifiers", () => {
    const rows = [
      ledger({ id: "tx_match", type: "refund", order_id: "ORD_target" }),
      ledger({ id: "tx_wrong_type", type: "recharge", order_id: "ORD_target" }),
      ledger({
        id: "tx_trace",
        type: "refund",
        trace_id: "TR_target",
        description: "Adjustment detail",
      }),
    ];
    const range = { from: "2026-09-01", to: "2026-09-01" };
    expect(
      filterLedger(rows, {
        ...range,
        type: "refund",
        query: " ord_TARGET ",
      }).map((row) => row.id),
    ).toEqual(["tx_match"]);
    expect(
      filterLedger(rows, { ...range, type: "all", query: "tr_TARGET" }).map(
        (row) => row.id,
      ),
    ).toEqual(["tx_trace"]);
    expect(
      filterLedger(rows, { ...range, query: "ADJUSTMENT DETAIL" }).map(
        (row) => row.id,
      ),
    ).toEqual(["tx_trace"]);
    expect(filterLedger(rows, { ...range, query: "unknown" })).toEqual([]);
  });

  it("filters orders by their creation date, status and provider/order identifiers", () => {
    const rows = [
      order({
        id: "start",
        created_at: "2026-08-31T20:00:00Z",
        status: "paid",
        paid_at: "2026-09-05T00:00:00Z",
      }),
      order({
        id: "end",
        created_at: "2026-09-01T19:59:59.999Z",
        status: "paid",
        provider_order_id: "PAY_target",
      }),
      order({
        id: "before",
        created_at: "2026-08-31T19:59:59.999Z",
        status: "paid",
      }),
      order({
        id: "after",
        created_at: "2026-09-01T20:00:00Z",
        status: "paid",
      }),
      order({ id: "pending", created_at: "2026-09-01T09:00:00Z" }),
    ];
    const range = { from: "2026-09-01", to: "2026-09-01", status: "paid" };
    expect(filterRechargeOrders(rows, range).map((row) => row.id)).toEqual([
      "end",
      "start",
    ]);
    expect(
      filterRechargeOrders(rows, { ...range, query: "pay_TARGET" }).map(
        (row) => row.id,
      ),
    ).toEqual(["end"]);
    expect(
      filterRechargeOrders(rows, { ...range, query: "START" }).map(
        (row) => row.id,
      ),
    ).toEqual(["start"]);
    expect(
      filterRechargeOrders(rows, { from: "2026-09-01", to: "2026-10-02" }),
    ).toEqual([]);
    expect(rows[0].id).toBe("start");
  });
});

describe("billing CSV exports", () => {
  it("preserves signed nanodollars and full nine-digit dollar precision beyond safe integers", () => {
    const output = serializeLedgerCsv(
      [
        ledger({
          time: "2026-08-31T20:01:02.345Z",
          type: "call_summary",
          amount_nano: "-296",
          balance_after_nano: "10000000000000000296",
          description: 'A "quoted", multiline\ndescription',
          order_id: "ord_9",
          trace_id: "tr_9",
        }),
      ],
      "en",
    );
    expect(output).toBe(
      '\uFEFF"Ledger ID","Time (UTC+04:00)","Type","Amount (USD)","Amount (nano-USD)","Balance after (USD)","Balance after (nano-USD)","Description","Order ID","Trace ID","Funding Source","API Quota Eligible"\r\n' +
        '"tx_1","2026-09-01T00:01:02.345+04:00","call_summary","-0.000000296","-296","10000000000.000000296","10000000000000000296","A ""quoted"", multiline\ndescription","ord_9","tr_9","legacy_unverified","false"\r\n',
    );
    expect(output).not.toContain("$");
  });

  it("neutralizes formulas in untrusted text without corrupting negative amount columns", () => {
    const output = serializeLedgerCsv(
      [
        ledger({
          id: '=HYPERLINK("https://example.invalid")',
          amount_nano: "-1000000000",
          balance_after_nano: "-1000000000",
          description: "  +SUM(A1:A2)",
          order_id: "@SUM(A1)",
          trace_id: "-2+3",
        }),
      ],
      "en",
    );
    expect(output).toContain('"\'=HYPERLINK(""https://example.invalid"")"');
    expect(output).toContain('"\'  +SUM(A1:A2)"');
    expect(output).toContain('"\'@SUM(A1)"');
    expect(output).toContain('"\'-2+3"');
    expect(output).toContain(
      '"-1.000000000","-1000000000","-1.000000000","-1000000000"',
    );
    for (const prefix of ["\t", "\r", "\n", "\u0000="]) {
      expect(
        serializeLedgerCsv([ledger({ description: `${prefix}SUM(A1)` })], "en"),
      ).toContain(`"'${prefix}SUM(A1)"`);
    }
  });

  it("exports recharge lifecycle fields, deriving amounts from nanodollars", () => {
    const output = serializeRechargeCsv(
      [
        order({
          status: "paid",
          amount_nano: "12345678123456789",
          amount_usd: "12345678.12",
          created_at: "2026-09-01T20:00:00Z",
          expires_at: "2026-09-01T20:30:00Z",
          paid_at: "2026-09-01T20:31:00Z",
          late_payment: true,
        }),
      ],
      "zh",
    );
    expect(output).toBe(
      '\uFEFF"订单编号","创建时间（UTC+04:00）","状态","金额（美元）","金额（纳美元）","到账时间（UTC+04:00）","过期时间（UTC+04:00）","晚到标记","资金来源","可兑换API配额"\r\n' +
        '"ord_1","2026-09-02T00:00:00.000+04:00","paid","12345678.123456789","12345678123456789","2026-09-02T00:31:00.000+04:00","2026-09-02T00:30:00.000+04:00","true","legacy_unverified","false"\r\n',
    );
    expect(
      serializeRechargeCsv([order({ late_payment: false })], "en"),
    ).toContain(
      ',"","2026-09-01T09:30:00.000+04:00","false","legacy_unverified","false"\r\n',
    );
    expect(serializeRechargeCsv([order()], "en")).toContain(
      ',"","2026-09-01T09:30:00.000+04:00","","legacy_unverified","false"\r\n',
    );
  });

  it("localizes only headers and produces a valid empty export", () => {
    const row = ledger({
      type: "adjustment",
      description: "Original description",
    });
    const outputs = ["zh", "en", "ar"].map((language) =>
      serializeLedgerCsv([row], language as "zh" | "en" | "ar"),
    );
    expect(new Set(outputs.map((output) => output.split("\r\n")[0])).size).toBe(
      3,
    );
    expect(new Set(outputs.map((output) => output.split("\r\n")[1])).size).toBe(
      1,
    );
    expect(outputs[2]).toContain('"adjustment"');
    expect(outputs[2]).toContain('"Original description"');
    expect(serializeLedgerCsv([], "zh").split("\r\n")).toHaveLength(2);
    expect(
      serializeRechargeCsv([], "ar").startsWith('\uFEFF"معرّف الطلب"'),
    ).toBe(true);
  });

  it("rejects malformed amounts instead of silently rounding or treating them as zero", () => {
    for (const invalid of ["", "1.2", "1e9", "=1+2", "NaN"]) {
      expect(() =>
        serializeLedgerCsv([ledger({ amount_nano: invalid })], "en"),
      ).toThrow("Invalid nano-dollar amount");
      expect(() =>
        serializeRechargeCsv([order({ amount_nano: invalid })], "en"),
      ).toThrow("Invalid nano-dollar amount");
    }
  });
});
