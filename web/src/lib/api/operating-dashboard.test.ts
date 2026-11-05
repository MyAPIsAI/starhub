import { describe, expect, it } from "vitest";
import {
  formatOperatingAmount,
  isOperatingDashboard,
  operatingChart,
  operatingShare,
  type OperatingDashboard,
} from "./operating-dashboard";

function report(): OperatingDashboard {
  return {
    timezone: "Asia/Dubai",
    region_id: "me",
    start_date: "2026-11-01",
    end_date: "2026-11-05",
    since: 1,
    until: 5,
    data_until: 4,
    updated_at: 4,
    refresh_seconds: 300,
    granularity: "day",
    totals: {
      recharge_nano: "0",
      consumption_nano: "0",
      requests: 0,
      successful: 0,
      failed: 0,
      cancelled: 0,
      active_accounts: 0,
      new_accounts: 0,
      paying_accounts: 0,
    },
    funding: {
      offline_nano: "0",
      simulation_nano: "0",
      unverified_nano: "0",
      other_credit_nano: "0",
    },
    series: [],
    groups: [],
    vendors: [],
    call_types: [],
    models: [],
    top_accounts: [],
    top_models: [],
  };
}
describe("operating dashboard contract and chart", () => {
  it("keeps sub-cent consumption visible and formats large amounts without floating-point loss", () => {
    expect(formatOperatingAmount("0")).toBe("$0.00");
    expect(formatOperatingAmount("66000000000000")).toBe("$66,000.00");
    expect(formatOperatingAmount("1000")).toBe("$0.000001");
    expect(formatOperatingAmount("1")).toBe("<$0.000001");
    expect(formatOperatingAmount("18446744073709551614")).toBe(
      "$18,446,744,073.71",
    );
  });
  it("accepts an explicit empty report but rejects missing, malformed and lossy money", () => {
    expect(isOperatingDashboard(report())).toBe(true);
    for (const bad of [
      undefined,
      {},
      { ...report(), series: [null] },
      { ...report(), timezone: "invalid-zone" },
      {
        ...report(),
        totals: { ...report().totals, consumption_nano: 9007199254740993 },
      },
      {
        ...report(),
        groups: [{ id: "g", name: "g", amount_nano: "1.1", requests: 1 }],
      },
    ])
      expect(isOperatingDashboard(bad)).toBe(false);
  });
  it("keeps exact tooltip amounts and accounts for every group in Other", () => {
    const data = report();
    data.groups = Array.from({ length: 10 }, (_, index) => ({
      id: index === 0 ? "__proto__" : `group.${index}`,
      name: "",
      amount_nano: String(9007199254740993n - BigInt(index)),
      requests: 1,
    }));
    data.series = [
      {
        at: 1,
        amount_nano: data.groups
          .reduce((sum, row) => sum + BigInt(row.amount_nano), 0n)
          .toString(),
        groups: data.groups,
      },
    ];
    const chart = operatingChart(data);
    expect(chart.groups).toHaveLength(7);
    expect(chart.hasOther).toBe(true);
    expect(chart.points[0].exact.g0).toBe("9007199254740993");
    expect(
      Object.values(chart.points[0].exact)
        .reduce((sum, value) => sum + BigInt(value), 0n)
        .toString(),
    ).toBe(data.series[0].amount_nano);
    expect(Object.keys(chart.points[0])).not.toContain("__proto__");
  });
  it("calculates shares from exact integers without using the largest row as 100%", () => {
    expect(operatingShare("38000000000", "100000000000")).toBe(38);
    expect(operatingShare("9007199254740993", "18014398509481986")).toBe(50);
    expect(operatingShare("0", "0")).toBe(0);
  });
});
