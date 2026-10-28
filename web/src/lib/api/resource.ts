import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut } from "./http";
import type { ResourcePlacement, ResourceSnapshot } from "../../config/models";
import {
  parseResourceCatalog,
  type ResourceCatalog,
} from "../resource-catalog";
import { useProductPreview } from "../product-preview";
import { ROLE_ADMIN, ROLE_ROOT, useSystemAccess } from "../system-access";

export function useResourceCatalog() {
  const account = useProductPreview((s) => s.user.id);
  const role = useSystemAccess((s) => s.role);
  return useQuery({
    queryKey: ["resource-catalog", account],
    enabled: role === ROLE_ADMIN || role === ROLE_ROOT,
    queryFn: async ({ signal }) =>
      parseResourceCatalog(await apiGet<unknown>("/api/resource", signal)),
  });
}

export function useResourceMutation() {
  const client = useQueryClient();
  const account = useProductPreview((s) => s.user.id);
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
    onSuccess: (data: ResourceCatalog) => {
      client.setQueryData(["resource-catalog", account], data);
      client.invalidateQueries({ queryKey: ["admin-audit"] });
    },
    onError: () => {
      void client.invalidateQueries({
        queryKey: ["resource-catalog", account],
      });
    },
  });
}
