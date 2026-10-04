import { describe, it, expect } from "vitest";
import {
  NANO_PER_QUOTA,
  QUOTA_PER_USD,
  NANO_PER_USD,
  quotaToNano,
  nanoToQuota,
  usdToQuota,
  usdToNano,
} from "./quota";

describe("quota conversion constants", () => {
  it("1 USD = 500000 quota", () => {
    expect(QUOTA_PER_USD).toBe(500_000);
  });

  it("1 USD = 1e9 nano", () => {
    expect(NANO_PER_USD).toBe(1_000_000_000);
  });

  it("1 quota = 2000 nano", () => {
    expect(NANO_PER_QUOTA).toBe(2000);
  });
});

describe("quotaToNano", () => {
  it("converts zero", () => {
    expect(quotaToNano(0)).toBe(0);
  });

  it("converts 1 quota to 2000 nano", () => {
    expect(quotaToNano(1)).toBe(2000);
  });

  it("converts 500000 quota (1 USD) to 1e9 nano", () => {
    expect(quotaToNano(500_000)).toBe(1_000_000_000);
  });
});

describe("nanoToQuota", () => {
  it("converts zero", () => {
    expect(nanoToQuota(0)).toBe(0);
  });

  it("converts 2000 nano to 1 quota", () => {
    expect(nanoToQuota(2000)).toBe(1);
  });

  it("rounds to nearest whole quota unit", () => {
    expect(nanoToQuota(1000)).toBe(1);
    expect(nanoToQuota(999)).toBe(0);
  });

  it("converts 1e9 nano (1 USD) to 500000 quota", () => {
    expect(nanoToQuota(1_000_000_000)).toBe(500_000);
  });

  it("quotaToNano then nanoToQuota is identity for integer quota", () => {
    for (const q of [0, 1, 42, 500_000, 12_345_678]) {
      expect(nanoToQuota(quotaToNano(q))).toBe(q);
    }
  });
});

describe("usdToQuota / usdToNano", () => {
  it("1 USD = 500000 quota", () => {
    expect(usdToQuota(1)).toBe(500_000);
  });

  it("1 USD = 1e9 nano", () => {
    expect(usdToNano(1)).toBe(1_000_000_000);
  });

  it("0.50 USD = 250000 quota", () => {
    expect(usdToQuota(0.5)).toBe(250_000);
  });
});
