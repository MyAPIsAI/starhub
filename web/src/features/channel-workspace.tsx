import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useSearchParams, Link } from "react-router-dom";
import {
  Network,
  Boxes,
  Search,
  Layers3,
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  SlidersHorizontal,
  Inbox,
  X,
  CircleAlert,
} from "lucide-react";
import type { PricingCatalogConfig, ResourceInstance } from "../config/models";
import { modelCatalog } from "../lib/platform-catalog";
import {
  browseChannelResources,
  getResourceState,
  type ChannelViewMode,
  type ResourceStatusFilter,
} from "../lib/channel-browser";
import { useSettingsText } from "./settings-text";
import { Button } from "../components/ui/button";
import "../components/channel-workspace.css";

export function ChannelWorkspace({
  config,
  onEdit,
  onRemoved,
  removedCount,
}: {
  config: PricingCatalogConfig;
  onEdit: (id: string) => void;
  onRemoved: () => void;
  removedCount: number;
}) {
  const { text } = useSettingsText();
  const [params, setParams] = useSearchParams();
  const snapshot = config.resources!;
  const view: ChannelViewMode =
    params.get("view") === "model" ? "model" : "channel";
  const selectedId = params.get(view) ?? "__all__";
  const query = params.get("query") ?? "";
  const groupId = params.get("group") ?? "";
  const status: ResourceStatusFilter = (
    ["ready", "unmapped", "paused", "unavailable"].includes(
      params.get("status") ?? "",
    )
      ? params.get("status")
      : "all"
  ) as ResourceStatusFilter;
  const [indexSearch, setIndexSearch] = useState("");
  const [expandedGroups, setExpandedGroups] = useState<string[]>([]);
  const listRef = useRef<HTMLDivElement>(null);
  const rowsRef = useRef<HTMLDivElement>(null);
  const result = browseChannelResources(config, {
    view,
    selectedId,
    query,
    groupId,
    status,
  });
  const selected = result.selectedGroup;
  const selectedAll = selected?.id === "__all__";
  const models = modelCatalog(config);
  const selectedModel =
    view === "model" ? models.find((m) => m.id === selected?.id) : undefined;
  const selectedChannel =
    view === "channel"
      ? snapshot.channels.find((c) => c.id === selected?.id)
      : undefined;
  const visibleGroups = result.groups.filter(
    (g) =>
      !indexSearch ||
      `${g.name} ${g.subtitle} ${g.id}`
        .toLowerCase()
        .includes(indexSearch.toLowerCase()),
  );
  const pageSize = 8;
  const pageCount = Math.max(1, Math.ceil(result.rows.length / pageSize));
  const requestedPage = Math.max(1, Number(params.get("page")) || 1);
  const page = Math.min(Math.floor(requestedPage), pageCount);
  const pageRows = result.rows.slice((page - 1) * pageSize, page * pageSize);
  const change = (updates: Record<string, string | null>, replace = false) => {
    setParams(
      (current) => {
        const next = new URLSearchParams(current);
        for (const [key, value] of Object.entries(updates)) {
          if (value === null || value === "") next.delete(key);
          else next.set(key, value);
        }
        return next;
      },
      { replace },
    );
  };
  const choose = (id: string) =>
    change({ [view]: id === "__all__" ? null : id, page: null });
  const switchView = (next: ChannelViewMode) => {
    if (next === view) return;
    setIndexSearch("");
    change({ view: next, page: null });
  };
  const showRelated = (resource: ResourceInstance) => {
    setIndexSearch("");
    change(
      view === "channel"
        ? {
            view: "model",
            model: config.placements?.[resource.id]?.modelId ?? "__unmapped__",
            page: null,
          }
        : { view: "channel", channel: resource.channelId, page: null },
    );
  };
  const clearFilters = () =>
    change({ query: null, group: null, status: null, page: null });
  const filtered = Boolean(query || groupId || status !== "all");
  const groupName = (id: string, name: string) =>
    id === "__all__"
      ? view === "channel"
        ? text(...translatedMessage("all_channels_4c3a0b8907"))
        : text(...translatedMessage("all_standard_models_c3130891d3"))
      : id === "__unmapped__"
        ? text(...translatedMessage("unmapped_resources_0e0d3146fb"))
        : name;
  const statusNames = {
    all: text(...translatedMessage("all_statuses_df82d81776")),
    ready: text(...translatedMessage("ready_to_serve_f881e287bd")),
    unmapped: text(...translatedMessage("needs_mapping_b4fd33786a")),
    paused: text(...translatedMessage("placement_paused_63daaa7870")),
    unavailable: text(...translatedMessage("source_unavailable_148180cd18")),
  };
  useEffect(() => {
    rowsRef.current?.scrollTo({ top: 0 });
  }, [view, selectedId, query, groupId, status, page]);
  useEffect(() => {
    listRef.current
      ?.querySelector('[aria-current="true"]')
      ?.scrollIntoView({ block: "nearest" });
  }, [view, selectedId]);
  const listKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    const buttons = Array.from(
      listRef.current?.querySelectorAll<HTMLButtonElement>("[data-group-id]") ??
        [],
    );
    const index = buttons.indexOf(event.target as HTMLButtonElement);
    if (index < 0) return;
    const target =
      event.key === "ArrowDown"
        ? Math.min(index + 1, buttons.length - 1)
        : event.key === "ArrowUp"
          ? Math.max(0, index - 1)
          : event.key === "Home"
            ? 0
            : event.key === "End"
              ? buttons.length - 1
              : -1;
    if (target >= 0) {
      event.preventDefault();
      buttons[target]?.focus();
      buttons[target]?.click();
    }
  };
  const contextName = selected
    ? groupName(selected.id, selected.name)
    : text(...translatedMessage("resource_instances_1159731b8a"));
  const renderGroups = (r: ResourceInstance) => {
    const ids = config.placements?.[r.id]?.groupIds ?? [];
    const expanded = expandedGroups.includes(r.id);
    return ids.length ? (
      <div className="channel-group-tags">
        {(expanded ? ids : ids.slice(0, 2)).map((id) => (
          <span key={id} title={config.groups[id]?.description}>
            {config.groups[id]?.name ?? id}
          </span>
        ))}
        {ids.length > 2 && (
          <button
            type="button"
            aria-expanded={expanded}
            aria-label={
              expanded
                ? text(
                    ...translatedMessage("collapse_groups_for_5aaed573dc", {
                      p0: r.id,
                    }),
                  )
                : text(
                    ...translatedMessage("show_all_groups_for_7b36c08252", {
                      p0: r.id,
                    }),
                  )
            }
            onClick={() =>
              setExpandedGroups((values) =>
                expanded
                  ? values.filter((id) => id !== r.id)
                  : [...values, r.id],
              )
            }
          >
            {expanded
              ? text(...translatedMessage("less_26c35982e7"))
              : `+${ids.length - 2}`}
          </button>
        )}
      </div>
    ) : (
      <span className="channel-muted">
        {text(...translatedMessage("not_placed_b1121df1b9"))}
      </span>
    );
  };
  const renderState = (r: ResourceInstance) => {
    const state = getResourceState(config, r);
    const sourceOk =
      r.status === "available" &&
      snapshot.channels.find((c) => c.id === r.channelId)?.enabled;
    return (
      <div className="channel-row-state">
        <span className={`channel-status is-${state.id}`}>
          <i />
          {state.id !== "paused"
            ? statusNames[state.id]
            : state.label === "模型已停用"
              ? text(...translatedMessage("model_disabled_5dcaa603d0"))
              : state.label === "分组未启用"
                ? text(...translatedMessage("groups_disabled_6bdef29fdc"))
                : text(...translatedMessage("not_placed_b1121df1b9"))}
        </span>
        <small>
          {sourceOk
            ? text(...translatedMessage("source_healthy_dc8280687c"))
            : r.status === "retired"
              ? text(...translatedMessage("source_retired_ef09cf1529"))
              : text(...translatedMessage("source_unavailable_b0c1804be3"))}
          {!sourceOk && config.placements?.[r.id]?.enabled
            ? text(...translatedMessage("placement_retained_4ec29c3040"))
            : ""}
        </small>
      </div>
    );
  };
  const relatedName = (r: ResourceInstance) =>
    view === "channel"
      ? (models.find((m) => m.id === config.placements?.[r.id]?.modelId)
          ?.name ?? text(...translatedMessage("unmapped_6d198b7550")))
      : (snapshot.channels.find((c) => c.id === r.channelId)?.name ??
        r.channelId);
  const relatedId = (r: ResourceInstance) =>
    view === "channel" ? config.placements?.[r.id]?.modelId : r.channelId;
  return (
    <section
      className="channel-workspace"
      aria-label={text(...translatedMessage("resource_workspace_57b1695233"))}
    >
      <div className="channel-view-bar">
        <div
          className="channel-view-switch"
          role="group"
          aria-label={text(...translatedMessage("resource_view_36383c9316"))}
        >
          <button
            aria-pressed={view === "channel"}
            onClick={() => switchView("channel")}
          >
            <Network size={16} />
            {text(...translatedMessage("by_channel_3bca83d786"))}
          </button>
          <button
            aria-pressed={view === "model"}
            onClick={() => switchView("model")}
          >
            <Boxes size={16} />
            {text(...translatedMessage("by_standard_model_38adc73787"))}
          </button>
        </div>
        <span className="channel-inventory-count">
          {text(
            ...translatedMessage("channels_instances_9430add3e6", {
              p0: snapshot.channels.length,
              p1: snapshot.instances.length,
            }),
          )}
        </span>
      </div>
      <div className="channel-split">
        <aside className="channel-index">
          <div className="channel-index-heading">
            <span>
              {view === "channel"
                ? text(...translatedMessage("channels_80408f5ebf"))
                : text(...translatedMessage("standard_models_c89f0ff4d7"))}
            </span>
            <span>
              {result.groups.filter((g) => !g.id.startsWith("__")).length}
            </span>
          </div>
          <label className="channel-index-search">
            <Search size={14} />
            <input
              aria-label={
                view === "channel"
                  ? text(...translatedMessage("search_channels_d050781403"))
                  : text(...translatedMessage("search_model_index_30eb49ca56"))
              }
              placeholder={
                view === "channel"
                  ? text(...translatedMessage("search_channels_d050781403"))
                  : text(...translatedMessage("search_name_or_id_010d37442e"))
              }
              value={indexSearch}
              onChange={(e) => setIndexSearch(e.target.value)}
            />
            {indexSearch && (
              <button
                aria-label={text(
                  ...translatedMessage("clear_index_search_a0d6e2afaf"),
                )}
                onClick={() => setIndexSearch("")}
              >
                <X size={13} />
              </button>
            )}
          </label>
          <div
            className="channel-index-list"
            ref={listRef}
            onKeyDown={listKeyboard}
          >
            <nav
              aria-label={
                view === "channel"
                  ? text(...translatedMessage("channel_index_427e1548f7"))
                  : text(...translatedMessage("model_index_a1dee1f330"))
              }
            >
              {visibleGroups.map((g) => {
                const active = selected?.id === g.id;
                const Icon =
                  g.id === "__unmapped__"
                    ? CircleAlert
                    : g.id === "__all__"
                      ? Layers3
                      : view === "channel"
                        ? Network
                        : Boxes;
                return (
                  <button
                    key={g.id}
                    type="button"
                    data-group-id={g.id}
                    aria-current={active ? "true" : undefined}
                    tabIndex={
                      active ||
                      (!visibleGroups.some(
                        (item) => item.id === selected?.id,
                      ) &&
                        g === visibleGroups[0])
                        ? 0
                        : -1
                    }
                    className={`channel-index-item ${active ? "is-selected" : ""} ${g.id === "__unmapped__" ? "is-unmapped" : ""}`}
                    onClick={() => choose(g.id)}
                  >
                    <span className="channel-index-icon">
                      <Icon size={17} />
                    </span>
                    <span className="channel-index-copy">
                      <strong title={g.name}>{groupName(g.id, g.name)}</strong>
                      <small title={g.subtitle}>
                        {g.id === "__all__"
                          ? text(
                              ...translatedMessage(
                                "all_resource_instances_df713a91f6",
                              ),
                            )
                          : g.id === "__unmapped__"
                            ? text(
                                ...translatedMessage(
                                  "awaiting_model_mapping_7176d56b0b",
                                ),
                              )
                            : view === "model"
                              ? g.id
                              : text(
                                  ...translatedMessage(
                                    "ready_to_serve_8c518d778a",
                                    { p0: g.ready },
                                  ),
                                )}
                      </small>
                    </span>
                    <span className="channel-index-number">{g.total}</span>
                  </button>
                );
              })}
              {!visibleGroups.length && (
                <p className="channel-index-empty">
                  {text(...translatedMessage("no_matching_entries_b654d04efa"))}
                </p>
              )}
            </nav>
          </div>
          <div className="channel-index-footer">
            <button type="button" onClick={onRemoved}>
              <Inbox size={15} />
              {text(...translatedMessage("removed_from_source_a60c5be193"))}
              <span>{removedCount}</span>
            </button>
          </div>
        </aside>
        <div className="channel-resource-panel">
          <div className="channel-mobile-index">
            <label>
              {view === "channel"
                ? text(...translatedMessage("choose_channel_fd331bd512"))
                : text(
                    ...translatedMessage("choose_standard_model_6e7fd191a2"),
                  )}
              <select
                aria-label={
                  view === "channel"
                    ? text(...translatedMessage("choose_channel_fd331bd512"))
                    : text(
                        ...translatedMessage(
                          "choose_standard_model_6e7fd191a2",
                        ),
                      )
                }
                value={selected?.id ?? "__all__"}
                onChange={(e) => choose(e.target.value)}
              >
                {result.groups.map((g) => (
                  <option key={g.id} value={g.id}>
                    {groupName(g.id, g.name)} ({g.total})
                  </option>
                ))}
              </select>
            </label>
            <button type="button" onClick={onRemoved}>
              {text(...translatedMessage("removed_sources_02c820feb7"))} (
              {removedCount})
            </button>
          </div>
          <header className="channel-context">
            <div className="channel-context-title">
              <span className="channel-context-icon">
                {view === "channel" ? (
                  <Network size={21} />
                ) : (
                  <Boxes size={21} />
                )}
              </span>
              <div>
                <h2>{contextName}</h2>
                <p>
                  {selectedAll
                    ? text(
                        ...translatedMessage(
                          "review_model_mapping_placement_and_source_sta_4fc47ed52a",
                        ),
                      )
                    : selected?.id === "__unmapped__"
                      ? text(
                          ...translatedMessage(
                            "map_a_standard_model_before_enabling_placemen_50eada2ba8",
                          ),
                        )
                      : selectedChannel
                        ? selectedChannel.provider
                        : selectedModel?.id}
                </p>
              </div>
            </div>
            {selectedModel && (
              <Button asChild variant="ghost" size="small">
                <Link
                  to={`/console/model-pricing?model=${encodeURIComponent(selectedModel.id)}`}
                >
                  {text(...translatedMessage("model_settings_66f4d98a52"))}
                  <ArrowUpRight size={14} />
                </Link>
              </Button>
            )}
            {selectedChannel && (
              <span
                className={`channel-source-pill ${selectedChannel.enabled ? "" : "is-off"}`}
              >
                <i />
                {selectedChannel.enabled
                  ? text(...translatedMessage("channel_enabled_d06eb06e13"))
                  : text(...translatedMessage("channel_disabled_a45b6201ca"))}
              </span>
            )}
          </header>
          {selectedId !== selected?.id && (
            <p className="channel-invalid-selection" role="status">
              {text(
                ...translatedMessage(
                  "the_previous_selection_is_unavailable_showing_d2fb47fb4f",
                ),
              )}
            </p>
          )}
          <div className="channel-resource-toolbar">
            <label className="channel-resource-search">
              <Search size={16} />
              <input
                aria-label={text(
                  ...translatedMessage("search_resource_instances_56bd6e03b5"),
                )}
                placeholder={text(
                  ...translatedMessage(
                    "search_instance_or_model_id_03b76eac60",
                  ),
                )}
                value={query}
                onChange={(e) =>
                  change({ query: e.target.value, page: null }, true)
                }
              />
              {query && (
                <button
                  onClick={() => change({ query: null, page: null }, true)}
                  aria-label={text(
                    ...translatedMessage("clear_resource_search_0fefe21ae6"),
                  )}
                >
                  <X size={14} />
                </button>
              )}
            </label>
            <div className="channel-filter-controls">
              <SlidersHorizontal size={15} />
              <select
                aria-label={text(
                  ...translatedMessage("filter_by_service_group_4fc7ad9f07"),
                )}
                value={groupId}
                onChange={(e) => change({ group: e.target.value, page: null })}
              >
                <option value="">
                  {text(...translatedMessage("all_groups_051837cdeb"))}
                </option>
                {groupId && !config.groups[groupId] && (
                  <option value={groupId}>
                    {text(...translatedMessage("group_removed_686b935703"))}
                  </option>
                )}
                {Object.entries(config.groups).map(([id, g]) => (
                  <option key={id} value={id}>
                    {g.name}
                  </option>
                ))}
              </select>
              <select
                aria-label={text(
                  ...translatedMessage("filter_by_resource_status_bab343f1db"),
                )}
                value={status}
                onChange={(e) =>
                  change({
                    status: e.target.value === "all" ? null : e.target.value,
                    page: null,
                  })
                }
              >
                {Object.entries(statusNames).map(([id, name]) => (
                  <option value={id} key={id}>
                    {name}
                  </option>
                ))}
              </select>
            </div>
          </div>
          {filtered && (
            <div className="channel-active-filters">
              <span>
                {text(
                  ...translatedMessage("matching_instances_4f4a5dbb3a", {
                    p0: result.filteredTotal,
                    p1: result.total,
                  }),
                )}
              </span>
              <button onClick={clearFilters}>
                {text(...translatedMessage("clear_filters_43c142eafa"))}
                <X size={12} />
              </button>
            </div>
          )}
          <div className="channel-rows-scroll" ref={rowsRef}>
            <table className="channel-resource-table">
              <thead>
                <tr>
                  <th>
                    {text(...translatedMessage("resource_instance_cc1b21a3ff"))}
                  </th>
                  {selectedAll && (
                    <th>
                      {view === "channel"
                        ? text(
                            ...translatedMessage("source_channel_e6d8b886ae"),
                          )
                        : text(
                            ...translatedMessage("standard_model_18b603bd24"),
                          )}
                    </th>
                  )}
                  <th>
                    {view === "channel"
                      ? text(...translatedMessage("standard_model_18b603bd24"))
                      : text(...translatedMessage("source_channel_e6d8b886ae"))}
                  </th>
                  <th>
                    {text(...translatedMessage("placement_groups_f2ad9a9265"))}
                  </th>
                  <th>{text(...translatedMessage("status_dda39dcd63"))}</th>
                  <th>{text(...translatedMessage("action_2a8e78b55a"))}</th>
                </tr>
              </thead>
              <tbody>
                {pageRows.map((r) => (
                  <tr key={r.id} data-resource-id={r.id}>
                    <td>
                      <button
                        className="channel-resource-name"
                        title={r.upstreamModel}
                        onClick={() => onEdit(r.id)}
                      >
                        {r.upstreamModel}
                      </button>
                      <code title={r.id}>{r.id}</code>
                    </td>
                    {selectedAll && (
                      <td>
                        <button
                          className="channel-related"
                          onClick={() => {
                            setIndexSearch("");
                            change(
                              view === "channel"
                                ? { channel: r.channelId, page: null }
                                : {
                                    model:
                                      config.placements?.[r.id]?.modelId ??
                                      "__unmapped__",
                                    page: null,
                                  },
                            );
                          }}
                        >
                          {view === "channel"
                            ? (snapshot.channels.find(
                                (c) => c.id === r.channelId,
                              )?.name ?? r.channelId)
                            : (models.find(
                                (m) =>
                                  m.id === config.placements?.[r.id]?.modelId,
                              )?.name ??
                              text(
                                ...translatedMessage("unmapped_6d198b7550"),
                              ))}
                        </button>
                      </td>
                    )}
                    <td>
                      <button
                        className={`channel-related ${!relatedId(r) ? "is-pending" : ""}`}
                        title={relatedName(r)}
                        onClick={() => showRelated(r)}
                      >
                        {relatedName(r)}
                        <ArrowUpRight size={11} />
                      </button>
                      {!selectedAll && view === "model" && (
                        <code>{r.channelId}</code>
                      )}
                    </td>
                    <td>{renderGroups(r)}</td>
                    <td>{renderState(r)}</td>
                    <td>
                      <Button size="small" onClick={() => onEdit(r.id)}>
                        {text(...translatedMessage("configure_b3a5bdb50a"))}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="channel-resource-cards">
              {pageRows.map((r) => (
                <article
                  className="channel-resource-card"
                  key={r.id}
                  data-resource-id={r.id}
                >
                  <div className="channel-card-heading">
                    <div>
                      <strong>{r.upstreamModel}</strong>
                      <code>{r.id}</code>
                    </div>
                    {renderState(r)}
                  </div>
                  <dl>
                    <div>
                      <dt>
                        {view === "channel"
                          ? text(
                              ...translatedMessage("standard_model_18b603bd24"),
                            )
                          : text(
                              ...translatedMessage("source_channel_e6d8b886ae"),
                            )}
                      </dt>
                      <dd>
                        <button
                          className="channel-related"
                          onClick={() => showRelated(r)}
                        >
                          {relatedName(r)}
                          <ArrowUpRight size={12} />
                        </button>
                      </dd>
                    </div>
                    {view === "channel" && (
                      <div>
                        <dt>
                          {text(
                            ...translatedMessage("source_channel_e6d8b886ae"),
                          )}
                        </dt>
                        <dd>
                          {
                            snapshot.channels.find((c) => c.id === r.channelId)
                              ?.name
                          }
                        </dd>
                      </div>
                    )}
                    {view === "model" && selectedAll && (
                      <div>
                        <dt>
                          {text(
                            ...translatedMessage("standard_model_18b603bd24"),
                          )}
                        </dt>
                        <dd>
                          <button
                            className="channel-related"
                            onClick={() =>
                              choose(
                                config.placements?.[r.id]?.modelId ??
                                  "__unmapped__",
                              )
                            }
                          >
                            {models.find(
                              (m) =>
                                m.id === config.placements?.[r.id]?.modelId,
                            )?.name ??
                              text(...translatedMessage("unmapped_6d198b7550"))}
                          </button>
                        </dd>
                      </div>
                    )}
                    <div>
                      <dt>{text(...translatedMessage("groups_03d4031d8b"))}</dt>
                      <dd>{renderGroups(r)}</dd>
                    </div>
                  </dl>
                  <Button size="small" onClick={() => onEdit(r.id)}>
                    {text(...translatedMessage("configure_b3a5bdb50a"))}
                  </Button>
                </article>
              ))}
            </div>
            {!pageRows.length && (
              <div className="channel-empty">
                <Inbox size={30} />
                <h3>
                  {filtered
                    ? text(
                        ...translatedMessage(
                          "no_matching_resources_45db4656e8",
                        ),
                      )
                    : text(...translatedMessage("no_resources_yet_be7813b165"))}
                </h3>
                <p>
                  {filtered
                    ? text(
                        ...translatedMessage(
                          "adjust_the_search_or_filters_saved_mappings_r_f11c5f461d",
                        ),
                      )
                    : selectedModel
                      ? text(
                          ...translatedMessage(
                            "this_model_has_no_linked_resources_map_an_ins_ddb63ff52b",
                          ),
                        )
                      : text(
                          ...translatedMessage(
                            "sync_source_resources_to_configure_model_mapp_0e27c51d01",
                          ),
                        )}
                </p>
                {filtered && (
                  <Button onClick={clearFilters}>
                    {text(...translatedMessage("clear_filters_43c142eafa"))}
                  </Button>
                )}
              </div>
            )}
          </div>
          <footer className="channel-pagination">
            <span>
              {result.filteredTotal
                ? text(
                    ...translatedMessage("of_instances_efc2bae166", {
                      p0: (page - 1) * pageSize + 1,
                      p1: Math.min(page * pageSize, result.filteredTotal),
                      p2: result.filteredTotal,
                    }),
                  )
                : text(...translatedMessage("0_instances_03f79aff73"))}
            </span>
            <div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={text(
                  ...translatedMessage("previous_resources_101e4f6cf4"),
                )}
                disabled={page <= 1}
                onClick={() =>
                  change({ page: page - 1 === 1 ? null : String(page - 1) })
                }
              >
                <ChevronLeft size={16} />
              </Button>
              <span>
                {page} / {pageCount}
              </span>
              <Button
                variant="ghost"
                size="icon"
                aria-label={text(
                  ...translatedMessage("next_resources_89fb990bd8"),
                )}
                disabled={page >= pageCount}
                onClick={() => change({ page: String(page + 1) })}
              >
                <ChevronRight size={16} />
              </Button>
            </div>
          </footer>
        </div>
      </div>
    </section>
  );
}
