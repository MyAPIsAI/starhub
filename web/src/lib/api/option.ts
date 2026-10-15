// Option API hooks — wrappers around the Go backend's /api/option
// endpoints (admin only).
//
// The backend stores configuration as key-value string pairs. The
// most relevant keys for pricing persistence are:
//   - "model_ratio": JSON map of model → price ratio (USD per 1M tokens)
//   - "completion_ratio": JSON map of model → output/input ratio
//
// These hooks let the frontend persist model pricing ratios to the
// server so they survive across sessions and instances.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiPost } from "./http";

export interface Option {
  key: string;
  value: string;
}

const OPTIONS_KEY = ["options"] as const;
const OPTION_STATUS_KEY = ["option-status"] as const;
const OPTION_HISTORY_KEY = ["option-history"] as const;

export function useOptions() {
  return useQuery({
    queryKey: OPTIONS_KEY,
    queryFn: ({ signal }) => apiGet<Option[]>("/api/option", signal),
  });
}

export function useUpdateOption() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      key: string;
      value: string;
      expected_version?: number;
    }) => apiPut("/api/option", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: OPTIONS_KEY });
      queryClient.invalidateQueries({ queryKey: OPTION_STATUS_KEY });
      queryClient.invalidateQueries({ queryKey: OPTION_HISTORY_KEY });
    },
  });
}

// --- Option status (GET /api/option/status) ---

export interface OptionStatus {
  database_version: number;
  loaded_version: number;
  in_sync: boolean;
  instances: { id: string; version: number; last_seen: number }[];
}

export function useOptionStatus() {
  return useQuery({
    queryKey: OPTION_STATUS_KEY,
    queryFn: ({ signal }) =>
      apiGet<OptionStatus>("/api/option/status", signal),
  });
}

// --- Option history (GET /api/option/history) ---

export interface OptionChange {
  id: number;
  version: number;
  key: string;
  old_value: string;
  new_value: string;
  actor_id: number;
  created_at: number; // unix seconds
}

export function useOptionHistory(limit = 50) {
  return useQuery({
    queryKey: [...OPTION_HISTORY_KEY, limit],
    queryFn: ({ signal }) =>
      apiGet<OptionChange[]>(`/api/option/history?limit=${limit}`, signal),
  });
}

// --- Option rollback (POST /api/option/rollback) ---

export function useRollbackOption() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { version: number; expected_version: number }) =>
      apiPost<{ version: number }>("/api/option/rollback", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: OPTIONS_KEY });
      queryClient.invalidateQueries({ queryKey: OPTION_STATUS_KEY });
      queryClient.invalidateQueries({ queryKey: OPTION_HISTORY_KEY });
    },
  });
}

/**
 * Build the model_ratio JSON value from the frontend's pricing config.
 * Each model's input_per_million_usd becomes its ratio (a float64).
 */
export function buildModelRatioJson(
  models: Record<string, { pricing: { input_per_million_usd: string } }>,
): string {
  const ratioMap: Record<string, number> = {};
  for (const [id, entry] of Object.entries(models)) {
    const price = parseFloat(entry.pricing.input_per_million_usd);
    if (Number.isFinite(price) && price > 0) {
      ratioMap[id] = price;
    }
  }
  return JSON.stringify(ratioMap);
}

/**
 * Build the completion_ratio JSON value from the frontend's pricing
 * config. Each model's output/input ratio is stored as a float64.
 */
export function buildCompletionRatioJson(
  models: Record<string, {
    pricing: {
      input_per_million_usd: string;
      output_per_million_usd: string;
    };
  }>,
): string {
  const ratioMap: Record<string, number> = {};
  for (const [id, entry] of Object.entries(models)) {
    const input = parseFloat(entry.pricing.input_per_million_usd);
    const output = parseFloat(entry.pricing.output_per_million_usd);
    if (Number.isFinite(input) && input > 0 && Number.isFinite(output)) {
      ratioMap[id] = output / input;
    }
  }
  return JSON.stringify(ratioMap);
}
