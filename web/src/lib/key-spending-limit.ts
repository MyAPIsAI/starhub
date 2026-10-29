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
