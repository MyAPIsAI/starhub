// Adapter between the backend Token API (/api/token) and the frontend
// PreviewKey shape used by the console's keys page.
//
// The backend stores quota as integer "quota units" (1 USD = 500,000
// quota, common.QuotaPerUSD). The frontend uses nano-USD strings (1 USD
// = 1,000,000,000 nano) for exact decimal display. This module owns the
// conversion so the UI components can keep their PreviewKey-shaped API.
//
// Key behavior:
//   - The list endpoint returns MASKED keys (sk-xxx******yyyy). The
//     plaintext key is only available from create/regenerate, captured
//     by the caller and merged into the adapted PreviewKey via the
//     plaintextKeys map. (B10)

import { useMemo } from "react";
import type { Token } from "./tokens";
import { useTokens } from "./tokens";
import type { PreviewKey } from "../product-preview";
import type { TierId } from "../../config/placeholders";

// --- Unit constants ---

// Backend: 1 USD = 500,000 quota units (common.QuotaPerUSD)
const QUOTA_PER_USD = 500_000;
// Frontend: 1 USD = 1,000,000,000 nano-USD
const NANO_PER_USD = 1_000_000_000;
// nano-USD per quota unit = 1e9 / 5e5 = 2000
const NANO_PER_QUOTA = BigInt(NANO_PER_USD / QUOTA_PER_USD);

// --- Status codes (common/config.go) ---
const STATUS_ENABLED = 1;

// --- Conversion helpers ---

/** Convert a USD amount to backend quota units. */
export function usdToQuota(usd: number): number {
  return Math.round(usd * QUOTA_PER_USD);
}

/** Convert backend quota units to a nano-USD string (exact integer). */
export function quotaToNanoUsd(quota: number): string {
  return (BigInt(Math.trunc(quota)) * NANO_PER_QUOTA).toString();
}

/** Convert backend quota units to a USD number (for PreviewKey.quotaLimit_usd). */
export function quotaToUsdNumber(quota: number): number {
  return quota / QUOTA_PER_USD;
}

// --- Group / tier mapping ---

/** Map a backend group string to a frontend tier ID. */
function groupToTier(group: string): TierId {
  // "default" and "" (no group) map to "pro" — the standard tier used
  // as the default in the create-key form.
  if (!group) return "default";
  // If the admin configured group ratios matching frontend tier IDs,
  // pass them through so the TierBadge renders the right label.
  return group;
}

/**
 * Map a frontend tier to a backend group. The backend validates group
 * against configured group ratios (setting.ContainsGroupRatio). "default"
 * is the only group configured out of the box, so all tiers map to it.
 * If the admin adds tier-matching groups, this can be relaxed.
 */
export function tierToGroup(tier: TierId): string {
  return tier;
}

// --- List helpers ---

/** Split a comma-separated string into a trimmed, filtered array. */
export function splitList(s: string): string[] {
  if (!s) return [];
  return s
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

/** Join an array into a comma-separated string. */
export function joinList(arr: string[]): string {
  return arr.join(",");
}

// --- Core adapter ---

/**
 * Convert a backend Token to a frontend PreviewKey.
 *
 * @param token       The backend token (key is masked from list endpoint).
 * @param plaintextKey  Optional plaintext key — only from create/regenerate.
 *                      When provided, it becomes `keySecret` so the UI can
 *                      reveal/copy the real key.
 */
export function tokenToPreviewKey(
  token: Token,
  plaintextKey?: string,
): PreviewKey {
  return {
    id: String(token.id),
    managementRevision: token.management_revision,
    name: token.name,
    // Backend doesn't track a single default model; left blank so
    // key-model-policy falls back to the group's model list.
    model: "",
    tier: groupToTier(token.group),
    active:
      token.status === STATUS_ENABLED &&
      (token.expired_time <= 0 || token.expired_time * 1000 > Date.now()),
    keyMasked: token.key,
    keySecret: plaintextKey ?? token.key,
    type: "client",
    smartAccess: token.smart_access === true,
    modelAccess: token.model_limits ? "custom" : "all",
    allowedModels: token.model_limits
      ? splitList(token.model_limits)
      : undefined,
    quotaLimit_usd: token.unlimited_quota
      ? null
      : quotaToUsdNumber(token.remain_quota),
    // Backend has no period-reset concept; "never" means the limit is
    // lifetime and totalUsed_nano is the sole usage counter.
    quotaPeriod: "never",
    currentPeriodUsed_nano: quotaToNanoUsd(token.used_quota),
    totalUsed_nano: quotaToNanoUsd(token.used_quota),
    validUntil:
      token.expired_time > 0
        ? new Date(token.expired_time * 1000).toISOString()
        : null,
    ipWhitelist: token.allow_ips ? splitList(token.allow_ips) : [],
    lastUsedAt: null,
    // Store raw remain_quota for the optimistic-concurrency guard
    // (expected_quota) in edit-key. The `quota` field is a legacy hint
    // slot on PreviewKey and is not used for display.
    quota: token.remain_quota,
  };
}

// --- Hook ---

/**
 * Fetch backend tokens and adapt them to PreviewKey[].
 *
 * @param plaintextKeys  A map of key-ID → plaintext key, populated by the
 *                       caller from create/regenerate mutation results.
 *                       Keys not in this map use the masked value.
 */
export function useBackendKeys(plaintextKeys: Record<string, string> = {}): {
  keys: PreviewKey[];
  loading: boolean;
  error: Error | null;
} {
  const { data: cached, isLoading, error } = useTokens();
  const data = error ? undefined : cached;
  const keys = useMemo(() => {
    if (!data) return [];
    return data.map((token) =>
      tokenToPreviewKey(token, plaintextKeys[String(token.id)]),
    );
  }, [data, plaintextKeys]);
  return { keys, loading: isLoading, error };
}
