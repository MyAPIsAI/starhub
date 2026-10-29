import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut } from "./http";
import {
  isAdmin,
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";
import {
  readServerTokenTimePricing,
  readTokenTimeAcknowledgement,
  type TokenTimeSchedule,
} from "../server-time-token-pricing";

function assertScope(scope: ReturnType<typeof sessionQueryIdentity>) {
  const current = useSystemAccess.getState();
  if (
    scope[1] === null ||
    !isAdmin(current.role) ||
    JSON.stringify(scope) !== JSON.stringify(sessionQueryIdentity(current))
  )
    throw Error(
      "A confirmed unchanged administrative session is required for independent prices",
    );
}

export function useServerTokenTimePricing(enabled: boolean) {
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["time-token-pricing", scope],
    enabled: enabled && scope[1] !== null && isAdmin(scope[3]),
    retry: false,
    queryFn: async ({ signal }) => {
      assertScope(scope);
      const response = await apiGet<unknown>(
        "/api/option/time-token-pricing",
        signal,
      );
      assertScope(scope);
      return readServerTokenTimePricing(response);
    },
  });
}

export function useSaveServerTokenTimePricing() {
  const scope = useSessionQueryIdentity(),
    client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (input: {
      expected_version: number;
      rules: Record<string, TokenTimeSchedule>;
    }) => {
      assertScope(scope);
      if (
        !Number.isSafeInteger(input.expected_version) ||
        input.expected_version < 0 ||
        input.expected_version >= Number.MAX_SAFE_INTEGER
      )
        throw Error("Invalid independent token pricing revision");
      const response = await apiPut<unknown>(
        "/api/option/time-token-pricing",
        input,
      );
      assertScope(scope);
      const result = readTokenTimeAcknowledgement(response);
      if (
        result.version < input.expected_version ||
        result.version > input.expected_version + 1
      )
        throw Error("Unexpected independent token pricing revision");
      return result;
    },
    onSettled: () => {
      for (const key of [
        "time-token-pricing",
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
        "resource-publication-context",
        "resource-publication-state",
      ])
        void client.invalidateQueries({ queryKey: [key] });
    },
  });
}
