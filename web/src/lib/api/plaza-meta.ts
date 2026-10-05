// Plaza metadata API hooks — calls the Go backend /api/plaza/meta
// endpoints (AdminAuth required). These manage the model display
// metadata + official reference prices, not the platform billing
// prices (which live in /api/option).

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiDelete } from "./http";
import type { ModelItem } from "../../config/models";

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

export function usePlazaModelMeta() {
  return useQuery({
    queryKey: META_KEY,
    queryFn: ({ signal }) => apiGet<ModelMeta[]>("/api/plaza/meta", signal),
  });
}

export function useUpsertModelMeta() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (meta: ModelMeta) => apiPut<ModelMeta>("/api/plaza/meta", meta),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: META_KEY });
      queryClient.invalidateQueries({ queryKey: ["plaza-models"] });
      queryClient.invalidateQueries({ queryKey: ["plaza-model"] });
      queryClient.invalidateQueries({ queryKey: ["plaza-filters"] });
    },
  });
}

export function useDeleteModelMeta() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (model: string) =>
      apiDelete<{ model: string }>(
        `/api/plaza/meta/${encodeURIComponent(model)}`,
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: META_KEY });
      queryClient.invalidateQueries({ queryKey: ["plaza-models"] });
      queryClient.invalidateQueries({ queryKey: ["plaza-model"] });
      queryClient.invalidateQueries({ queryKey: ["plaza-filters"] });
    },
  });
}

// --- Adapter: ModelMeta <-> ModelItem ---

const KNOWN_TAGS = ["text", "code", "vision", "reasoning", "image", "video"];

function formatContextDisplay(length: number): string {
  if (length <= 0) return "—";
  if (length >= 1_000_000) return `${(length / 1_000_000).toFixed(0)}M`;
  if (length >= 1000) return `${Math.round(length / 1000)}K`;
  return String(length);
}

/** Convert a backend ModelMeta to the frontend ModelItem shape. */
export function metaToModelItem(meta: ModelMeta): ModelItem {
  const tags = meta.tags
    ? meta.tags
        .split(",")
        .map((t) => t.trim())
        .filter((t): t is ModelItem["capabilities"][number] =>
          KNOWN_TAGS.includes(t),
        )
    : [];
  return {
    id: meta.model,
    name: meta.vendor_display || meta.model,
    vendor: meta.vendor || meta.vendor_display || meta.model,
    capabilities: tags.length ? tags : ["text"],
    contextLength: meta.context_length,
    contextDisplay: formatContextDisplay(meta.context_length),
    pricing: {
      input_per_million_usd: meta.official_input_per_m?.toFixed(6) ?? "0",
      output_per_million_usd: meta.official_output_per_m?.toFixed(6) ?? "0",
      ...(meta.official_cache_read_per_m != null
        ? {
            cache_read_per_million_usd:
              meta.official_cache_read_per_m.toFixed(6),
          }
        : {}),
      ...(meta.official_cache_write_per_m != null
        ? {
            cache_write_per_million_usd:
              meta.official_cache_write_per_m.toFixed(6),
          }
        : {}),
    },
    protocols: [],
    description: {
      en: meta.description || "",
      zh: meta.description || "",
      ar: meta.description || "",
    },
    tierAvailability: {
      ultra: true,
      pro: true,
      standard: true,
      lite: true,
    },
    online: true,
  };
}

/** Convert a frontend ModelItem to a backend ModelMeta for upsert. */
export function modelItemToMeta(model: ModelItem): ModelMeta {
  const toFloat = (s: string | undefined): number | null => {
    if (!s) return null;
    const n = Number(s);
    return Number.isFinite(n) && n >= 0 ? n : null;
  };
  return {
    model: model.id,
    vendor: model.vendor,
    vendor_display: model.name,
    description: model.description.en || model.description.zh || "",
    tags: model.capabilities.join(","),
    context_length: model.contextLength,
    max_output: 0,
    official_input_per_m: toFloat(model.pricing.input_per_million_usd),
    official_output_per_m: toFloat(model.pricing.output_per_million_usd),
    official_cache_read_per_m: toFloat(
      model.pricing.cache_read_per_million_usd,
    ),
    official_cache_write_per_m: toFloat(
      model.pricing.cache_write_per_million_usd,
    ),
    official_per_request: null,
    per_request_usd: null,
    score: null,
    compatible_groups: "",
    sort_weight: 0,
    created_at: 0,
  };
}
