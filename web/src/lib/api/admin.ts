import { quotaMoneyNano, formatMoney } from "../money";
// Admin API hooks — typed wrappers around the Go backend's admin-only
// endpoints: /api/user (list + update), /api/audit, /api/metrics,
// /api/channel (list + test).
//
// The admin portal (pages/admin/index.tsx) uses these to replace the
// demo store with real backend data.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiPost } from "./http";

// --- Types: Users ---

/** Backend publicUser shape (controller/user.go publicUser). */
export interface AdminUser {
  id: number;
  username: string;
  email: string;
  role: number;
  status: number; // 1 = enabled, 2 = disabled
  quota_nano?: string;
  used_quota_nano?: string;
  quota: number; // Legacy quota ticks.
  used_quota: number; // Legacy quota ticks.
  group: string;
  aff_code: string;
  inviter_id: number;
  created_at: number; // unix seconds
}

export interface UpdateUserInput {
  id: number;
  quota?: number;
  expected_quota?: number;
  status?: number;
  group?: string;
  password?: string;
}

// --- Types: Audit ---

export interface AuditLogEntry {
  id: number;
  user_id: number;
  username: string;
  method: string;
  path: string;
  status: number;
  ip: string;
  created_at: number; // unix seconds
}

export interface AuditLogPage {
  items: AuditLogEntry[];
  total: number;
}

// --- Types: Metrics ---

export interface RequestCounter {
  class: string;
  count: number;
}

export interface ChannelStat {
  channel_id: number;
  samples: number;
  success_rate: number;
  avg_ttft_ms: number;
  p95_ttft_ms: number;
  avg_latency_ms: number;
  last_ttft_ms: number;
  last_attempt_at: number; // unix seconds
}

export interface MetricsSnapshot {
  requests: RequestCounter[] | null;
  channel_stats: ChannelStat[] | null;
  window: number;
  affinity: unknown[];
  generated_at: number; // unix seconds
}

// --- Types: Channels ---

export interface AdminChannel {
  task_plugin_id?: number;
  id: number;
  name: string;
  type: number;
  key: string; // masked
  key_mode: string;
  key_count: number;
  multi_key_status: Record<string, number> | null;
  base_url: string;
  models: string;
  group: string;
  status: number;
  priority: number;
  weight: number;
  response_time: number;
  used_quota: number;
  created_at: number; // unix seconds
}

/** Backend channelTestResult (controller/channel.go:232). */
export interface ChannelTestResult {
  ok: boolean;
  model: string;
  elapsed_ms: number;
  message: string;
  status: number;
}

/** Per-key upstream usage from /api/channel/{id}/upstream-usage. */
export interface UpstreamKeyUsage {
  total_used_nano?: string;
  total_available_nano?: string;
  total_granted_nano?: string;
  key_index: number;
  key_masked: string;
  available: boolean;
  error?: string;
  name?: string;
  total_used: number;
  total_available: number;
  total_granted: number;
  unlimited: boolean;
  expires_at: number;
  model_limits?: unknown;
  user_quota?: {
    total_available: number;
    total_used: number;
  };
}

/** Aggregated upstream usage result. */
export interface UpstreamUsageResult {
  channel_id: number;
  channel_name: string;
  local_used_quota: number;
  local_used_quota_nano?: string;
  upstream_total_used_nano?: string;
  diff_nano?: string;
  keys: UpstreamKeyUsage[];
  upstream_total_used: number;
  diff: number;
  supported: boolean;
}

// --- Query keys ---

const ADMIN_USERS_KEY = ["admin-users"] as const;
const ADMIN_AUDIT_KEY = ["admin-audit"] as const;
const ADMIN_METRICS_KEY = ["admin-metrics"] as const;
const ADMIN_CHANNELS_KEY = ["admin-channels"] as const;

// --- Hooks: Users ---

export function useAdminUsers() {
  return useQuery({
    queryKey: ADMIN_USERS_KEY,
    queryFn: ({ signal }) => apiGet<AdminUser[]>("/api/user", signal),
  });
}

export function useUpdateUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateUserInput) => apiPut<null>("/api/user", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ADMIN_USERS_KEY });
    },
  });
}

// --- Hooks: Audit ---

export function useAdminAudit(page = 1, pageSize = 20) {
  return useQuery({
    queryKey: [...ADMIN_AUDIT_KEY, page, pageSize],
    queryFn: ({ signal }) =>
      apiGet<AuditLogPage>(
        `/api/audit?p=${page}&page_size=${pageSize}`,
        signal,
      ),
  });
}

// --- Hooks: Metrics ---

export function useAdminMetrics() {
  return useQuery({
    queryKey: ADMIN_METRICS_KEY,
    queryFn: ({ signal }) => apiGet<MetricsSnapshot>("/api/metrics", signal),
    refetchInterval: 15000,
  });
}

// --- Hooks: Channels ---

export function useAdminChannels() {
  return useQuery({
    queryKey: ADMIN_CHANNELS_KEY,
    queryFn: ({ signal }) => apiGet<AdminChannel[]>("/api/channel", signal),
  });
}

export function useTestChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, model }: { id: number; model?: string }) =>
      apiPost<ChannelTestResult>(
        `/api/channel/test/${id}${model ? `?model=${encodeURIComponent(model)}` : ""}`,
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ADMIN_CHANNELS_KEY });
      queryClient.invalidateQueries({ queryKey: ADMIN_METRICS_KEY });
    },
  });
}

export function useUpstreamUsage() {
  return useMutation({
    mutationFn: (id: number) =>
      apiGet<UpstreamUsageResult>(`/api/channel/${id}/upstream-usage`),
  });
}

// --- Helpers ---

const STATUS_ENABLED = 1;

export function userStatusText(status: number): string {
  return status === STATUS_ENABLED ? "NORMAL" : "DISABLED";
}

export function userIsActive(status: number): boolean {
  return status === STATUS_ENABLED;
}

/** Preserve exact server money and support legacy quota projections. */
export function quotaToUsd(quota: number, exact?: string): string {
  return formatMoney(quotaMoneyNano(exact, quota), "detail").replace("$", "");
}
