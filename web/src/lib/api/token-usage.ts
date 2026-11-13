import { quotaMoneyNano } from "../money";
import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import { assertTokenSession } from "./tokens";
import { useSessionQueryIdentity } from "../system-access";

export interface TokenUsageAmounts {
  quota_nano?: string;
  unfunded_quota_nano?: string;
  requests: number;
  quota: number;
  unfunded_quota: number;
  prompt_tokens: number;
  completion_tokens: number;
  success_count: number;
  failed_count: number;
}
export interface TokenPeriodRecord {
  quota_limit_nano?: string;
  used_quota_nano?: string;
  reserved_quota_nano?: string;
  carry_quota_nano?: string;
  id: number;
  token_id: number;
  period: string;
  timezone: string;
  start: number;
  end: number;
  quota_limit: number;
  used_quota: number;
  reserved_quota: number;
  carry_quota: number;
  closed_at: number;
}
export interface TokenUsageDetails {
  schema: "token_usage_v1";
  user_id: number;
  token_id: number;
  observed_at: number;
  current: {
    used_quota_nano?: string;
    reserved_quota_nano?: string;
    quota_limit_nano?: string | null;
    remaining_quota_nano?: string | null;
    lifetime_used_quota_nano?: string;
    period: "never" | "daily" | "monthly";
    timezone: string;
    period_id: number;
    start: number;
    end: number;
    used_quota: number;
    reserved_quota: number;
    quota_limit: number | null;
    remaining_quota: number | null;
    lifetime_used_quota: number;
  };
  days: number;
  trend: ({ date: string } & TokenUsageAmounts)[];
  models: ({ model: string } & TokenUsageAmounts)[];
  model_page: number;
  model_total: number;
  periods: TokenPeriodRecord[];
  period_page: number;
  period_total: number;
  page_size: number;
}

const safe = (n: unknown): n is number =>
  Number.isSafeInteger(n) && Number(n) >= 0;
const validMoney = (exact: unknown, legacy: number) => {
  try {
    return BigInt(quotaMoneyNano(exact, legacy)) >= 0n;
  } catch {
    return false;
  }
};
const validTimezone = (value: unknown): value is string => {
  if (typeof value !== "string" || !value) return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value });
    return true;
  } catch {
    return false;
  }
};
export function validateTokenUsage(
  value: TokenUsageDetails,
  owner: number,
  tokenID: number,
): TokenUsageDetails {
  const current = value?.current;
  if (
    value?.schema !== "token_usage_v1" ||
    value.user_id !== owner ||
    value.token_id !== tokenID ||
    !current ||
    !["never", "daily", "monthly"].includes(current.period) ||
    !validTimezone(current.timezone) ||
    !safe(value.observed_at) ||
    ![current.period_id, current.start, current.end].every(safe) ||
    !validMoney(current.used_quota_nano, current.used_quota) ||
    !validMoney(current.reserved_quota_nano, current.reserved_quota) ||
    !validMoney(
      current.lifetime_used_quota_nano,
      current.lifetime_used_quota,
    ) ||
    (current.quota_limit !== null &&
      !validMoney(current.quota_limit_nano, current.quota_limit)) ||
    (current.remaining_quota !== null &&
      !validMoney(current.remaining_quota_nano, current.remaining_quota)) ||
    (current.quota_limit === null && current.quota_limit_nano != null) ||
    (current.remaining_quota === null &&
      current.remaining_quota_nano != null) ||
    value.days !== 30 ||
    !Array.isArray(value.trend) ||
    value.trend.length !== 30 ||
    !Array.isArray(value.models) ||
    !Array.isArray(value.periods) ||
    ![
      value.model_total,
      value.period_total,
      value.model_page,
      value.period_page,
      value.page_size,
    ].every(safe) ||
    value.page_size < 1 ||
    value.model_page < 1 ||
    value.period_page < 1
  )
    throw Error("Key usage was not confirmed");
  for (const row of [...value.trend, ...value.models]) {
    if (
      ![
        row.requests,
        row.prompt_tokens,
        row.completion_tokens,
        row.success_count,
        row.failed_count,
      ].every(safe) ||
      !validMoney(row.quota_nano, row.quota) ||
      !validMoney(row.unfunded_quota_nano, row.unfunded_quota)
    )
      throw Error("Key usage amounts were not confirmed");
  }
  if (
    value.trend.some((row) => !/^\d{4}-\d{2}-\d{2}$/.test(row.date)) ||
    value.models.some((row) => typeof row.model !== "string") ||
    value.periods.some(
      (row) =>
        row.token_id !== tokenID ||
        !validTimezone(row.timezone) ||
        !["daily", "monthly"].includes(row.period) ||
        ![row.id, row.start, row.end, row.closed_at].every(safe) ||
        !validMoney(row.quota_limit_nano, row.quota_limit) ||
        !validMoney(row.used_quota_nano, row.used_quota) ||
        !validMoney(row.reserved_quota_nano, row.reserved_quota) ||
        !validMoney(row.carry_quota_nano, row.carry_quota),
    )
  )
    throw Error("Key usage scope was not confirmed");
  return value;
}

export function useOwnedTokenUsage(
  tokenID: number,
  periodPage: number,
  modelPage: number,
) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["owned-token-usage", identity, tokenID, periodPage, modelPage],
    enabled:
      identity[1] !== null && Number.isSafeInteger(tokenID) && tokenID > 0,
    retry: false,
    refetchInterval: 30_000,
    queryFn: async ({ signal }) => {
      assertTokenSession(identity);
      const value = await apiGet<TokenUsageDetails>(
        `/api/token/${tokenID}/usage?period_page=${periodPage}&model_page=${modelPage}`,
        signal,
      );
      assertTokenSession(identity);
      return validateTokenUsage(value, identity[1]!, tokenID);
    },
  });
}
