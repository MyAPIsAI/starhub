import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut } from "./http";
import { useSessionQueryIdentity } from "../system-access";
import {
  readServerPricing,
  readPricingAcknowledgement,
  type PricingOption,
} from "../server-pricing";
export function useServerPricing(enabled: boolean) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["server-pricing", identity],
    enabled,
    retry: false,
    queryFn: async ({ signal }) =>
      readServerPricing(await apiGet<unknown>("/api/option/pricing", signal)),
  });
}
export function useSaveServerPricing() {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (input: {
      expected_version: number;
      options: PricingOption[];
    }) =>
      readPricingAcknowledgement(
        await apiPut<unknown>("/api/option/pricing", input),
      ),
    onSettled: () => {
      for (const key of [
        "server-pricing",
        "options",
        "option-status",
        "option-history",
        "plaza-models",
        "plaza-model",
        "plaza-filters",
        "plaza-rank",
        "service-group-configuration",
      ])
        void client.invalidateQueries({ queryKey: [key] });
    },
  });
}
