import { describe, expect, it } from "vitest";
import {
  parseUsageReport,
  UsageReportContractError,
} from "../src/lib/usage-report-contract";

function emptyReport() {
  return {
    timezone: "Asia/Dubai",
    start_date: "2026-11-01",
    end_date: "2026-11-03",
    group_by: "key",
    granularity: "day",
    totals: {
      id: "",
      name: "",
      requests: 0,
      successful: 0,
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      amount_nano: "0",
    },
    rows: [],
    series: [],
    by_key: [],
    by_model: [],
    options: {},
  };
}

describe("usage API contract boundary", () => {
  it("accepts a genuine empty report and preserves exact monetary strings", () => {
    const report = emptyReport();
    report.totals.amount_nano = "9223372036854775807000";
    expect(parseUsageReport(report)).toBe(report);
    expect(parseUsageReport(report).totals.amount_nano).toBe(
      "9223372036854775807000",
    );
  });
  it.each([
    [
      "legacy user list",
      { items: [{ username: "private-account" }], total: 1 },
    ],
    ["missing report", undefined],
    ["incomplete report", { totals: { amount_nano: "0" } }],
  ])("rejects %s before putting it in the report cache", (_name, payload) => {
    expect(() => parseUsageReport(payload)).toThrow(UsageReportContractError);
  });
  it("rejects malformed money and unsupported timezones before rendering", () => {
    const report = emptyReport();
    report.totals.amount_nano = "invalid money";
    expect(() => parseUsageReport(report)).toThrow(UsageReportContractError);
    report.totals.amount_nano = "0";
    report.timezone = "Unsupported/Timezone";
    expect(() => parseUsageReport(report)).toThrow(UsageReportContractError);
  });
});
