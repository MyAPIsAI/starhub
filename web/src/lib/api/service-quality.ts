// Service quality API hooks — wrappers around the Go backend's
// /api/service-quality endpoint.
//
// The backend aggregates the Log table into per-model quality metrics
// (success rate, average latency, error rate, request count, quota).

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import { useSessionQueryIdentity, useSystemAccess } from "../system-access";

export interface ServiceQualityMetric {
  model: string;
  total_requests: number;
  success_count: number;
  failed_count: number;
  success_rate: number; // 0.0–1.0
  avg_duration_ms: number;
  p95_duration_ms: number;
  total_quota: number;
  total_quota_nano?: string;
  error_rate: number; // 0.0–1.0
}

export interface ServiceQualityDailyMetric {
  date: string; // "2006-01-02"
  total_requests: number;
  success_count: number;
  failed_count: number;
  success_rate: number; // 0.0–1.0
  avg_duration_ms: number;
  p95_duration_ms: number; // 0 when no positive duration samples exist.
}

export interface ServiceQualitySummary {
  window_hours: number;
  window_start: number;
  window_end: number; // Exclusive Unix timestamp.
  overall: ServiceQualityMetric;
  by_model: ServiceQualityMetric[];
  by_day: ServiceQualityDailyMetric[];
}

export function useServiceQuality(
  hours: number = 24,
  model?: string,
  userId?: number,
  options: { month?: string; enabled?: boolean } = {},
) {
  const identity = useSessionQueryIdentity();
  const authenticated = useSystemAccess(
    (state) => state.session.accountId !== null,
  );
  const params = new URLSearchParams(
    options.month === undefined
      ? { hours: String(hours) }
      : { month: options.month },
  );
  if (model) params.set("model", model);
  if (userId !== undefined) params.set("user_id", String(userId));
  const period = options.month === undefined ? hours : { month: options.month };
  const baseKey = model
    ? (["service-quality", identity, period, model] as const)
    : (["service-quality", identity, period] as const);
  return useQuery({
    queryKey: userId === undefined ? baseKey : [...baseKey, { userId }],
    enabled: authenticated && options.enabled !== false,
    queryFn: async ({ signal }) => {
      const data = await apiGet<ServiceQualitySummary>(
        `/api/service-quality?${params}`,
        signal,
      );
      if (options.month !== undefined) {
        const start = new Date(`${options.month}-01T00:00:00Z`);
        const end = new Date(start);
        end.setUTCMonth(end.getUTCMonth() + 1);
        // Reject servers that ignore month and return a rolling-hour window.
        if (
          data.window_start !== start.getTime() / 1000 ||
          !Number.isSafeInteger(data.window_end) ||
          data.window_end <= data.window_start ||
          data.window_end > end.getTime() / 1000
        ) {
          throw new Error("Invalid monthly service quality window");
        }
      }
      return data;
    },
  });
}
