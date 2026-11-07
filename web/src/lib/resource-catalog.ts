import type { ResourceSnapshot, ResourcePlacement } from "../config/models";

export interface ResourceIdentity {
  channel_id: string;
  upstream_model: string;
  canonical_model: string;
  provider: string;
}
export interface ResourceIdentityReview {
  before: ResourceIdentity;
  after: ResourceIdentity;
}
export interface RemovedPlacement {
  id: number;
  resource_id: string;
  model_id: string | null;
  group_ids: string[];
  removed_at: number;
  sequence: number;
  identity: ResourceIdentity;
}

export interface ResourceCatalog {
  placement_reviews?: Record<string, ResourceIdentityReview>;
  removed_placements?: RemovedPlacement[];
  removed_placement_total?: number;
  revision: number;
  snapshot: ResourceSnapshot | null;
  placements: Record<string, ResourcePlacement>;
  regions: Record<string, ResourceRegion>;
  updated_at: number;
  affects_gateway_routing: boolean;
}

export interface ResourceRegion {
  id: string;
  name: string;
  description: string;
  updated_at: number;
}

const object = (value: unknown): value is Record<string, unknown> =>
  Boolean(value && typeof value === "object" && !Array.isArray(value));
const text = (value: unknown, max: number, required = true): value is string =>
  typeof value === "string" &&
  (!required || value.length > 0) &&
  new TextEncoder().encode(value).length <= max &&
  value.trim() === value &&
  !/[\u0000-\u001f\u007f-\u009f]/.test(value);
const integer = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const model = (value: unknown): value is string =>
  text(value, 256) && value !== "myapis-auto" && value !== "veyra-auto";
const enums = (value: unknown, allowed: string[]): boolean =>
  Array.isArray(value) &&
  new Set(value).size === value.length &&
  value.every((entry) => typeof entry === "string" && allowed.includes(entry));

const knownFields = (value: Record<string, unknown>, allowed: string[]) =>
  Object.keys(value).every(
    (key) => allowed.includes(key) && value[key] !== null,
  );
const publicationTime = (value: unknown) => {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) ||
    !Number.isFinite(Date.parse(value))
  )
    return false;
  const year = Number(value.slice(0, 4)),
    month = Number(value.slice(5, 7)),
    day = Number(value.slice(8, 10));
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  return (
    month >= 1 &&
    month <= 12 &&
    day >= 1 &&
    day <=
      [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1] &&
    Number(value.slice(11, 13)) < 24
  );
};

export function parseResourceSnapshot(value: unknown): ResourceSnapshot {
  const invalid = () => {
    throw new Error("Invalid resource snapshot");
  };
  if (
    !object(value) ||
    !knownFields(value, [
      "id",
      "sequence",
      "source",
      "publishedAt",
      "receivedAt",
      "channels",
      "instances",
    ]) ||
    !text(value.id, 128) ||
    !integer(value.sequence) ||
    value.sequence < 1 ||
    !text(value.source, 256, false) ||
    !publicationTime(value.publishedAt) ||
    !Array.isArray(value.channels) ||
    !Array.isArray(value.instances) ||
    value.channels.length > 500 ||
    value.instances.length > 10000
  )
    return invalid();
  const channels = new Set<string>();
  for (const channel of value.channels) {
    if (
      !object(channel) ||
      !knownFields(channel, ["id", "name", "provider", "enabled"]) ||
      !text(channel.id, 128) ||
      channels.has(channel.id) ||
      !text(channel.name, 256) ||
      !text(channel.provider, 256, false) ||
      typeof channel.enabled !== "boolean"
    )
      return invalid();
    channels.add(channel.id);
  }
  const ids = new Set<string>();
  for (const resource of value.instances) {
    if (
      object(resource) &&
      ((resource.price_tier !== undefined &&
        ![4, 6, 8, 10].includes(resource.price_tier as number)) ||
        (resource.quality_level !== undefined &&
          !["S0", "S1", "S2", "U"].includes(
            resource.quality_level as string,
          )) ||
        (resource.state !== undefined &&
          !["hot", "cold", "bad", "disabled"].includes(
            resource.state as string,
          )) ||
        (resource.trend !== undefined &&
          !["stable", "degraded"].includes(resource.trend as string)) ||
        (resource.score !== undefined &&
          (typeof resource.score !== "number" ||
            !Number.isFinite(resource.score) ||
            resource.score < 0 ||
            resource.score > 100)))
    )
      return invalid();
    if (
      !object(resource) ||
      !knownFields(resource, [
        "id",
        "channelId",
        "upstreamModel",
        "canonicalModel",
        "protocols",
        "capabilities",
        "contextLength",
        "region",
        "status",
        "latencyMs",
        "price_tier",
        "quality_level",
        "state",
        "trend",
        "score",
      ]) ||
      !text(resource.id, 128) ||
      ids.has(resource.id) ||
      !text(resource.channelId, 128) ||
      !channels.has(resource.channelId) ||
      !model(resource.upstreamModel) ||
      !model(resource.canonicalModel) ||
      !enums(resource.protocols, ["OpenAI", "Anthropic", "Gemini"]) ||
      !enums(resource.capabilities, [
        "text",
        "code",
        "vision",
        "reasoning",
        "image",
        "video",
        "function_call",
        "responses",
        "audio",
        "embedding",
        "rerank",
      ]) ||
      !integer(resource.contextLength) ||
      !text(resource.region, 128, false) ||
      !["available", "unavailable", "retired"].includes(
        resource.status as string,
      ) ||
      typeof resource.latencyMs !== "number" ||
      !Number.isFinite(resource.latencyMs) ||
      resource.latencyMs < 0 ||
      resource.latencyMs > Number.MAX_SAFE_INTEGER
    )
      return invalid();
    ids.add(resource.id);
  }
  return value as unknown as ResourceSnapshot;
}

