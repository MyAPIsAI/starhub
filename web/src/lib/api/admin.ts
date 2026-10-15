// Admin API hooks — typed wrappers around the Go backend's admin-only
// endpoints: /api/user (list + update), /api/audit, /api/metrics,
// /api/channel (list).
//
// The admin portal (pages/admin/index.tsx) uses these to replace the
// demo store with real backend data.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut } from "./http";

// --- Types: Users ---

/** Backend publicUser shape (controller/user.go publicUser). */
export interface AdminUser {
  id: number;
  username: string;
  email: string;
  role: number;
  status: number; // 1 = enabled, 2 = disabled
  quota: number; // nano-USD
  used_quota: number; // nano-USD
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
  requests: RequestCounter[];
  channel_stats: ChannelStat[];
  window: number;
  affinity: unknown[];
  generated_at: number; // unix seconds
}

// --- Types: Channels ---

export interface AdminChannel {
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

// --- Helpers ---

const STATUS_ENABLED = 1;

export function userStatusText(status: number): string {
  return status === STATUS_ENABLED ? "NORMAL" : "DISABLED";
}

export function userIsActive(status: number): boolean {
  return status === STATUS_ENABLED;
}

/** Convert nano-USD quota to a display string like "100.00". */
export function quotaToUsd(quota: number): string {
  return (quota / 1e9).toFixed(2);
}