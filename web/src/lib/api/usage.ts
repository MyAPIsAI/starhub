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
}

export interface UsageResponse {
  days: number;
  items: DailyUsage[];
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
