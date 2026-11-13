import { decimalUsdToNano } from "./money";
import { decimalUsdNano, type PreviewKey } from "./product-preview";

export function keySpendingLimitValue(key?: PreviewKey): string {
  if (!key || key.quotaLimit_usd == null) return "";
  const nano =
    key.quotaLimit_nano == null
      ? decimalUsdNano(key.quotaLimit_usd)
      : BigInt(key.quotaLimit_nano);
  const fraction = (nano % 1_000_000_000n)
    .toString()
    .padStart(9, "0")
    .replace(/0+$/, "");
  return `${nano / 1_000_000_000n}${fraction ? `.${fraction}` : ""}`;
}

export function remainingQuotaForLifetimeLimit(
  value: string,
  key?: PreviewKey,
): number {
  if (!/^\d+(\.\d{1,6})?$/.test(value)) throw Error("invalid_amount");
  const [whole, fraction = ""] = value.split(".");
  const micro = BigInt(whole) * 1_000_000n + BigInt(fraction.padEnd(6, "0"));
  // Round down to the backend's two-microdollar unit, never above the limit.
  const totalQuota = micro / 2n;
  const usedNano = BigInt(key?.totalUsed_nano ?? "0");
  const usedQuota = (usedNano + 1999n) / 2000n;
  if (totalQuota < usedQuota) throw Error("below_used");
  const remaining = totalQuota - usedQuota;
  if (remaining > BigInt(Number.MAX_SAFE_INTEGER)) throw Error("too_large");
  return Number(remaining);
}

export function quotaForPeriodLimit(value: string, key?: PreviewKey): number {
  const limit = remainingQuotaForLifetimeLimit(value);
  const held = BigInt(key?.currentPeriodReserved_nano ?? "0");
  const used =
    key?.quotaPeriod !== "never"
      ? BigInt(key?.currentPeriodUsed_nano ?? "0")
      : 0n;
  if (BigInt(limit) * 2000n < held + used) throw Error("below_used");
  return limit;
}

export function remainingNanoForLifetimeLimit(
  value: string,
  key?: PreviewKey,
): string {
  const total = BigInt(decimalUsdToNano(value));
  if (total < 0n) throw Error("invalid_amount");
  if (total / 2000n > 9_007_199_254_740_991n) throw Error("too_large");
  const used = BigInt(key?.totalUsed_nano ?? "0");
  const pending = BigInt(key?.currentPeriodReserved_nano ?? "0");
  if (total < used + pending) throw Error("below_used");
  return (total - used).toString();
}

export function nanoForPeriodLimit(value: string, key?: PreviewKey): string {
  const total = BigInt(remainingNanoForLifetimeLimit(value));
  const held = BigInt(key?.currentPeriodReserved_nano ?? "0");
  const used =
    key?.quotaPeriod !== "never"
      ? BigInt(key?.currentPeriodUsed_nano ?? "0")
      : 0n;
  if (total < held + used) throw Error("below_used");
  return total.toString();
}
