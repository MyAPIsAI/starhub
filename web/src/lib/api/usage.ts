// Usage and logs API hooks — wrappers around the Go backend's
// /api/user/usage and /api/log endpoints.
//
// These power the console overview's metrics, charts, and recent-
// requests table with real backend data instead of the demo store.

import { quotaMoneyNano } from "../money";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import {
  requestTimeBounds,
  type RequestTimeRange,
  type RequestHistoryStatus,
} from "../request-history";
import { apiGet } from "./http";
import { catalogModels } from "../../config/models";

// --- Types matching the backend DailyUsage struct (model/log.go) ---

export interface DailyUsage {
  quota_nano?: string;
  unfunded_quota_nano?: string;
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
  quota_nano?: string;
  unfunded_quota_nano?: string;
  date: string;
  model: string;
  requests: number;
  quota: number;
  prompt_tokens: number;
  completion_tokens: number;
}

export interface TokenDailyUsage {
  quota_nano?: string;
  unfunded_quota_nano?: string;
  date: string;
  token_id: number;
  token_name: string;
  requests: number;
  quota: number;
  prompt_tokens: number;
  completion_tokens: number;
}

export interface UsageResponse {
  days: number;
  items: DailyUsage[];
  by_model?: ModelDailyUsage[];
  by_token?: TokenDailyUsage[];
}

// --- Types matching the backend Log struct (model/log.go) ---

export interface LogRow {
  request_id?: string;
  http_status?: number;
  failure_stage?: string;
  error_code?: string;
  error_message?: string;
  request_observed?: boolean;
  admission_rejected?: boolean;
  upstream_attempts?: number;
  quota_nano?: string;
  unfunded_quota_nano?: string;
  id: number;
  user_id: number;
  token_id: number;
  token_name: string;
  channel_id: number;
  model: string;
  requested_model?: string;
  protocol?: string;
  usage_source?: string;
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
  filtering_supported?: boolean;
  items: LogRow[];
  total: number;
  page: number;
  page_size: number;
  summary?: LogPageSummary;
}

export interface LogPageSummary {
  observed_requests: number;
  rejected_requests: number;
  upstream_attempts: number;
  historical_requests: number;
  success_count: number;
  failed_count: number;
  cancelled_count: number;
}

// --- Hooks ---

export function useUserUsage(days: number = 7) {
  return useQuery({
    queryKey: ["user-usage", days] as const,
    queryFn: ({ signal }) =>
      apiGet<UsageResponse>(`/api/user/usage?days=${days}`, signal),
  });
}

export function useTokenUsage(tokenId: number, days: number = 7) {
  return useQuery({
    queryKey: ["token-usage", tokenId, days] as const,
    queryFn: ({ signal }) =>
      apiGet<UsageResponse>(
        `/api/user/usage?days=${days}&token_id=${tokenId}`,
        signal,
      ),
    enabled: tokenId > 0,
  });
}

