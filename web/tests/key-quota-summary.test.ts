import { describe, expect, it } from "vitest";
import { keyQuotaSummary } from "../src/lib/key-quota-summary";
import type { PreviewKey } from "../src/lib/product-preview";

const NOW = "2026-09-28T12:00:00.000Z";
const key = (overrides: Partial<PreviewKey> = {}): PreviewKey => ({
  id: "quota-display-test",
  name: "额度显示测试",
  model: "gpt-4.1",
  tier: "pro",
  active: true,
  keyMasked: "sk-test-••••",
  keySecret: "unused-test-credential",
  type: "client",
  quotaLimit_usd: 500,
  quotaPeriod: "monthly",
  usagePeriodStartedAt: NOW,
  currentPeriodUsed_nano: "28450000000",
  totalUsed_nano: "48900000000",
  ...overrides,
});

describe("key list quota mirrors the charging period", () => {
  it("ignores the old static percentage and derives 5.69% from 28.45 / 500", () => {
    expect(keyQuotaSummary(key({ quota: 38 }), NOW)).toMatchObject({
      usedNano: "28450000000",
      limitNano: "500000000000",
      remainingNano: "471550000000",
      percentage: 5.69,
      nearLimit: false,
      exhausted: false,
    });
  });

  it("uses lifetime spend for a non-resetting key and hides unlimited progress", () => {
    expect(
      keyQuotaSummary(
        key({ quota: 14, quotaPeriod: "never", quotaLimit_usd: null }),
        NOW,
      ),
    ).toMatchObject({
      usedNano: "48900000000",
      limitNano: null,
      remainingNano: null,
      percentage: null,
      nearLimit: false,
      exhausted: false,
      period: "never",
    });
    expect(
      keyQuotaSummary(
        key({ quotaPeriod: undefined, quotaLimit_usd: undefined }),
        NOW,
      ),
    ).toMatchObject({
      usedNano: "48900000000",
      percentage: null,
      period: "never",
    });
  });

  it("distinguishes a zero-dollar limit from unlimited", () => {
    expect(
      keyQuotaSummary(
        key({ quotaLimit_usd: 0, currentPeriodUsed_nano: "0" }),
        NOW,
      ),
    ).toMatchObject({
      usedNano: "0",
      limitNano: "0",
      remainingNano: "0",
      percentage: 100,
      nearLimit: true,
      exhausted: true,
    });
  });

  it("preserves exact spend and caps the visual bar when consumption exceeds a reduced limit", () => {
    expect(keyQuotaSummary(key({ quotaLimit_usd: 20 }), NOW)).toMatchObject({
      usedNano: "28450000000",
      limitNano: "20000000000",
      remainingNano: "0",
      percentage: 100,
      nearLimit: true,
      exhausted: true,
    });
  });

  it("compares the warning threshold using nano-dollars, not a rounded percentage", () => {
    const base = key({
      quotaLimit_usd: 0.00000001,
      currentPeriodUsed_nano: "8",
    });
    expect(keyQuotaSummary(base, NOW)).toMatchObject({
      percentage: 80,
      nearLimit: false,
    });
    expect(
      keyQuotaSummary({ ...base, currentPeriodUsed_nano: "9" }, NOW),
    ).toMatchObject({
      percentage: 90,
      nearLimit: true,
      exhausted: false,
    });
    expect(
      keyQuotaSummary(
        key({ quotaLimit_usd: 1, currentPeriodUsed_nano: "899999999" }),
        NOW,
      ).nearLimit,
    ).toBe(false);
  });

  it("resets displayed daily usage at the same Dubai midnight as enforcement", () => {
    const daily = key({
      quotaPeriod: "daily",
      usagePeriodStartedAt: "2026-09-27T19:59:59.000Z",
    });
    expect(keyQuotaSummary(daily, "2026-09-27T19:59:59.999Z").usedNano).toBe(
      "28450000000",
    );
    expect(keyQuotaSummary(daily, "2026-09-27T20:00:00.000Z")).toMatchObject({
      usedNano: "0",
      remainingNano: "500000000000",
      percentage: 0,
    });
  });

  it("resets monthly usage at the Dubai month boundary without losing lifetime spend", () => {
    const monthly = key({ usagePeriodStartedAt: "2026-09-30T19:59:59.000Z" });
    expect(keyQuotaSummary(monthly, "2026-09-30T19:59:59.999Z").usedNano).toBe(
      "28450000000",
    );
    expect(keyQuotaSummary(monthly, "2026-09-30T20:00:00.000Z").usedNano).toBe(
      "0",
    );
    expect(monthly.totalUsed_nano).toBe("48900000000");
  });

  it("keeps legacy usage when a periodic key has no recorded start or last-use date", () => {
    expect(
      keyQuotaSummary(
        key({ usagePeriodStartedAt: undefined, lastUsedAt: null }),
        NOW,
      ).usedNano,
    ).toBe("28450000000");
  });
});
