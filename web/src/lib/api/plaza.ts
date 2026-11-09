// Plaza API hooks — calls the Go backend /api/plaza/* endpoints.
//
// The backend returns plazaRow objects with snake_case JSON keys and
// a nested pricing object. The adapter (plazaToDirectoryModel)
// converts these to the frontend DirectoryModel shape used by the
// catalog pages.

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import { useSessionQueryIdentity } from "../system-access";
import type { DirectoryModel } from "../../config/model-directory";
import type { TierId } from "../../config/placeholders";
import type { ServiceLevel } from "../service-level";
import { timeQuoteRefreshDelay, type TimeQuote } from "../time-quote";

// --- Backend response types (mirror controller/plaza_pricing.go) ---

export interface PublicGroupQuote {
  id: string;
  name: string;
  available: boolean;
  callable: boolean;
  multiplier: number;
  pricing: PlazaPricing;
  protocols: string[];
  service_level: ServiceLevel;
}

export interface CatalogStatus {
  state: "normal" | "degraded" | "outage" | "insufficient_data";
  samples: number;
  success_rate: number | null;
  ttft_p95_ms: number | null;
  window_minutes: number;
  at: number;
}

export interface PlazaPricing {
  unit?: "million_tokens" | "request" | "image" | "second";
  mode: string;
  input_per_m: number | null;
  output_per_m: number | null;
  cache_read_per_m: number | null;
  cache_write_per_m: number | null;
  audio_input_per_m?: number | null;
  audio_output_per_m?: number | null;
  per_request: number | null;
  expression?: string | null;
  official_input_per_m: number | null;
  official_output_per_m: number | null;
  official_cache_read_per_m: number | null;
  official_cache_write_per_m: number | null;
  official_per_request: number | null;
  group_ratio: number;
  explicit: boolean;
  currency: string;
  time_pricing?: TimeQuote;
}

export interface PlazaRow {
  public_catalog?: boolean;
  kind?: "specified" | "smart";
  price_group?: string;
  callable?: boolean;
  group_prices?: PublicGroupQuote[];
  official_source?: string;
  official_verified?: string;
  official_conditions?: string;
  official_historical?: boolean;
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
  group_sla?: ServiceLevel | null;
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
  status?: CatalogStatus;
  model: PlazaRow;
  group: string;
  group_sla?: ServiceLevel | null;
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
    expression?: string | null;
  } | null;
}

// --- Query params ---

export interface PlazaQuery {
  scope?: "public";
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

export function usePlazaModels(query: PlazaQuery = {}, enabled = true) {
  const identity = usePlazaIdentity();
  return useQuery<PlazaListResponse>({
    queryKey: ["plaza-models", identity, query],
    enabled,
    refetchInterval: (state) =>
      timeQuoteRefreshDelay(
        state.state.data?.items.map((row) => row.pricing.time_pricing) ?? [],
      ) || (query.scope === "public" ? 60_000 : false),
    staleTime: 60_000,
    refetchOnWindowFocus: query.scope === "public",
    queryFn: ({ signal }) =>
      apiGet<PlazaListResponse>(
        `/api/${query.scope === "public" ? "catalog" : "plaza"}/models${buildQueryString(query)}`,
        signal,
      ),
  });
}

export function usePlazaModel(id: string | undefined, publicCatalog = false) {
  const identity = usePlazaIdentity();
  return useQuery<PlazaDetailResponse>({
    queryKey: [publicCatalog ? "catalog-model" : "plaza-model", identity, id],
    enabled: Boolean(id),
    refetchInterval: (state) =>
      timeQuoteRefreshDelay([state.state.data?.model.pricing.time_pricing]) ||
      (publicCatalog ? 60_000 : false),
    staleTime: 60_000,
    refetchOnWindowFocus: publicCatalog,
    queryFn: ({ signal }) =>
      apiGet<PlazaDetailResponse>(
        `/api/${publicCatalog ? "catalog" : "plaza"}/models/${encodeURIComponent(id!)}`,
        signal,
      ),
  });
}

export function usePlazaFilters(publicCatalog = false) {
  const identity = usePlazaIdentity();
  return useQuery({
    queryKey: [publicCatalog ? "catalog-filters" : "plaza-filters", identity],
    staleTime: 60_000,
    refetchInterval: 60_000,
    queryFn: ({ signal }) =>
      apiGet<PlazaFiltersResponse>(
        `/api/${publicCatalog ? "catalog" : "plaza"}/filters`,
        signal,
      ),
  });
}

// Session transitions rotate the cache even between accounts with the same role.
// The backend still resolves the actual group from its cookie on every request.
function usePlazaIdentity() {
  return useSessionQueryIdentity();
}

// --- Adapter: PlazaRow -> DirectoryModel ---

function formatContextDisplay(length: number): string {
  if (length <= 0) return "";
  if (length >= 1_000_000) return `${(length / 1_000_000).toFixed(0)}M`;
  if (length >= 1000) return `${Math.round(length / 1000)}K`;
  return String(length);
}

const TIER_IDS: TierId[] = ["ultra", "pro", "standard", "lite"];

export type PlazaDirectoryModel = DirectoryModel & {
  publicCatalog?: boolean;
  priceGroup?: string;
  callable?: boolean;
  groupPrices?: PublicGroupQuote[];
  vendorId: string;
  pricing: PlazaPricing;
  availableGroups: string[];
};

/** Resolve older display-name URLs while preserving vendor identifiers. */
export function resolvePlazaVendor(
  value: string,
  vendors: readonly PlazaVendorFacet[],
): string {
  return (
    vendors.find((vendor) => vendor.value === value)?.value ??
    vendors.find((vendor) => vendor.label === value)?.value ??
    value
  );
}

/** Convert a backend PlazaRow to the frontend DirectoryModel shape. */
export function plazaToDirectoryModel(row: PlazaRow): PlazaDirectoryModel {
  const tags = row.tags ?? [];
  const capabilities = tags.filter(
    (t): t is DirectoryModel["capabilities"][number] =>
      [
        "text",
        "code",
        "vision",
        "reasoning",
        "image",
        "video",
        "audio",
        "embedding",
        "rerank",
      ].includes(t),
  ) as DirectoryModel["capabilities"];

  // Derive tierAvailability from available_groups: any group means
  // available at all tiers (the backend doesn't expose per-tier yet).
  const tierAvailability = row.group_prices
    ? Object.fromEntries(row.group_prices.map((g) => [g.id, g.available]))
    : TIER_IDS.reduce(
        (acc, tier) => {
          acc[tier] = row.is_available;
          return acc;
        },
        {} as Record<TierId, boolean>,
      );

  return {
    publicCatalog: row.public_catalog,
    priceGroup: row.price_group,
    callable: row.callable,
    groupPrices: row.group_prices,
    id: row.id,
    name: row.name,
    vendor: row.vendor_display || row.vendor,
    vendorId: row.vendor,
    kind: row.kind ?? "specified",
    capabilities: capabilities.length
      ? capabilities
      : row.public_catalog
        ? []
        : ["text"],
    contextLength: row.context_length,
    contextDisplay: formatContextDisplay(row.context_length),
    protocols: (row.protocols ?? []).filter(
      (protocol): protocol is DirectoryModel["protocols"][number] =>
        protocol === "OpenAI" ||
        protocol === "Anthropic" ||
        protocol === "Gemini",
    ),
    pricing: row.pricing,
    availableGroups: row.available_groups,
    description: {
      en: row.description || "",
      zh: row.description || "",
      ar: row.description || "",
    },
    tierAvailability,
    online: row.is_available,
  };
}
