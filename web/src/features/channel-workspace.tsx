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
import { useSettingsText } from "./pricing-settings-shared";
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
        ? text("全部渠道", "All channels", "كل القنوات")
        : text("全部标准模型", "All standard models", "كل النماذج القياسية")
      : id === "__unmapped__"
        ? text("未映射资源", "Unmapped resources", "موارد غير مرتبطة")
        : name;
  const statusNames = {
    all: text("全部状态", "All statuses", "كل الحالات"),
    ready: text("可供调用", "Ready to serve", "متاح للطلبات"),
    unmapped: text("待映射", "Needs mapping", "يحتاج ربطاً"),
    paused: text("投放暂停", "Placement paused", "النشر متوقف"),
    unavailable: text("来源异常", "Source unavailable", "المصدر غير متاح"),
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
    : text("资源实例", "Resource instances", "الموارد");
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
                    `收起 ${r.id} 的分组`,
                    `Collapse groups for ${r.id}`,
                    `طي مجموعات ${r.id}`,
                  )
                : text(
                    `展开 ${r.id} 的全部投放分组`,
                    `Show all groups for ${r.id}`,
                    `عرض مجموعات ${r.id}`,
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
            {expanded ? text("收起", "Less", "أقل") : `+${ids.length - 2}`}
          </button>
        )}
      </div>
    ) : (
      <span className="channel-muted">
        {text("尚未投放", "Not placed", "لم يُنشر")}
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
              ? text("模型已停用", "Model disabled", "النموذج معطل")
              : state.label === "分组未启用"
                ? text("分组未启用", "Groups disabled", "المجموعات معطلة")
                : text("尚未投放", "Not placed", "لم يُنشر")}
        </span>
        <small>
          {sourceOk
            ? text("来源可用", "Source healthy", "المصدر متاح")
            : r.status === "retired"
              ? text("来源已下架", "Source retired", "أزيل من المصدر")
              : text("来源不可用", "Source unavailable", "المصدر غير متاح")}
          {!sourceOk && config.placements?.[r.id]?.enabled
            ? text(" · 投放配置保留", " · Placement retained", " · النشر محفوظ")
            : ""}
        </small>
      </div>
    );
  };
  const relatedName = (r: ResourceInstance) =>
    view === "channel"
      ? (models.find((m) => m.id === config.placements?.[r.id]?.modelId)
          ?.name ?? text("未映射", "Unmapped", "غير مرتبط"))
      : (snapshot.channels.find((c) => c.id === r.channelId)?.name ??
        r.channelId);
  const relatedId = (r: ResourceInstance) =>
    view === "channel" ? config.placements?.[r.id]?.modelId : r.channelId;
  return (
    <section
      className="channel-workspace"
      aria-label={text(
        "资源管理工作区",
        "Resource workspace",
        "مساحة إدارة الموارد",
      )}
    >
      <div className="channel-view-bar">
        <div
          className="channel-view-switch"
          role="group"
          aria-label={text(
            "资源查看方式",
            "Resource view",
            "طريقة عرض الموارد",
          )}
        >
          <button
            aria-pressed={view === "channel"}
            onClick={() => switchView("channel")}
          >
            <Network size={16} />
            {text("按渠道", "By channel", "حسب القناة")}
          </button>
          <button
            aria-pressed={view === "model"}
            onClick={() => switchView("model")}
          >
            <Boxes size={16} />
            {text("按标准模型", "By standard model", "حسب النموذج")}
          </button>
        </div>
        <span className="channel-inventory-count">
          {text(
            `${snapshot.channels.length} 个渠道 · ${snapshot.instances.length} 个资源实例`,
            `${snapshot.channels.length} channels · ${snapshot.instances.length} instances`,
            `${snapshot.channels.length} قنوات · ${snapshot.instances.length} موارد`,
          )}
        </span>
      </div>
      <div className="channel-split">
        <aside className="channel-index">
          <div className="channel-index-heading">
            <span>
              {view === "channel"
                ? text("渠道目录", "Channels", "القنوات")
                : text("标准模型目录", "Standard models", "النماذج القياسية")}
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
                  ? text("搜索渠道", "Search channels", "بحث القنوات")
                  : text(
                      "搜索标准模型目录",
                      "Search model index",
                      "بحث النماذج",
                    )
              }
              placeholder={
                view === "channel"
                  ? text("搜索渠道", "Search channels", "بحث القنوات")
                  : text(
                      "搜索名称或标识",
                      "Search name or ID",
                      "بحث الاسم أو المعرّف",
                    )
              }
              value={indexSearch}
              onChange={(e) => setIndexSearch(e.target.value)}
            />
            {indexSearch && (
              <button
                aria-label={text(
                  "清空目录搜索",
                  "Clear index search",
                  "مسح البحث",
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
                  ? text("渠道目录", "Channel index", "دليل القنوات")
                  : text("标准模型目录", "Model index", "دليل النماذج")
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
                              "查看所有来源实例",
                              "All resource instances",
                              "كل موارد المصدر",
                            )
                          : g.id === "__unmapped__"
                            ? text(
                                "等待绑定标准模型",
                                "Awaiting model mapping",
                                "بانتظار ربط النموذج",
                              )
                            : view === "model"
                              ? g.id
                              : text(
                                  `${g.ready} 个可供调用`,
                                  `${g.ready} ready to serve`,
                                  `${g.ready} جاهز للطلبات`,
                                )}
                      </small>
                    </span>
                    <span className="channel-index-number">{g.total}</span>
                  </button>
                );
              })}
              {!visibleGroups.length && (
                <p className="channel-index-empty">
                  {text(
                    "没有匹配的目录项",
                    "No matching entries",
                    "لا توجد نتائج",
                  )}
                </p>
              )}
            </nav>
          </div>
          <div className="channel-index-footer">
            <button type="button" onClick={onRemoved}>
              <Inbox size={15} />
              {text("来源已移除", "Removed from source", "أزيل من المصدر")}
              <span>{removedCount}</span>
            </button>
          </div>
        </aside>
        <div className="channel-resource-panel">
          <div className="channel-mobile-index">
            <label>
              {view === "channel"
                ? text("选择渠道", "Choose channel", "اختر القناة")
                : text("选择标准模型", "Choose standard model", "اختر النموذج")}
              <select
                aria-label={
                  view === "channel"
                    ? text("选择渠道", "Choose channel", "اختر القناة")
                    : text(
                        "选择标准模型",
                        "Choose standard model",
                        "اختر النموذج",
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
              {text("来源已移除", "Removed sources", "المصادر المحذوفة")} (
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
                        "集中查看资源映射、销售投放与来源状态",
                        "Review model mapping, placement and source status",
                        "راجع ربط النماذج والنشر وحالة المصدر",
                      )
                    : selected?.id === "__unmapped__"
                      ? text(
                          "映射标准模型后，才能配置对客投放",
                          "Map a standard model before enabling placement",
                          "اربط نموذجاً قياسياً قبل النشر",
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
                  {text("模型设置", "Model settings", "إعدادات النموذج")}
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
                  ? text("渠道启用", "Channel enabled", "القناة مفعلة")
                  : text("渠道停用", "Channel disabled", "القناة معطلة")}
              </span>
            )}
          </header>
          {selectedId !== selected?.id && (
            <p className="channel-invalid-selection" role="status">
              {text(
                "原选择已不在当前目录，已显示全部资源。",
                "The previous selection is unavailable. Showing all resources.",
                "الاختيار السابق غير متاح؛ تظهر كل الموارد.",
              )}
            </p>
          )}
          <div className="channel-resource-toolbar">
            <label className="channel-resource-search">
              <Search size={16} />
              <input
                aria-label={text(
                  "搜索资源实例",
                  "Search resource instances",
                  "بحث الموارد",
                )}
                placeholder={text(
                  "搜索实例或模型标识",
                  "Search instance or model ID",
                  "بحث المورد أو معرّف النموذج",
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
                    "清空资源搜索",
                    "Clear resource search",
                    "مسح بحث الموارد",
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
                  "按投放分组筛选",
                  "Filter by service group",
                  "تصفية حسب مجموعة الخدمة",
                )}
                value={groupId}
                onChange={(e) => change({ group: e.target.value, page: null })}
              >
                <option value="">
                  {text("全部分组", "All groups", "كل المجموعات")}
                </option>
                {groupId && !config.groups[groupId] && (
                  <option value={groupId}>
                    {text("分组已移除", "Group removed", "المجموعة محذوفة")}
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
                  "按资源状态筛选",
                  "Filter by resource status",
                  "تصفية حسب الحالة",
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
                  `当前范围匹配 ${result.filteredTotal} / ${result.total} 个实例`,
                  `${result.filteredTotal} / ${result.total} matching instances`,
                  `${result.filteredTotal} / ${result.total} موارد مطابقة`,
                )}
              </span>
              <button onClick={clearFilters}>
                {text("清空筛选", "Clear filters", "مسح الفلاتر")}
                <X size={12} />
              </button>
            </div>
          )}
          <div className="channel-rows-scroll" ref={rowsRef}>
            <table className="channel-resource-table">
              <thead>
                <tr>
                  <th>{text("资源实例", "Resource instance", "المورد")}</th>
                  {selectedAll && (
                    <th>
                      {view === "channel"
                        ? text("来源渠道", "Source channel", "قناة المصدر")
                        : text("标准模型", "Standard model", "النموذج القياسي")}
                    </th>
                  )}
                  <th>
                    {view === "channel"
                      ? text("标准模型", "Standard model", "النموذج القياسي")
                      : text("来源渠道", "Source channel", "قناة المصدر")}
                  </th>
                  <th>
                    {text("投放分组", "Placement groups", "مجموعات النشر")}
                  </th>
                  <th>{text("状态", "Status", "الحالة")}</th>
                  <th>{text("操作", "Action", "الإجراء")}</th>
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
                              text("未映射", "Unmapped", "غير مرتبط"))}
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
                        {text("映射与投放", "Configure", "إعداد النشر")}
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
                              "标准模型",
                              "Standard model",
                              "النموذج القياسي",
                            )
                          : text("来源渠道", "Source channel", "قناة المصدر")}
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
                          {text("来源渠道", "Source channel", "قناة المصدر")}
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
                            "标准模型",
                            "Standard model",
                            "النموذج القياسي",
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
                            )?.name ?? text("未映射", "Unmapped", "غير مرتبط")}
                          </button>
                        </dd>
                      </div>
                    )}
                    <div>
                      <dt>{text("投放分组", "Groups", "المجموعات")}</dt>
                      <dd>{renderGroups(r)}</dd>
                    </div>
                  </dl>
                  <Button size="small" onClick={() => onEdit(r.id)}>
                    {text("映射与投放", "Configure", "إعداد النشر")}
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
                        "没有匹配的资源实例",
                        "No matching resources",
                        "لا موارد مطابقة",
                      )
                    : text(
                        "暂时没有资源实例",
                        "No resources yet",
                        "لا توجد موارد بعد",
                      )}
                </h3>
                <p>
                  {filtered
                    ? text(
                        "调整当前范围的搜索或筛选，不会改变已有映射。",
                        "Adjust the search or filters. Saved mappings remain unchanged.",
                        "عدّل البحث أو الفلاتر دون تغيير الربط المحفوظ.",
                      )
                    : selectedModel
                      ? text(
                          "该标准模型尚未关联资源实例，可从渠道视角配置映射。",
                          "This model has no linked resources. Map an instance from the channel view.",
                          "لا موارد مرتبطة بهذا النموذج. اربط مورداً من عرض القنوات.",
                        )
                      : text(
                          "同步上游资源后，可在这里完成模型映射与投放。",
                          "Sync source resources to configure model mapping and placement here.",
                          "زامن موارد المصدر لإعداد الربط والنشر هنا.",
                        )}
                </p>
                {filtered && (
                  <Button onClick={clearFilters}>
                    {text("清空筛选", "Clear filters", "مسح الفلاتر")}
                  </Button>
                )}
              </div>
            )}
          </div>
          <footer className="channel-pagination">
            <span>
              {result.filteredTotal
                ? text(
                    `显示 ${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, result.filteredTotal)} / ${result.filteredTotal} 个实例`,
                    `${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, result.filteredTotal)} of ${result.filteredTotal} instances`,
                    `${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, result.filteredTotal)} من ${result.filteredTotal}`,
                  )
                : text("0 个实例", "0 instances", "0 موارد")}
            </span>
            <div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={text(
                  "上一页资源",
                  "Previous resources",
                  "الموارد السابقة",
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
                  "下一页资源",
                  "Next resources",
                  "الموارد التالية",
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
