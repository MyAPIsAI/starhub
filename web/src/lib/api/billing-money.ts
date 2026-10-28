const MIN_INT64 = -(1n << 63n);
const MAX_INT64 = (1n << 63n) - 1n;

/** Prefer exact server strings. Unsafe legacy numbers have already lost precision. */
export function billingNano(exact: unknown, legacy: unknown): string {
  if (exact !== undefined) {
    if (
      typeof exact !== "string" ||
      !/^-?(0|[1-9]\d*)$/.test(exact) ||
      exact.length > 20
    )
      throw new Error("Invalid exact billing amount");
    const amount = BigInt(exact);
    if (amount < MIN_INT64 || amount > MAX_INT64)
      throw new Error("Billing amount exceeds int64");
    return amount.toString();
  }
  if (typeof legacy !== "number" || !Number.isSafeInteger(legacy))
    throw new Error("Legacy billing amount is not exact");
  return BigInt(legacy).toString();
}
