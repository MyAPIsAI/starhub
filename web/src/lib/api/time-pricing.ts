import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut } from "./http";
import { useSessionQueryIdentity } from "../system-access";
import {
  readServerTimePricing,
  type ServerTimeSchedule,
} from "../server-time-pricing";
import { readPricingAcknowledgement } from "../server-pricing";

export function useServerTimePricing(enabled: boolean) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["time-pricing", identity],
    enabled,
    retry: false,
    queryFn: async ({ signal }) =>
      readServerTimePricing(
        await apiGet<unknown>("/api/option/time-pricing", signal),
      ),
  });
}
export function useSaveServerTimePricing() {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (input: {
      expected_version: number;
      rules: Record<string, ServerTimeSchedule>;
    }) => {
      const response = await apiPut<unknown>("/api/option/time-pricing", input);
      if (
        !response ||
        typeof response !== "object" ||
        (response as Record<string, unknown>).schema !==
          "weekly_multiplier_v1" ||
        (response as Record<string, unknown>).billing_active !== true
      )
        throw Error("Unsupported time-pricing acknowledgement");
      return readPricingAcknowledgement(response);
    },
    onSettled: () => {
      for (const key of [
        "time-pricing",
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
