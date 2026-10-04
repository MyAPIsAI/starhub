import { describe, expect, it } from "vitest";
import { formatMoney, sumNano } from "../src/lib/money";

describe("M4 §2.3 display precision", () => {
  it("preserves a 296-nanodollar charge in the detail view", () => {
    expect(formatMoney("296", "call")).toBe("<$0.000001");
    expect(formatMoney("296", "detail")).toBe("$0.000000296");
    expect(formatMoney("0", "call")).toBe("$0.000000");
    expect(formatMoney("1500", "call")).toBe("$0.000002");
  });
  it("does not lose digits beyond JavaScript safe integers or int64 sums", () => {
    expect(formatMoney("12345678123456789", "detail")).toBe(
      "$12,345,678.123456789",
    );
    expect(sumNano(["10000000000000000000", "296"])).toBe(
      "10000000000000000296",
    );
    expect(formatMoney("10000000000000000296", "detail")).toBe(
      "$10,000,000,000.000000296",
    );
  });
  it("rounds totals only after summation and keeps negative signs", () => {
    expect(
      formatMoney(sumNano(["44444444", "44444444", "44444444"]), "total"),
    ).toBe("$0.1333");
    expect(formatMoney("-300000000", "balance")).toBe("−$0.30");
  });
});
