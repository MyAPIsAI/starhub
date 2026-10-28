export const pricingKeys = [
  "model_ratio",
  "completion_ratio",
  "model_price",
  "model_price_expr",
  "cache_ratio",
  "audio_ratio",
] as const;
export type PricingKey = (typeof pricingKeys)[number];
export type PricingOption = { key: PricingKey; value: string };
export interface ServerPricing {
  version: number;
  loaded_version: number;
  atomic_batch: true;
  options: PricingOption[];
  input: Record<string, number>;
  completion: Record<string, number>;
  perRequest: Record<string, number>;
  expressions: Record<string, string>;
}
const version = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
function map(
  value: unknown,
  expressions = false,
): Record<string, number | string> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw Error("Invalid pricing map");
  const entries = Object.entries(value);
  if (
    entries.some(
      ([key, entry]) =>
        !key.trim() ||
        (expressions
          ? typeof entry !== "string"
          : typeof entry !== "number" ||
            !Number.isFinite(entry) ||
            entry < 0 ||
            entry > 1e9),
    )
  )
    throw Error("Invalid pricing map");
  return Object.fromEntries(entries);
}
export function readServerPricing(value: unknown): ServerPricing {
  if (!value || typeof value !== "object")
    throw Error("Invalid server pricing");
  const state = value as Record<string, unknown>;
  if (
    !version(state.version) ||
    !version(state.loaded_version) ||
    state.atomic_batch !== true ||
    !Array.isArray(state.options) ||
    state.options.length !== pricingKeys.length
  )
    throw Error("Unsupported server pricing");
  const options = state.options.map((value) => {
    if (!value || typeof value !== "object")
      throw Error("Invalid pricing option");
    const item = value as Record<string, unknown>;
    if (
      !pricingKeys.includes(item.key as PricingKey) ||
      typeof item.value !== "string"
    )
      throw Error("Invalid pricing option");
    return { key: item.key as PricingKey, value: item.value };
  });
  if (new Set(options.map((o) => o.key)).size !== pricingKeys.length)
    throw Error("Duplicate pricing option");
  const parsed = Object.fromEntries(
    options.map((o) => [o.key, JSON.parse(o.value) as unknown]),
  );
  for (const key of ["cache_ratio", "audio_ratio"]) {
    const rate = parsed[key];
    if (
      typeof rate !== "number" ||
      !Number.isFinite(rate) ||
      rate < 0 ||
      rate > 1000
    )
      throw Error("Invalid detail ratio");
  }
  return {
    version: state.version,
    loaded_version: state.loaded_version,
    atomic_batch: true,
    options,
    input: map(parsed.model_ratio) as Record<string, number>,
    completion: map(parsed.completion_ratio) as Record<string, number>,
    perRequest: map(parsed.model_price) as Record<string, number>,
    expressions: map(parsed.model_price_expr, true) as Record<string, string>,
  };
}
export function ownPrice<T>(
  values: Record<string, T>,
  id: string,
): T | undefined {
  return Object.prototype.hasOwnProperty.call(values, id)
    ? values[id]
    : undefined;
}
export function serverPricingMode(
  state: ServerPricing,
  id: string,
): "per_token" | "per_request" | "expression" {
  return ownPrice(state.expressions, id)?.trim()
    ? "expression"
    : (ownPrice(state.perRequest, id) ?? 0) > 0
      ? "per_request"
      : "per_token";
}
export function validServerRatio(value: string): boolean {
  return (
    /^(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(value.trim()) &&
    Number.isFinite(Number(value)) &&
    Number(value) >= 0 &&
    Number(value) <= 1e9
  );
}
export function replaceServerModelRatios(
  state: ServerPricing,
  id: string,
  input: string,
  completion: string,
): PricingOption[] {
  if (
    !id.trim() ||
    !validServerRatio(input) ||
    !validServerRatio(completion) ||
    serverPricingMode(state, id) !== "per_token"
  )
    throw Error("Invalid token pricing edit");
  return [
    {
      key: "model_ratio",
      value: JSON.stringify({ ...state.input, [id]: Number(input) }),
    },
    {
      key: "completion_ratio",
      value: JSON.stringify({ ...state.completion, [id]: Number(completion) }),
    },
  ];
}
export function validServerRequestPrice(value: string): boolean {
  return validServerRatio(value) && Number(value) > 0 && Number(value) <= 10000;
}
export function replaceServerRequestPrice(
  state: ServerPricing,
  id: string,
  price: string,
): PricingOption[] {
  if (
    !id.trim() ||
    !validServerRequestPrice(price) ||
    serverPricingMode(state, id) !== "per_request"
  )
    throw Error("Invalid per-request pricing edit");
  return [
    {
      key: "model_price",
      value: JSON.stringify({ ...state.perRequest, [id]: Number(price) }),
    },
  ];
}
export function validServerExpression(value: string): boolean {
  return (
    value.trim().length > 0 && new TextEncoder().encode(value).length <= 512
  );
}
export function replaceServerExpression(
  state: ServerPricing,
  id: string,
  expression: string,
): PricingOption[] {
  if (
    !id.trim() ||
    !validServerExpression(expression) ||
    serverPricingMode(state, id) !== "expression"
  )
    throw Error("Invalid expression pricing edit");
  return [
    {
      key: "model_price_expr",
      value: JSON.stringify({ ...state.expressions, [id]: expression }),
    },
  ];
}
export function readPricingAcknowledgement(value: unknown): {
  version: number;
  loaded_version: number;
} {
  if (!value || typeof value !== "object")
    throw Error("Invalid pricing acknowledgement");
  const result = value as Record<string, unknown>;
  if (!version(result.version) || !version(result.loaded_version))
    throw Error("Invalid pricing acknowledgement");
  return { version: result.version, loaded_version: result.loaded_version };
}
