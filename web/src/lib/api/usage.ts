// Usage and logs API hooks — wrappers around the Go backend's
// /api/user/usage and /api/log endpoints.
//
// These power the console overview's metrics, charts, and recent-
// requests table with real backend data instead of the demo store.

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";

// --- Types matching the backend DailyUsage struct (model/log.go) ---

export interface DailyUsage {
  date: string; // "2006-01-02"
  requests: number;
  quota: number; // backend quota units (1 USD = 500,000 quota)
  unfunded_quota: number;
  prompt_tokens: number;
  completion_tokens: number;
  success_count: number;
  failed_count: number;
}

export interface ModelDailyUsage {
  date: string;
  model: string;
  requests: number;
  quota: number;
  prompt_tokens: number;
  completion_tokens: number;
}

export interface UsageResponse {
  days: number;
  items: DailyUsage[];
  by_model?: ModelDailyUsage[];
}

// --- Types matching the backend Log struct (model/log.go) ---

export interface LogRow {
  id: number;
  user_id: number;
  token_id: number;
  token_name: string;
  channel_id: number;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  quota: number;
  unfunded_quota: number;
  used_channels: string;
  duration_ms: number;
  status: number; // 1=success, 2=failed
  created_at: number; // unix seconds
}

export interface LogsResponse {
  items: LogRow[];
  total: number;
  page: number;
  page_size: number;
}

// --- Hooks ---

export function useUserUsage(days: number = 7) {
  return useQuery({
    queryKey: ["user-usage", days] as const,
    queryFn: ({ signal }) =>
      apiGet<UsageResponse>(
        `/api/user/usage?days=${days}`,
        signal,
      ),
  });
}

export function useLogs(page: number = 1, pageSize: number = 20) {
  return useQuery({
    queryKey: ["logs", page, pageSize] as const,
    queryFn: ({ signal }) =>
      apiGet<LogsResponse>(
        `/api/log?p=${page}&page_size=${pageSize}`,
        signal,
      ),
  });
}

// --- Constants for unit conversion ---

// Backend: 1 USD = 500,000 quota units (common.QuotaPerUSD)
const QUOTA_PER_USD = 500_000;
// Frontend: 1 USD = 1,000,000,000 nano-USD
const NANO_PER_USD = 1_000_000_000;
const NANO_PER_QUOTA = BigInt(NANO_PER_USD / QUOTA_PER_USD); // 2000

/** Convert backend quota units to a nano-USD string. */
export function quotaToNanoUsd(quota: number): string {
  return (BigInt(Math.trunc(quota)) * NANO_PER_QUOTA).toString();
}

/** Convert backend quota units to a USD number. */
export function quotaToUsdNumber(quota: number): number {
  return quota / QUOTA_PER_USD;
}

// --- Adapter: LogRow -> PreviewRequest-compatible shape ---

/**
 * Adapt a backend LogRow to the PreviewRequest shape expected by the
 * request-history component. Fields the backend doesn't have (vendor,
 * duration_ms, status, error_code) are filled with sensible defaults.
 */
export function adaptLogRow(log: LogRow): PreviewRequestLike {
  return {
    trace_id: `log_${log.id}`,
    time: new Date(log.created_at * 1000).toISOString(),
    model: log.model,
    vendor: log.model.split("-")[0] ?? log.model,
    status: log.status === 2 ? "failed" : "success",
    cost_nano: quotaToNanoUsd(log.quota),
    duration_ms: log.duration_ms,
    key_id: log.token_id ? String(log.token_id) : "",
    input_tokens: log.prompt_tokens,
    output_tokens: log.completion_tokens,
    error_code: "",
    error_message: "",
  };
}

export interface PreviewRequestLike {
  trace_id: string;
  time: string;
  model: string;
  vendor: string;
  status: "success" | "failed";
  cost_nano: string;
  duration_ms: number;
  key_id: string;
  input_tokens: number;
  output_tokens: number;
  error_code: string;
  error_message: string;
}

/**
 * Fetch backend logs with a larger page size for the requests page.
 * Returns adapted PreviewRequest-compatible objects.
 */
export function useBackendRequests(pageSize: number = 100) {
  const { data, isLoading, error } = useLogs(1, pageSize);
  const rows = (data?.items ?? []).map(adaptLogRow);
  return { rows, total: data?.total ?? 0, loading: isLoading, error };
}
