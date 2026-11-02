import { describe, expect, it } from "vitest";
import { tokenToPreviewKey } from "./key-adapter";
import type { Token } from "./tokens";
import { getClientModels } from "../client-config";
import { keyQuotaSummary } from "../key-quota-summary";
import {
  keySpendingLimitValue,
  remainingQuotaForLifetimeLimit,
} from "../key-spending-limit";

const token: Token = {
  id: 1,
  user_id: 1,
  name: "Review",
  key: "sk-review******mask",
  status: 1,
  group: "pro",
  expired_time: -1,
  remain_quota: 1000000,
  unlimited_quota: true,
  used_quota: 0,
  model_limits: "gpt-4.1",
  allow_ips: "",
  auto_groups: "",
  created_at: 0,
};
describe("backend smart-routing capability", () => {
  it("uses the explicit capability and preserves the actual-model allowlist", () => {
    const key = tokenToPreviewKey({ ...token, smart_access: true });
    expect(getClientModels(key, "openai").map((model) => model.id)).toEqual([
      "gpt-4.1",
      "myapis-auto",
    ]);
    expect(getClientModels(key, "claude-code")).toEqual([]);
    expect(key.allowedModels).toEqual(["gpt-4.1"]);
    expect(key.keySecret).toBe(token.key);
  });
  it.each([false, undefined])(
    "does not infer missing smart permission: %s",
    (smart_access) => {
      const key = tokenToPreviewKey({ ...token, smart_access });
      expect(getClientModels(key, "openai").map((model) => model.id)).toEqual([
        "gpt-4.1",
      ]);
    },
  );
  it("does not turn a virtual-only allowlist into actual model access", () => {
    const key = tokenToPreviewKey({
      ...token,
      smart_access: true,
      model_limits: "veyra-auto,myapis-auto",
    });
    expect(getClientModels(key, "openai")).toEqual([]);
  });
  it("does not treat an expired key as active just because its status is enabled", () => {
    const key = tokenToPreviewKey({
      ...token,
      smart_access: true,
      expired_time: Math.floor(Date.now() / 1000) - 60,
    });
    expect(key.active).toBe(false);
  });
});

describe("backend lifetime spending limits", () => {
  it.each([
    { used: 40, remaining: 60 },
    { used: 60, remaining: 40 },
    { used: 100, remaining: 0 },
  ])(
    "keeps used=$used and remaining=$remaining distinct",
    ({ used, remaining }) => {
      const key = tokenToPreviewKey({
        ...token,
        unlimited_quota: false,
        used_quota: used * 500_000,
        remain_quota: remaining * 500_000,
      });
      expect(key.quotaLimit_usd).toBe(100);
      expect(keySpendingLimitValue(key)).toBe("100");
      expect(keyQuotaSummary(key)).toMatchObject({
        usedNano: String(BigInt(used) * 1_000_000_000n),
        remainingNano: String(BigInt(remaining) * 1_000_000_000n),
        exhausted: remaining === 0,
      });
      expect(remainingQuotaForLifetimeLimit("100", key)).toBe(
        remaining * 500_000,
      );
    },
  );

  it("round-trips a limit whose sum exceeds safe integer quota units", () => {
    const key = tokenToPreviewKey({
      ...token,
      unlimited_quota: false,
      remain_quota: Number.MAX_SAFE_INTEGER,
      used_quota: 1,
    });
    expect(key.quotaLimit_nano).toBe("18014398509481984000");
    expect(keySpendingLimitValue(key)).toBe("18014398509.481984");
    expect(
      remainingQuotaForLifetimeLimit(keySpendingLimitValue(key), key),
    ).toBe(Number.MAX_SAFE_INTEGER);
    expect(keyQuotaSummary(key).remainingNano).toBe("18014398509481982000");
  });

  it("distinguishes unlimited keys from a zero lifetime limit", () => {
    const unlimited = tokenToPreviewKey(token);
    expect(keySpendingLimitValue(unlimited)).toBe("");
    expect(keyQuotaSummary(unlimited).remainingNano).toBeNull();
    const zero = tokenToPreviewKey({
      ...token,
      unlimited_quota: false,
      used_quota: 0,
      remain_quota: 0,
    });
    expect(keySpendingLimitValue(zero)).toBe("0");
    expect(keyQuotaSummary(zero).exhausted).toBe(true);
  });

  it("rejects limits below recorded spend and keeps backend-unit precision", () => {
    const key = tokenToPreviewKey({ ...token, used_quota: 20_000_000 });
    expect(() => remainingQuotaForLifetimeLimit("39.999998", key)).toThrow(
      "below_used",
    );
    expect(remainingQuotaForLifetimeLimit("40", key)).toBe(0);
    expect(remainingQuotaForLifetimeLimit("40.000002", key)).toBe(1);
    expect(remainingQuotaForLifetimeLimit("0.000001")).toBe(0);
    for (const invalid of ["-1", "1e3", "1.0000001", "NaN"])
      expect(() => remainingQuotaForLifetimeLimit(invalid)).toThrow(
        "invalid_amount",
      );
    expect(() =>
      remainingQuotaForLifetimeLimit("999999999999999999999"),
    ).toThrow("too_large");
  });
});