export function parseResourceCatalog(value: unknown): ResourceCatalog {
  if (
    !object(value) ||
    !integer(value.revision) ||
    !integer(value.updated_at) ||
    typeof value.affects_gateway_routing !== "boolean" ||
    !object(value.placements) ||
    !object(value.regions) ||
    Object.keys(value.regions).length > 1000
  )
    throw new Error("Invalid resource catalog");
  const snapshot =
    value.snapshot === null ? null : parseResourceSnapshot(value.snapshot);
  for (const [id, region] of Object.entries(value.regions)) {
    if (
      !text(id, 128) ||
      !object(region) ||
      region.id !== id ||
      !text(region.name, 128) ||
      !text(region.description, 512, false) ||
      !integer(region.updated_at)
    )
      throw new Error("Invalid resource region");
  }
  if (
    snapshot &&
    (typeof snapshot.receivedAt !== "string" ||
      !Number.isFinite(Date.parse(snapshot.receivedAt)))
  )
    throw new Error("Invalid resource receipt time");
  const ids = new Set(snapshot?.instances.map((resource) => resource.id) ?? []);
  if (Object.keys(value.placements).length !== ids.size)
    throw new Error("Incomplete resource placements");
  for (const [id, placement] of Object.entries(value.placements)) {
    if (
      !ids.has(id) ||
      !object(placement) ||
      !(placement.modelId === null || model(placement.modelId)) ||
      !Array.isArray(placement.groupIds) ||
      placement.groupIds.length > 100 ||
      !placement.groupIds.every((group) => text(group, 128)) ||
      new Set(placement.groupIds).size !== placement.groupIds.length ||
      typeof placement.enabled !== "boolean" ||
      (placement.enabled &&
        (placement.modelId === null || placement.groupIds.length === 0))
    )
      throw new Error("Invalid resource placement");
  }
  const identity = (entry: unknown): boolean =>
    object(entry) &&
    text(entry.channel_id, 128) &&
    model(entry.upstream_model) &&
    model(entry.canonical_model) &&
    text(entry.provider, 256, false);
  if (value.placement_reviews !== undefined) {
    if (!object(value.placement_reviews))
      throw new Error("Invalid identity reviews");
    for (const [id, review] of Object.entries(value.placement_reviews)) {
      if (
        !ids.has(id) ||
        !object(review) ||
        !identity(review.before) ||
        !identity(review.after) ||
        (value.placements[id] as ResourcePlacement).enabled
      )
        throw new Error("Invalid identity review");
    }
  }
  if (value.removed_placements !== undefined) {
    if (
      !Array.isArray(value.removed_placements) ||
      value.removed_placements.length > 100 ||
      !integer(value.removed_placement_total) ||
      value.removed_placement_total < value.removed_placements.length
    )
      throw new Error("Invalid removed placements");
    const archiveIds = new Set<number>();
    for (const row of value.removed_placements) {
      if (
        !object(row) ||
        !integer(row.id) ||
        row.id === 0 ||
        archiveIds.has(row.id) ||
        !text(row.resource_id, 128) ||
        !(row.model_id === null || model(row.model_id)) ||
        !Array.isArray(row.group_ids) ||
        !row.group_ids.every((group) => text(group, 128)) ||
        !integer(row.removed_at) ||
        !integer(row.sequence) ||
        !identity(row.identity)
      )
        throw new Error("Invalid removed placement");
      archiveIds.add(row.id);
    }
  }
  return value as unknown as ResourceCatalog;
}

export function resourceImportDiff(
  current: ResourceSnapshot | null,
  next: ResourceSnapshot,
) {
  const before = new Map(current?.instances.map((row) => [row.id, row]) ?? []);
  const after = new Map(next.instances.map((row) => [row.id, row]));
  return {
    added: next.instances.filter((row) => !before.has(row.id)).length,
    removed: [...before.keys()].filter((id) => !after.has(id)).length,
    changed: next.instances.filter(
      (row) =>
        before.has(row.id) &&
        JSON.stringify(before.get(row.id)) !== JSON.stringify(row),
    ).length,
  };
}
