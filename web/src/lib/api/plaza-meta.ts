// Plaza metadata API hooks — calls the Go backend /api/plaza/meta
// endpoints (AdminAuth required). These manage the model display
// metadata + official reference prices, not the platform billing
// prices (which live in /api/option).

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiRequest } from "./http";
import { useSessionQueryIdentity } from "../system-access";
import {
  readModelMetadataCatalog,
  readMetadataSave,
  readMetadataDelete,
} from "../model-metadata";

// --- Backend response type (mirrors model/modelmeta.go:27-73) ---

export interface ModelMeta {
  model: string;
  vendor: string;
  vendor_display: string;
  description: string;
  tags: string;
  context_length: number;
  max_output: number;
  official_input_per_m: number | null;
  official_output_per_m: number | null;
  official_cache_read_per_m: number | null;
  official_cache_write_per_m: number | null;
  official_per_request: number | null;
  per_request_usd: number | null;
  score: number | null;
  compatible_groups: string;
  sort_weight: number;
  created_at: number;
}

const META_KEY = ["plaza-meta"] as const;

export function usePlazaModelMeta(enabled = true) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: [...META_KEY, identity],
    enabled,
    retry: false,
    queryFn: async ({ signal }) =>
      readModelMetadataCatalog(
        await apiGet<unknown>("/api/plaza/meta/configuration", signal),
      ),
  });
}

export function useUpsertModelMeta() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      meta: ModelMeta;
      expected_catalog_revision: number;
    }) =>
      readMetadataSave(
        await apiPut<unknown>("/api/plaza/meta", {
          ...input.meta,
          expected_catalog_revision: input.expected_catalog_revision,
        }),
      ),
    retry: false,
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: META_KEY });
      for (const key of [
        "plaza-models",
        "plaza-model",
        "plaza-filters",
        "plaza-rank",
      ])
        void queryClient.invalidateQueries({ queryKey: [key] });
    },
  });
}

export function useDeleteModelMeta() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      model: string;
      expected_catalog_revision: number;
    }) =>
      readMetadataDelete(
        await apiRequest<unknown>(
          `/api/plaza/meta/${encodeURIComponent(input.model)}`,
          {
            method: "DELETE",
            body: JSON.stringify({
              expected_catalog_revision: input.expected_catalog_revision,
            }),
          },
        ),
      ),
    retry: false,
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: META_KEY });
      for (const key of [
        "plaza-models",
        "plaza-model",
        "plaza-filters",
        "plaza-rank",
      ])
        void queryClient.invalidateQueries({ queryKey: [key] });
    },
  });
}
