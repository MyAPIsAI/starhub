import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiDelete } from "./http";
import type { Option, OptionStatus } from "./option";
import { useSessionQueryIdentity } from "../system-access";

export async function readAffinityDraft(signal: AbortSignal) {
  // Capture the precondition before reading values so a concurrent write can
  // only cause a conflict, never authorize overwriting a newer configuration.
  const { database_version: version } = await apiGet<OptionStatus>(
    "/api/option/status",
    signal,
  );
  if (!Number.isSafeInteger(version) || version < 0)
    throw Error("Invalid option version");
  const rows = await apiGet<Option[]>("/api/option", signal);
  return {
    version,
    values: Object.fromEntries(
      rows
        .filter((row) => row.key.startsWith("routing_affinity_"))
        .map((row) => [row.key.slice("routing_affinity_".length), row.value]),
    ),
  };
}

export interface RoutingAffinityStats {
  instance?: string;
  observed_at?: number;
  events?: Record<string, number>;
  config?: {
    enabled: boolean;
    mode: "sticky" | "hash";
    ttl_seconds: number;
    cooldown_seconds: number;
    prefix_cache: boolean;
  };
  enabled: boolean;
  total: number;
  by_rule_name: Record<string, number>;
  rules: number;
}

export interface RoutingAffinityCacheCleared {
  removed: number;
}

/** useRoutingAffinityStats polls the live rule-affinity cache for the
 *  persistent gateway bindings panel. Process-local; reflects this replica. */
export function useRoutingAffinityStats(enabled: boolean) {
  return useQuery({
    queryKey: ["routing-affinity-cache"],
    enabled,
    refetchInterval: enabled ? 15000 : false,
    queryFn: ({ signal }) =>
      apiGet<RoutingAffinityStats>(
        "/api/option/routing_affinity_cache",
        signal,
      ),
  });
}

export function useClearRoutingAffinityCache() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiDelete<RoutingAffinityCacheCleared>(
        "/api/option/routing_affinity_cache",
      ),
    onSettled: () =>
      client.invalidateQueries({ queryKey: ["routing-affinity-cache"] }),
  });
}

export function useAffinityDetails(
  instance: string,
  rule: string,
  page: number,
) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: [
      "routing-affinity-cache",
      "details",
      identity,
      instance,
      rule,
      page,
    ],
    retry: false,
    gcTime: 0,
    refetchInterval: 15000,
    queryFn: ({ signal }) =>
      apiGet<{
        instance: string;
        observed_at: number;
        total: number;
        entries: { channel_id: string; expires_at: number }[];
      }>(
        `/api/option/routing_affinity_cache?${new URLSearchParams({ rule, page: String(page) })}`,
        signal,
      ),
  });
}
