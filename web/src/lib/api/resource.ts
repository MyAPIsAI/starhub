import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiRequest } from "./http";
import type { ResourcePlacement, ResourceSnapshot } from "../../config/models";
import { parseResourceCatalog } from "../resource-catalog";
import {
  ROLE_ADMIN,
  ROLE_ROOT,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

export function useResourceCatalog() {
  const identity = useSessionQueryIdentity();
  const account = useSystemAccess((s) => s.session.accountId);
  const role = useSystemAccess((s) => s.role);
  return useQuery({
    queryKey: ["resource-catalog", identity],
    enabled: account !== null && (role === ROLE_ADMIN || role === ROLE_ROOT),
    retry: false,
    queryFn: async ({ signal }) =>
      parseResourceCatalog(await apiGet<unknown>("/api/resource", signal)),
  });
}

export function useResourceMutation() {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (
      input: { expected_revision: number } & (
        | { snapshot: ResourceSnapshot }
        | { id: string; placement: ResourcePlacement }
      ),
    ) =>
      parseResourceCatalog(
        await apiPut<unknown>(
          "snapshot" in input
            ? "/api/resource"
            : `/api/placement/${encodeURIComponent(input.id)}`,
          "snapshot" in input
            ? input
            : {
                expected_revision: input.expected_revision,
                placement: input.placement,
              },
        ),
      ),
    onSettled: () => {
      for (const key of [
        "resource-catalog",
        "resource-publication-context",
        "admin-audit",
      ])
        void client.invalidateQueries({ queryKey: [key] });
    },
  });
}

export function useRegionMutation() {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (
      input: { id: string; expected_revision: number } & (
        | { operation: "save"; name: string; description: string }
        | { operation: "delete" }
      ),
    ) =>
      parseResourceCatalog(
        await apiRequest<unknown>(
          `/api/region/${encodeURIComponent(input.id)}`,
          {
            method: input.operation === "delete" ? "DELETE" : "PUT",
            body: JSON.stringify(
              input.operation === "delete"
                ? { expected_revision: input.expected_revision }
                : {
                    expected_revision: input.expected_revision,
                    name: input.name,
                    description: input.description,
                  },
            ),
          },
        ),
      ),
    onSettled: () => {
      for (const key of [
        "resource-catalog",
        "resource-publication-context",
        "admin-audit",
      ])
        void client.invalidateQueries({ queryKey: [key] });
    },
  });
}
