import { describe, it, expect } from "vitest";
import {
  differenceLabel,
  reconciliationStatus,
} from "../src/features/admin-reconciliation";
import { formatMoney } from "../src/lib/money";
const zh = (a: string) => a;
describe("Financial reconciliation presentation", () => {
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
