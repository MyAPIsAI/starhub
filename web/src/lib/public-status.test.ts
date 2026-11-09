import { describe, expect, it } from "vitest";
import { statusPeriod } from "./public-status";
import type { StatusDay } from "./api/public-pages";
const day = (samples: number, successful: number): StatusDay => ({
  date: "2026-11-09",
  samples,
  successful,
  success_rate: samples < 50 ? null : successful / samples,
  state: "insufficient_data",
});
describe("status period aggregation", () => {
  it("weights requests instead of averaging daily percentages, including gray days", () => {
    expect(statusPeriod([day(1, 1), day(99, 0)])).toEqual({
      samples: 100,
      successRate: 0.01,
    });
    expect(statusPeriod([day(1, 1), day(50, 2)]).successRate).toBe(3 / 51);
  });
  it("preserves zero success and distinguishes missing history", () => {
    expect(statusPeriod([day(50, 0)]).successRate).toBe(0);
    expect(statusPeriod([day(0, 0)]).successRate).toBeNull();
    expect(statusPeriod([]).successRate).toBeNull();
    expect(
      statusPeriod([
        { ...day(20, 1), successful: undefined } as unknown as StatusDay,
      ]).successRate,
    ).toBeNull();
  });
});