export function useLogs(page: number = 1, pageSize: number = 20) {
  return useQuery({
    queryKey: ["logs", page, pageSize] as const,
    queryFn: ({ signal }) =>
      apiGet<LogsResponse>(`/api/log?p=${page}&page_size=${pageSize}`, signal),
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

// --- Vendor resolution ---

// Build a lookup from exact model id → vendor name, plus prefix → vendor
// for models not in the catalog but sharing a known prefix (e.g.
// "gpt-4o-mini" → "OpenAI").
const vendorByModelId = new Map<string, string>();
const vendorByPrefix: [string, string][] = [];
for (const model of catalogModels) {
  vendorByModelId.set(model.id, model.vendor);
}
// Ordered prefix table for fuzzy matching. Longer prefixes first so
// "dall-e" wins over "dall".
const knownPrefixes: [string, string][] = [
  ["gpt-", "OpenAI"],
  ["o1-", "OpenAI"],
  ["o3-", "OpenAI"],
  ["o4-", "OpenAI"],
  ["dall-e", "OpenAI"],
  ["tts-", "OpenAI"],
  ["whisper", "OpenAI"],
  ["claude-", "Anthropic"],
  ["gemini-", "Google"],
  ["deepseek-", "DeepSeek"],
  ["qwen-", "Qwen"],
];
vendorByPrefix.push(...knownPrefixes);

/** Resolve a vendor display name from a model id. Falls back to the
 *  model's first dash-segment when no known prefix matches. */
export function resolveVendor(modelId: string): string {
  const exact = vendorByModelId.get(modelId);
  if (exact) return exact;
  const lower = modelId.toLowerCase();
  for (const [prefix, vendor] of vendorByPrefix) {
    if (lower.startsWith(prefix)) return vendor;
  }
  return modelId.split("-")[0] ?? modelId;
}

// --- Adapter: LogRow -> PreviewRequest-compatible shape ---

/**
 * Adapt a backend LogRow to the PreviewRequest shape expected by the
 * request-history component. The vendor field is resolved from the
 * model catalog and a prefix table; duration_ms and status come from
 * the backend Log table.
 */
export function adaptLogRow(log: LogRow): PreviewRequestLike {
  return {
    trace_id: `log_${log.id}`,
    source: "api",
    request_id: log.request_id || undefined,
    http_status: log.http_status || undefined,
    failure_stage: log.failure_stage || undefined,
    upstream_attempts: log.request_observed ? log.upstream_attempts : undefined,
    time: new Date(log.created_at * 1000).toISOString(),
    model: log.model,
    requested_model: log.requested_model || log.model,
    actual_model: log.channel_id > 0 ? log.model : "",
    channel_id:
      Number.isSafeInteger(log.channel_id) && log.channel_id > 0
        ? String(log.channel_id)
        : undefined,
    usage_source:
      log.usage_source === "upstream" ||
      log.usage_source === "admission_estimate"
        ? log.usage_source
        : undefined,
    protocol:
      log.protocol === "OpenAI" ||
      log.protocol === "Anthropic" ||
      log.protocol === "Gemini"
        ? log.protocol
        : undefined,
    vendor: resolveVendor(log.model),
    status: log.status === 1 ? "success" : "failed",
    outcome:
      log.status === 3 ? "cancelled" : log.status === 1 ? "success" : "failed",
    cost_nano: quotaMoneyNano(log.quota_nano, log.quota),
    duration_ms: log.duration_ms,
    key_id: log.token_id ? String(log.token_id) : "",
    input_tokens: log.prompt_tokens,
    output_tokens: log.completion_tokens,
    error_code: log.error_code || "",
    error_message: log.error_message || "",
  };
}

export interface PreviewRequestLike {
  request_id?: string;
  http_status?: number;
  failure_stage?: string;
  upstream_attempts?: number;
  source?: "api";
  trace_id: string;
  time: string;
  model: string;
  requested_model: string;
  actual_model: string;
  channel_id?: string;
  usage_source?: "upstream" | "admission_estimate";
  protocol?: "OpenAI" | "Anthropic" | "Gemini";
  outcome?: "success" | "failed" | "cancelled";
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
export function useBackendRequests(pageSize: number = 20) {
  const [params] = useSearchParams();
  const rawPage = Number(params.get("page"));
  const page =
    Number.isSafeInteger(rawPage) && rawPage > 0 && rawPage <= 1_000_000_000
      ? rawPage
      : 1;
  const rawRange = params.get("range") as RequestTimeRange;
  const range = (["today", "7d", "30d", "custom", "all"] as string[]).includes(
    rawRange,
  )
    ? rawRange
    : "7d";
  const rawStatus = params.get("status") as RequestHistoryStatus;
  const status = (
    ["all", "success", "failed", "cancelled"] as string[]
  ).includes(rawStatus)
    ? rawStatus
    : "all";
  const bounds = requestTimeBounds({
    range,
    start: params.get("start") || "",
    end: params.get("end") || "",
  });
  const search = new URLSearchParams({
    p: String(page),
    page_size: String(pageSize),
    status,
  });
  if (Number.isFinite(bounds.from))
    search.set("from", String(Math.floor(bounds.from / 1000)));
  if (Number.isFinite(bounds.until))
    search.set("until", String(Math.floor(bounds.until / 1000)));
  if (params.get("query")?.trim())
    search.set("query", params.get("query")!.trim());
  const query = useQuery({
    queryKey: ["logs", "filtered", search.toString()],
    queryFn: async ({ signal }) => {
      const data = await apiGet<LogsResponse>(`/api/log?${search}`, signal);
      if (data.filtering_supported !== true)
        throw new Error("request_log_filters_unavailable");
      return data;
    },
    enabled: !bounds.error,
  });
  const { isLoading, error, refetch } = query;
  const data = bounds.error || error ? undefined : query.data;
  // Stable rows keep URL-selected detail effects from reopening on each render.
  const rows = useMemo(
    () => (data?.items ?? []).map(adaptLogRow),
    [data?.items],
  );
  return {
    rows,
    total: data?.total ?? 0,
    page,
    summary: data?.summary,
    loading: isLoading,
    error,
    refetch,
  };
}
