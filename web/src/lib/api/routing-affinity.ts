import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiDelete } from "./http";

export interface RoutingAffinityStats {
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
