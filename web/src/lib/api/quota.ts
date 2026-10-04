// Quota <-> nano-USD conversion.
//
// The Go backend stores money as integer "quota" units where
// 1 USD = 500_000 quota (common/config.go:25). The frontend uses
// nano-USD strings (1 USD = 1_000_000_000 nano) for BigInt-safe
// money math. The conversion factor is therefore:
//
//   1 quota = 1_000_000_000 / 500_000 nano = 2000 nano
//
// All API boundary code must route through these helpers so the
// factor lives in exactly one place. Add to quota.test.ts when
// extending.

export const QUOTA_PER_USD = 500_000;
export const NANO_PER_USD = 1_000_000_000;

/** nano-USD per single quota unit (exact, no remainder). */
export const NANO_PER_QUOTA = NANO_PER_USD / QUOTA_PER_USD; // 2000

/** Convert backend quota to frontend nano-USD. */
export const quotaToNano = (quota: number): number => quota * NANO_PER_QUOTA;

/** Convert frontend nano-USD to backend quota, rounded to the
 * nearest whole unit. Use this when sending money values to the API. */
export const nanoToQuota = (nano: number): number =>
  Math.round(nano / NANO_PER_QUOTA);

/** Convert a USD amount to quota units. */
export const usdToQuota = (usd: number): number =>
  Math.round(usd * QUOTA_PER_USD);

/** Convert a USD amount to nano-USD. */
export const usdToNano = (usd: number): number =>
  Math.round(usd * NANO_PER_USD);
