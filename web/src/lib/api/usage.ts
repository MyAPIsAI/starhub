// Usage and logs API hooks — wrappers around the Go backend's
// /api/user/usage and /api/log endpoints.
//
// These power the console overview's metrics, charts, and recent-
// requests table with real backend data instead of the demo store.

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { apiGet } from "./http";
import { catalogModels } from "../../config/models";

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
      apiGet<UsageResponse>(`/api/user/usage?days=${days}`, signal),
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
    time: new Date(log.created_at * 1000).toISOString(),
    model: log.model,
    requested_model: log.requested_model || log.model,
    actual_model: log.model,
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
  source?: "api";
  trace_id: string;
  time: string;
  model: string;
  requested_model: string;
  actual_model: string;
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
export function useBackendRequests(pageSize: number = 100) {
  const { data, isLoading, error, refetch } = useLogs(1, pageSize);
  // Stable rows keep URL-selected detail effects from reopening on each render.
  const rows = useMemo(
    () => (data?.items ?? []).map(adaptLogRow),
    [data?.items],
  );
  return { rows, total: data?.total ?? 0, loading: isLoading, error, refetch };
}
