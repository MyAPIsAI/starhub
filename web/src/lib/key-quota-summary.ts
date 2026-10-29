import {
  decimalUsdNano,
  keyPeriodUsed,
  type PreviewKey,
} from "./product-preview";

/** A read-only display of the same quota period and precision used by charging. */
export function keyQuotaSummary(
  key: PreviewKey,
  now = new Date().toISOString(),
) {
  const used = keyPeriodUsed(key, now);
  const limit =
    key.quotaLimit_nano != null
      ? BigInt(key.quotaLimit_nano)
      : key.quotaLimit_usd == null
        ? null
        : decimalUsdNano(key.quotaLimit_usd);
  return {
    usedNano: used.toString(),
    limitNano: limit?.toString() ?? null,
    remainingNano:
      limit === null ? null : (limit > used ? limit - used : 0n).toString(),
    percentage:
      limit === null
        ? null
        : limit === 0n || used >= limit
          ? 100
          : Number((used * 10_000n) / limit) / 100,
    exhausted: limit !== null && used >= limit,
    exceeded: limit !== null && used > limit,
    nearLimit: limit !== null && (limit === 0n || used * 100n >= limit * 90n),
    period: key.quotaPeriod ?? "never",
  };
}
