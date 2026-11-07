import { describe, it, expect } from "vitest";
import {
  differenceLabel,
  reconciliationStatus,
  reconciliationSummary,
  type ReconciliationReport,
} from "../src/features/admin-reconciliation";
import { formatMoney } from "../src/lib/money";
const zh = (a: string) => a;
const report = (
  fields: Partial<ReconciliationReport>,
): ReconciliationReport => ({
  id: 1,
  date: "2026-11-07",
  user_id: 1,
  username: "fixture",
  timezone: "Asia/Dubai",
  usage_nano: "0",
  api_ledger_nano: "0",
  order_nano: "0",
  recharge_ledger_nano: "0",
  balance_expected_nano: "0",
  balance_actual_nano: "0",
  reserved_nano: "0",
  differences: [],
  provisional: false,
  created_at: 100,
  external_status: "not_imported",
  imported_sources: [],
  checks_version: 2,
  ...fields,
});
describe("Financial reconciliation presentation", () => {
  it("counts distinct affected dates and accounts without treating missing end-day reports as matched", () => {
    const difference = {
      kind: "balance_vs_ledger",
      funding_source: "api_quota",
      expected_nano: "0",
      actual_nano: "1",
      delta_nano: "1",
    };
    const result = reconciliationSummary(
      [
        report({ differences: [difference], user_id: 1 }),
        report({ differences: [difference], user_id: 2 }),
        report({ differences: [difference], date: "2026-11-06", user_id: 1 }),
        report({ provisional: true }),
        report({ checks_version: 0 }),
      ],
      "2026-11-08",
    );
    expect(result).toMatchObject({
      differenceDays: 2,
      affectedAccounts: 2,
      provisionalReports: 1,
      reviewReports: 1,
      endState: "empty",
      checkedAt: 0,
    });
  });
  it("keeps unverified and provisional end-day reports out of completed results", () => {
    expect(
      reconciliationSummary(
        [report({ admission_time_verified: false })],
        "2026-11-07",
      ).endState,
    ).toBe("review");
    expect(
      reconciliationSummary([report({ provisional: true })], "2026-11-07")
        .endState,
    ).toBe("provisional");
    expect(reconciliationSummary([report({})], "2026-11-07").endState).toBe(
      "matched",
    );
    expect(
      reconciliationStatus(report({ admission_time_verified: false }), zh),
    ).toBe("待重新核验");
  });
  it("keeps actual differences visible during the current day", () => {
    expect(
      reconciliationStatus(
        {
          checks_version: 2,
          provisional: true,
          differences: [
            {
              kind: "balance_vs_ledger",
              funding_source: "api_quota",
              expected_nano: "1000000000",
              actual_nano: "1000000001",
              delta_nano: "1",
            },
          ],
        },
        zh,
      ),
    ).toBe("存在差异");
  });
  it("does not certify reports created before the complete checks", () => {
    expect(
      reconciliationStatus(
        { checks_version: 0, provisional: false, differences: [] },
        zh,
      ),
    ).toBe("待重新核验");
  });
  it("names business differences and preserves one nano in detail amounts", () => {
    expect(differenceLabel("provider_order_vs_ledger", zh)).toBe(
      "渠道成功订单与充值流水",
    );
    expect(differenceLabel("key_usage_vs_details", zh)).toBe(
      "密钥当日用量与调用明细",
    );
    expect(formatMoney("1000000001", "detail")).toBe("$1.000000001");
    expect(formatMoney("1", "detail")).toBe("$0.000000001");
  });
});
