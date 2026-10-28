import type { ModelMeta } from "./api/plaza-meta";

const stringFields = [
  "model",
  "vendor",
  "vendor_display",
  "description",
  "tags",
  "compatible_groups",
] as const;
const nullablePrices = [
  "official_input_per_m",
  "official_output_per_m",
  "official_cache_read_per_m",
  "official_cache_write_per_m",
  "official_per_request",
  "per_request_usd",
] as const;
export function readModelMetadata(value: unknown): ModelMeta {
  if (!value || typeof value !== "object")
    throw Error("Invalid model metadata");
  const row = value as Record<string, unknown>;
  if (
    stringFields.some((key) => typeof row[key] !== "string") ||
    !(row.model as string).trim() ||
    ["context_length", "max_output"].some(
      (key) =>
        typeof row[key] !== "number" ||
        !Number.isSafeInteger(row[key]) ||
        (row[key] as number) < 0,
    ) ||
    ["sort_weight", "created_at"].some(
      (key) => typeof row[key] !== "number" || !Number.isSafeInteger(row[key]),
    ) ||
    nullablePrices.some(
      (key) =>
        row[key] !== null &&
        (typeof row[key] !== "number" ||
          !Number.isFinite(row[key]) ||
          (row[key] as number) < 0),
    ) ||
    (row.score !== null &&
      (typeof row.score !== "number" ||
        !Number.isFinite(row.score) ||
        row.score < 0 ||
        row.score > 100))
  )
    throw Error("Invalid model metadata");
  return { ...row } as unknown as ModelMeta;
}
export function readModelMetadataList(value: unknown): ModelMeta[] {
  if (!Array.isArray(value)) throw Error("Invalid model metadata list");
  const rows = value.map(readModelMetadata);
  if (new Set(rows.map((row) => row.model)).size !== rows.length)
    throw Error("Duplicate model metadata");
  return rows.sort((a, b) => a.model.localeCompare(b.model));
}
export interface ModelMetadataCatalog {
  revision: number;
  revision_guards: true;
  models: ModelMeta[];
}
const revision = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
export function readModelMetadataCatalog(value: unknown): ModelMetadataCatalog {
  if (!value || typeof value !== "object")
    throw Error("Invalid model metadata catalog");
  const state = value as Record<string, unknown>;
  if (state.revision_guards !== true || !revision(state.revision))
    throw Error("Unsupported model metadata catalog");
  return {
    revision: state.revision,
    revision_guards: true,
    models: readModelMetadataList(state.models),
  };
}
export function readMetadataSave(value: unknown): {
  meta: ModelMeta;
  revision: number;
} {
  if (!value || typeof value !== "object")
    throw Error("Invalid metadata acknowledgement");
  const result = value as Record<string, unknown>;
  if (!revision(result.revision))
    throw Error("Invalid metadata acknowledgement");
  return { meta: readModelMetadata(result.meta), revision: result.revision };
}
export function readMetadataDelete(value: unknown): {
  model: string;
  revision: number;
} {
  if (!value || typeof value !== "object")
    throw Error("Invalid deletion acknowledgement");
  const result = value as Record<string, unknown>;
  if (
    typeof result.model !== "string" ||
    !result.model ||
    !revision(result.revision)
  )
    throw Error("Invalid deletion acknowledgement");
  return { model: result.model, revision: result.revision };
}
export function emptyModelMetadata(): ModelMeta {
  return {
    model: "",
    vendor: "",
    vendor_display: "",
    description: "",
    tags: "",
    context_length: 0,
    max_output: 0,
    official_input_per_m: null,
    official_output_per_m: null,
    official_cache_read_per_m: null,
    official_cache_write_per_m: null,
    official_per_request: null,
    per_request_usd: null,
    score: null,
    compatible_groups: "",
    sort_weight: 0,
    created_at: 0,
  };
}
