import type { ResourceSnapshot, ResourcePlacement } from "../config/models";

export interface ResourceCatalog {
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

export function parseResourceSnapshot(value: unknown): ResourceSnapshot {
  const invalid = () => {
    throw new Error("Invalid resource snapshot");
  };
  if (
    !object(value) ||
    !text(value.id, 128) ||
    !integer(value.sequence) ||
    value.sequence < 1 ||
    !text(value.source, 256, false) ||
    typeof value.publishedAt !== "string" ||
    !Number.isFinite(Date.parse(value.publishedAt)) ||
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
      !object(resource) ||
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
