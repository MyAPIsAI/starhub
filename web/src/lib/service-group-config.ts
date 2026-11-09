import type { ServiceGroup } from "./api/service-group";
import { readServiceLevel } from "./service-level";

export interface ManagedServiceGroup extends ServiceGroup {
  revision: number;
}
export interface ServiceGroupCatalog {
  groups: ManagedServiceGroup[];
  group_ratios: Record<string, number>;
  option_version: number;
  loaded_option_version: number;
  revision_guards: true;
  option_version_guard: true;
  account_access_guard: true;
  account_list_validation: true;
}

const integer = (value: unknown, min = 0): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= min;

export function readManagedServiceGroup(value: unknown): ManagedServiceGroup {
  if (!value || typeof value !== "object")
    throw new Error("Invalid group definition");
  const g = value as Record<string, unknown>;
  if (
    !integer(g.id, 1) ||
    !integer(g.revision) ||
    typeof g.group_id !== "string" ||
    !g.group_id ||
    typeof g.name !== "string" ||
    typeof g.description !== "string" ||
    typeof g.enabled !== "boolean" ||
    (g.status_page_mode !== undefined &&
      !["auto", "include", "exclude"].includes(g.status_page_mode as string)) ||
    (g.visibility !== "common" && g.visibility !== "custom") ||
    !Array.isArray(g.account_ids) ||
    g.account_ids.some((id) => typeof id !== "string") ||
    typeof g.multiplier !== "string" ||
    !integer(g.created_at) ||
    !integer(g.updated_at)
  )
    throw new Error("Invalid group definition");
  return {
    id: g.id,
    revision: g.revision,
    group_id: g.group_id,
    name: g.name,
    description: g.description,
    enabled: g.enabled,
    status_page_mode:
      (g.status_page_mode as "auto" | "include" | "exclude" | undefined) ??
      "auto",
    visibility: g.visibility,
    account_ids: [...g.account_ids] as string[],
    multiplier: g.multiplier,
    created_at: g.created_at,
    updated_at: g.updated_at,
    sla: readServiceLevel(g.sla),
  };
}

// Explicit capabilities are required even when there are no definitions to inspect.
export function readServiceGroupCatalog(value: unknown): ServiceGroupCatalog {
  if (!value || typeof value !== "object")
    throw new Error("Invalid group configuration");
  const c = value as Record<string, unknown>;
  if (
    c.revision_guards !== true ||
    c.option_version_guard !== true ||
    c.account_access_guard !== true ||
    c.account_list_validation !== true ||
    !integer(c.option_version) ||
    !integer(c.loaded_option_version) ||
    !Array.isArray(c.groups) ||
    !c.group_ratios ||
    typeof c.group_ratios !== "object" ||
    Array.isArray(c.group_ratios)
  )
    throw new Error("Unsupported group configuration");
  const groups = c.groups.map(readManagedServiceGroup);
  if (
    new Set(groups.map((g) => g.id)).size !== groups.length ||
    new Set(groups.map((g) => g.group_id)).size !== groups.length
  )
    throw new Error("Duplicate group identity");
  const ratios = Object.entries(c.group_ratios);
  if (
    ratios.some(
      ([key, ratio]) =>
        !key.trim() ||
        typeof ratio !== "number" ||
        !Number.isFinite(ratio) ||
        ratio < 0 ||
        ratio > 1e9,
    )
  )
    throw new Error("Invalid configured ratio");
  return {
    groups,
    group_ratios: Object.fromEntries(ratios) as Record<string, number>,
    option_version: c.option_version,
    loaded_option_version: c.loaded_option_version,
    revision_guards: true,
    option_version_guard: true,
    account_access_guard: true,
    account_list_validation: true,
  };
}

export function configuredGroupRatio(
  catalog: ServiceGroupCatalog,
  id: string,
): number | undefined {
  return Object.prototype.hasOwnProperty.call(catalog.group_ratios, id)
    ? catalog.group_ratios[id]
    : undefined;
}

export function validGroupDefinitionID(id: string): boolean {
  return /^[a-z0-9][a-z0-9_-]{1,63}$/.test(id);
}

// Keep IDs as strings so int64 account identities never pass through Number.
export function parseGroupAccountIDs(value: string): string[] | null {
  const ids = value.trim().split(/[\s,]+/);
  if (
    ids.length > 100 ||
    new Set(ids).size !== ids.length ||
    ids.join(",").length > 1024 ||
    ids.some((id) => !/^[1-9][0-9]{0,18}$/.test(id))
  )
    return null;
  return ids.every((id) => BigInt(id) <= 9223372036854775807n) ? ids : null;
}

export function validGroupMultiplier(value: string): boolean {
  const text = value.trim();
  if (text.length > 128) return false;
  const match = /^\+?(\d+(?:\.\d*)?|\.\d+)(?:[eE]([+-]?\d+))?$/.exec(text);
  if (!match) return false;
  const exponent = Number(match[2] ?? 0);
  if (!Number.isInteger(exponent) || Math.abs(exponent) > 1000) return false;
  const fraction = match[1].split(".")[1]?.length ?? 0;
  const power = 4 + exponent - fraction;
  let scaled = BigInt(match[1].replace(".", ""));
  if (power >= 0) scaled *= 10n ** BigInt(power);
  else {
    const divisor = 10n ** BigInt(-power);
    if (scaled % divisor !== 0n) return false;
    scaled /= divisor;
  }
  return scaled <= 1000000n;
}
