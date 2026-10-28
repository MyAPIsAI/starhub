// Service quality API hooks — wrappers around the Go backend's
// /api/service-quality endpoint.
//
// The backend aggregates the Log table into per-model quality metrics
// (success rate, average latency, error rate, request count, quota).

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";

export interface ServiceQualityMetric {
  model: string;
  total_requests: number;
  success_count: number;
  failed_count: number;
  success_rate: number; // 0.0–1.0
  avg_duration_ms: number;
  p95_duration_ms: number;
  total_quota: number;
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
  overall: ServiceQualityMetric;
  by_model: ServiceQualityMetric[];
  by_day: ServiceQualityDailyMetric[];
}

export function useServiceQuality(hours: number = 24) {
  return useQuery({
    queryKey: ["service-quality", hours] as const,
    queryFn: ({ signal }) =>
      apiGet<ServiceQualitySummary>(
        `/api/service-quality?hours=${hours}`,
        signal,
      ),
  });
}
