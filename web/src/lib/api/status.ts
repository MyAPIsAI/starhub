import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";

export type PublicServiceState =
  "operational" | "degraded" | "unavailable" | "unknown";
export interface PublicStatusMetric {
  total_requests: number;
  success_count: number;
  failed_count: number;
  success_rate: number | null;
  avg_duration_ms: number | null;
}
export interface PublicStatusModel {
  id: string;
  vendor: string;
  vendor_display: string;
  available: boolean;
  state: PublicServiceState;
  recent: PublicStatusMetric;
  history: PublicStatusMetric;
  days: Array<PublicStatusMetric & { date: string }>;
}
export interface PublicStatusOverview {
  generated_at: number;
  window_start: number;
  window_end: number;
  history_start: string;
  timezone: "UTC";
  state: PublicServiceState;
  models: PublicStatusModel[];
}
export function usePublicStatus() {
  return useQuery({
    queryKey: ["public-status-overview"],
    retry: false,
    refetchInterval: 60000,
    queryFn: async ({ signal }) => {
      const view = await apiGet<PublicStatusOverview>(
        "/api/status/overview",
        signal,
        { handleUnauthorizedLocally: true },
      );
      if (
        !view ||
        !Array.isArray(view.models) ||
        view.timezone !== "UTC" ||
        !Number.isSafeInteger(view.generated_at)
      )
        throw new Error("Invalid public status response");
      return view;
    },
  });
}
