import { describe, it, expect } from "vitest";
import { tokenToPreviewKey } from "../src/lib/api/key-adapter";
import { keyQuotaSummary } from "../src/lib/key-quota-summary";
import { quotaForPeriodLimit } from "../src/lib/key-spending-limit";
import type { Token } from "../src/lib/api/tokens";

const sample: Token = {
  id: 1,
  user_id: 1,
  name: "period",
  key: "masked",
  status: 1,
  group: "default",
  expired_time: -1,
  remain_quota: 700,
  unlimited_quota: false,
  used_quota: 10000,
  model_limits: "model",
  allow_ips: "",
  auto_groups: "",
  created_at: 0,
  quota_period: "daily",
  period_quota_limit: 1000,
  period_used_quota: 200,
  period_reserved_quota: 100,
  quota_timezone: "Asia/Shanghai",
  quota_region: "cn",
  period_start: 1794499200,
  period_end: 1794585600,
};

describe("server-owned key quota periods", () => {
  it("separates current spending, held quota, allowance and lifetime totals", () => {
    const key = tokenToPreviewKey(sample),
      summary = keyQuotaSummary(key);
    expect(summary.usedNano).toBe("400000");
    expect(summary.reservedNano).toBe("200000");
    expect(summary.remainingNano).toBe("1400000");
    expect(key.totalUsed_nano).toBe("20000000");
    expect(key.quotaTimezone).toBe("Asia/Shanghai");
    expect(summary.limitNano).toBe("2000000");
  });
  it("waits for server rollover instead of resetting a stale snapshot in Dubai time", () => {
    const key = tokenToPreviewKey(sample);
    expect(keyQuotaSummary(key, "2030-01-01T00:00:00Z").usedNano).toBe(
      "400000",
    );
  });
  it("validates a period limit against current spending rather than lifetime spending", () => {
    const key = tokenToPreviewKey(sample);
    expect(quotaForPeriodLimit("0.002", key)).toBe(1000);
    expect(() => quotaForPeriodLimit("0.0001", key)).toThrow("below_used");
    expect(() => quotaForPeriodLimit("1.0000001", key)).toThrow(
      "invalid_amount",
    );
  });
  it("includes pending lifetime reservations when reconstructing the old lifetime allowance", () => {
    const key = tokenToPreviewKey({
      ...sample,
      quota_period: "never",
      remain_quota: 700,
      used_quota: 200,
      pending_quota: 100,
    });
    expect(key.quotaLimit_nano).toBe("2000000");
  });
});
