/** Shared request record type. Originally defined in mocks/service.ts;
 *  extracted to break the type dependency chain from lib/billing.ts
 *  and lib/product-preview.ts to the mock data module. */
export interface RequestRecord {
  trace_id: string;
  model: string;
  vendor: string;
  tier: import("../config/placeholders").TierId;
  status: "success" | "failed" | "interrupted";
  time: string;
  cost_nano: string;
  input_tokens: number;
  output_tokens: number;
  cache_tokens: number;
  duration_ms: number;
  ttft_ms: number;
  fallback: boolean;
  source: "playground";
  error_code: string | null;
  input_price_nano: string;
  output_price_nano: string;
}
