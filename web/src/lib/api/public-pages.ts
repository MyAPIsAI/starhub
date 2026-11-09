import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import type { CatalogStatus } from "./plaza";
export interface PublishedModel {
  id: string;
  name: string;
  vendor: string;
  tags: string[];
  protocols: string[];
}
export interface PublicHome {
  models: PublishedModel[];
  groups: { id: string; name: string; description: string }[];
}
export interface StatusDay {
  date: string;
  samples: number;
  successful: number;
  success_rate: number | null;
  state: CatalogStatus["state"];
}
export interface PublicStatusModel extends PublishedModel {
  current: CatalogStatus;
  last_24_hours: {
    samples: number;
    success_rate: number | null;
    ttft_samples: number;
    ttft_p50_ms: number | null;
    ttft_p95_ms: number | null;
  };
  daily: StatusDay[];
}
export interface PublicStatus {
  items: PublicStatusModel[];
  total: number;
  page: number;
  page_size: number;
  vendors: string[];
  types: { id: string; count: number }[];
  catalog_total: number;
  at: number;
  timezone: string;
  refresh_seconds: number;
  selected_group_count: number;
  latency_target_ms: number | null;
}
const publicOptions = { handleUnauthorizedLocally: true };
export function usePublicHome() {
  return useQuery({
    queryKey: ["public-home"],
    queryFn: ({ signal }) =>
      apiGet<PublicHome>("/api/public/home", signal, publicOptions),
    retry: false,
    staleTime: 0,
    refetchInterval: 300000,
  });
}
export function usePublicStatus(
  page: number,
  search: string,
  vendor: string,
  type = "",
  state = "",
) {
  const params = new URLSearchParams({
    page: String(page),
    page_size: "20",
    q: search,
    vendor,
    type,
    state,
  });
  return useQuery({
    queryKey: ["public-status", page, search, vendor, type, state],
    queryFn: ({ signal }) =>
      apiGet<PublicStatus>(
        `/api/status/models?${params}`,
        signal,
        publicOptions,
      ),
    retry: false,
    staleTime: 0,
    refetchInterval: 300000,
  });
}

export interface StatusHour {
  start: number;
  end: number;
  samples: number;
  successful: number;
  success_rate: number | null;
}
export interface HourlyStatus {
  model: string;
  timezone: string;
  at: number;
  history: Omit<StatusHour, "successful"> & { hours: StatusHour[] };
}
export function usePublicStatusHours(
  model: string,
  date: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ["public-status-hours", model, date],
    enabled,
    queryFn: ({ signal }) =>
      apiGet<HourlyStatus>(
        `/api/status/models/${encodeURIComponent(model)}/history?${new URLSearchParams({ date })}`,
        signal,
        publicOptions,
      ),
    retry: false,
    staleTime: 0,
    refetchInterval: 300000,
  });
}
