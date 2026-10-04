// Plaza API hooks — calls the Go backend /api/plaza/* endpoints.
//
// The backend returns plazaRow objects with snake_case JSON keys and
// a nested pricing object. The adapter (plazaToDirectoryModel)
// converts these to the frontend DirectoryModel shape used by the
// catalog pages.

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import type { DirectoryModel } from "../../config/model-directory";
import type { TierId } from "../../config/placeholders";

// --- Backend response types (mirror controller/plaza_pricing.go) ---

export interface PlazaPricing {
  mode: string;
  input_per_m: number | null;
  output_per_m: number | null;
  cache_read_per_m: number | null;
  cache_write_per_m: number | null;
  per_request: number | null;
  official_input_per_m: number | null;
  official_output_per_m: number | null;
  official_cache_read_per_m: number | null;
  official_cache_write_per_m: number | null;
  official_per_request: number | null;
  group_ratio: number;
  explicit: boolean;
  currency: string;
}

export interface PlazaRow {
  id: string;
  name: string;
  vendor: string;
  vendor_display: string;
  description: string;
  tags: string[];
  context_length: number;
  max_output: number;
  pricing: PlazaPricing;
  available_groups: string[];
  is_available: boolean;
  unavailable_reason: string | null;
  score: number | null;
  calls_7d: number | null;
  channel_count: number;
  created_at: number;
  sort_weight: number;
}

export interface PlazaListResponse {
  total: number;
  page: number;
  page_size: number;
  group: string;
  items: PlazaRow[];
}

export interface PlazaVendorFacet {
  value: string;
  label: string;
  count: number;
}

export interface PlazaPriceBucket {
  bucket: string;
  from: number;
  to: number;
  count: number;
}

export interface PlazaFiltersResponse {
  vendors: PlazaVendorFacet[];
  tags: PlazaVendorFacet[];
  price_histogram: PlazaPriceBucket[];
  total: number;
  group: string;
}

export interface PlazaDetailResponse {
  model: PlazaRow;
  related: {
    same_vendor: PlazaRow[];
    same_price: PlazaRow[];
  };
  max_output: number;
  meta: {
    score: number | null;
    sort_weight: number;
    created_at: number;
    per_request: number | null;
  } | null;
}

// --- Query params ---

export interface PlazaQuery {
  q?: string;
  vendors?: string;
  tags?: string;
  context_min?: number;
  price_max?: number;
  modes?: string;
  available_only?: number;
  discounted_only?: number;
  sort?: string;
  page?: number;
  page_size?: number;
}

function buildQueryString(query: PlazaQuery): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== null && value !== "") {
      params.set(key, String(value));
    }
  }
  const s = params.toString();
  return s ? `?${s}` : "";
}

// --- Hooks ---

export function usePlazaModels(query: PlazaQuery = {}) {
  return useQuery({
    queryKey: ["plaza-models", query],
    queryFn: ({ signal }) =>
      apiGet<PlazaListResponse>(`/api/plaza/models${buildQueryString(query)}`, signal),
  });
}

export function usePlazaModel(id: string | undefined) {
  return useQuery({
    queryKey: ["plaza-model", id],
    enabled: Boolean(id),
    queryFn: ({ signal }) =>
      apiGet<PlazaDetailResponse>(
        `/api/plaza/models/${encodeURIComponent(id!)}`,
        signal,
      ),
  });
}

export function usePlazaFilters() {
  return useQuery({
    queryKey: ["plaza-filters"],
    queryFn: ({ signal }) =>
      apiGet<PlazaFiltersResponse>("/api/plaza/filters", signal),
  });
}

// --- Adapter: PlazaRow -> DirectoryModel ---

function formatContextDisplay(length: number): string {
  if (length <= 0) return "";
  if (length >= 1_000_000) return `${(length / 1_000_000).toFixed(0)}M`;
  if (length >= 1000) return `${Math.round(length / 1000)}K`;
  return String(length);
}

const TIER_IDS: TierId[] = ["ultra", "pro", "standard", "lite"];

/** Convert a backend PlazaRow to the frontend DirectoryModel shape. */
export function plazaToDirectoryModel(row: PlazaRow): DirectoryModel {
  const tags = row.tags ?? [];
  const capabilities = tags.filter(
    (t): t is DirectoryModel["capabilities"][number] =>
      ["text", "code", "vision", "reasoning", "image", "video"].includes(t),
  ) as DirectoryModel["capabilities"];

  // Derive tierAvailability from available_groups: any group means
  // available at all tiers (the backend doesn't expose per-tier yet).
  const tierAvailability = TIER_IDS.reduce(
    (acc, tier) => {
      acc[tier] = row.is_available;
      return acc;
    },
    {} as Record<TierId, boolean>,
  );

  return {
    id: row.id,
    name: row.name,
    vendor: row.vendor_display || row.vendor,
    kind: "specified",
    capabilities: capabilities.length ? capabilities : ["text"],
    contextLength: row.context_length,
    contextDisplay: formatContextDisplay(row.context_length),
    protocols: [],
    description: {
      en: row.description || "",
      zh: row.description || "",
      ar: row.description || "",
    },
    tierAvailability,
    online: row.is_available,
  } as DirectoryModel;
}