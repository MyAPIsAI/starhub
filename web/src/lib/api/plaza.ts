// Plaza API hooks — calls the Go backend /api/plaza/* endpoints.
//
// The backend returns plazaRow objects with snake_case JSON keys and
// a nested pricing object. Catalog pages keep those prices and group IDs
// separate from the local console demo's pricing configuration.

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import { useProductPreview } from "../product-preview";

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
  protocols: string[];
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
  group: string;
  related: {
    same_vendor: PlazaRow[];
    same_price: PlazaRow[];
  };
  max_output?: number;
  meta?: {
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
  const identity = usePlazaIdentity();
  return useQuery({
    queryKey: ["plaza-models", identity, query],
    queryFn: ({ signal }) =>
      apiGet<PlazaListResponse>(
        `/api/plaza/models${buildQueryString(query)}`,
        signal,
      ),
  });
}

export function usePlazaModel(id: string | undefined) {
  const identity = usePlazaIdentity();
  return useQuery({
    queryKey: ["plaza-model", identity, id],
    enabled: Boolean(id),
    queryFn: ({ signal }) =>
      apiGet<PlazaDetailResponse>(
        `/api/plaza/models/${encodeURIComponent(id!)}`,
        signal,
      ),
  });
}

export function usePlazaFilters() {
  const identity = usePlazaIdentity();
  return useQuery({
    queryKey: ["plaza-filters", identity],
    queryFn: ({ signal }) =>
      apiGet<PlazaFiltersResponse>("/api/plaza/filters", signal),
  });
}

function usePlazaIdentity() {
  return useProductPreview((state) =>
    state.user.isLoggedIn ? state.user.id : "anonymous",
  );
}

export interface PlazaDirectoryModel {
  id: string;
  name: string;
  vendorId: string;
  vendor: string;
  capabilities: string[];
  contextLength: number;
  contextDisplay: string;
  maxOutput: number;
  protocols: string[];
  description: { en: string; zh: string; ar: string };
  pricing: PlazaPricing;
  availableGroups: string[];
  isAvailable: boolean;
  unavailableReason: string | null;
}

function formatContextDisplay(length: number): string {
  if (length <= 0) return "—";
  if (length >= 1_000_000) return `${length / 1_000_000}M`;
  if (length >= 1000) return `${length / 1000}K`;
  return String(length);
}

/** Preserve the backend catalog's identifiers, prices and coverage. */
export function plazaToDirectoryModel(row: PlazaRow): PlazaDirectoryModel {
  return {
    id: row.id,
    name: row.name,
    vendorId: row.vendor,
    vendor: row.vendor_display || row.vendor,
    capabilities: row.tags ?? [],
    contextLength: row.context_length,
    contextDisplay: formatContextDisplay(row.context_length),
    maxOutput: row.max_output,
    protocols: row.protocols ?? [],
    description: {
      en: row.description || "",
      zh: row.description || "",
      ar: row.description || "",
    },
    pricing: row.pricing,
    availableGroups: row.available_groups ?? [],
    isAvailable: row.is_available,
    unavailableReason: row.unavailable_reason,
  };
}
