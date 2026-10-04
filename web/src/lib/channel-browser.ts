import type { PricingCatalogConfig, ResourceInstance } from "../config/models";
import { modelCatalog } from "./platform-catalog";

export type ChannelViewMode = "channel" | "model";
export type ResourceStatusFilter =
  "all" | "ready" | "unmapped" | "paused" | "unavailable";

export interface ChannelBrowserGroup {
  id: string;
  name: string;
  subtitle: string;
  total: number;
  ready: number;
  unmapped: number;
}

export interface ChannelBrowserOptions {
  view: ChannelViewMode;
  selectedId?: string;
  query?: string;
  groupId?: string;
  status?: ResourceStatusFilter;
}

/** Operational presentation only. Request authorization stays in key-model-policy. */
export function getResourceState(
  config: PricingCatalogConfig,
  resource: ResourceInstance,
): { id: Exclude<ResourceStatusFilter, "all">; label: string } {
  const channel = config.resources?.channels.find(
    (item) => item.id === resource.channelId,
  );
  if (!channel?.enabled || resource.status !== "available")
    return {
      id: "unavailable",
      label: resource.status === "retired" ? "来源已下架" : "来源不可用",
    };

  const placement = config.placements?.[resource.id];
  const model = modelCatalog(config).find(
    (item) => item.id === placement?.modelId,
  );
  if (!model) return { id: "unmapped", label: "待映射" };
  if (!placement?.enabled || !placement.groupIds.length)
    return { id: "paused", label: "未投放" };
  if (!model.online) return { id: "paused", label: "模型已停用" };
  if (!placement.groupIds.some((id) => config.groups[id]?.enabled))
    return { id: "paused", label: "分组未启用" };
  return { id: "ready", label: "可用" };
}

export function browseChannelResources(
  config: PricingCatalogConfig,
  options: ChannelBrowserOptions,
): {
  groups: ChannelBrowserGroup[];
  selectedGroup: ChannelBrowserGroup | undefined;
  rows: ResourceInstance[];
  /** Selected left-hand group's count before right-hand filters. */
  total: number;
  filteredTotal: number;
} {
  const resources = config.resources?.instances ?? [];
  const channels = config.resources?.channels ?? [];
  const models = modelCatalog(config);
  const byModel = new Map(models.map((model) => [model.id, model]));
  const byChannel = new Map(channels.map((channel) => [channel.id, channel]));
  const states = new Map(
    resources.map((resource) => [
      resource.id,
      getResourceState(config, resource),
    ]),
  );
  const groupIdFor = (resource: ResourceInstance) => {
    if (options.view === "channel") return resource.channelId;
    const mappedId = config.placements?.[resource.id]?.modelId;
    return mappedId && byModel.has(mappedId) ? mappedId : "__unmapped__";
  };
  const definitions =
    options.view === "channel"
      ? channels.map((channel) => ({
          id: channel.id,
          name: channel.name,
          subtitle: channel.provider,
        }))
      : [
          ...models.map((model) => ({
            id: model.id,
            name: model.name,
            subtitle: model.vendor,
          })),
          {
            id: "__unmapped__",
            name: "待映射资源",
            subtitle: "尚未关联标准模型",
          },
        ];
  const members = new Map<string, ResourceInstance[]>();
  for (const resource of resources) {
    const id = groupIdFor(resource);
    const rows = members.get(id) ?? [];
    rows.push(resource);
    members.set(id, rows);
  }
  members.set("__all__", resources);
  definitions.unshift({
    id: "__all__",
    name: options.view === "channel" ? "全部渠道" : "全部标准模型",
    subtitle: "全部资源实例",
  });
  const groups = definitions.map((group) => {
    const rows = members.get(group.id) ?? [];
    return {
      ...group,
      total: rows.length,
      ready: rows.filter((row) => states.get(row.id)?.id === "ready").length,
      unmapped: rows.filter((row) => states.get(row.id)?.id === "unmapped")
        .length,
    };
  });
  const terms = (options.query ?? "").trim().toLocaleLowerCase().split(/\s+/);
  const matches = (resource: ResourceInstance) => {
    const placement = config.placements?.[resource.id];
    const model = placement?.modelId
      ? byModel.get(placement.modelId)
      : undefined;
    const channel = byChannel.get(resource.channelId);
    const searchable = [
      resource.id,
      resource.upstreamModel,
      resource.canonicalModel,
      placement?.modelId,
      model?.name,
      model?.vendor,
      channel?.name,
      channel?.provider,
    ]
      .filter(Boolean)
      .join(" ")
      .toLocaleLowerCase();
    return (
      terms.every((term) => searchable.includes(term)) &&
      (!options.groupId || placement?.groupIds.includes(options.groupId)) &&
      (!options.status ||
        options.status === "all" ||
        states.get(resource.id)?.id === options.status)
    );
  };
  // Filters never move the left-hand selection, including an empty result.
  const selectedGroup =
    groups.find((group) => group.id === options.selectedId) ?? groups[0];
  const allSelected = selectedGroup
    ? (members.get(selectedGroup.id) ?? [])
    : [];
  const rows = allSelected.filter(matches);
  return {
    groups,
    selectedGroup,
    rows,
    total: allSelected.length,
    filteredTotal: rows.length,
  };
}
