import { message as translatedMessage } from "../../i18n/messages";
import {
  useEffect,
  useRef,
  useState,
  type MouseEvent,
  type PointerEvent,
} from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  Search,
  Cpu,
  ArrowUpRight,
  Sparkles,
  LayoutGrid,
  List,
  Info,
  X,
} from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { Button } from "../../components/ui/button";
import { type DirectoryModel } from "../../config/model-directory";
import { usePreferences } from "../../lib/preferences";
import { useModelPricing } from "../../lib/model-pricing";
import { catalogVendor } from "../../lib/catalog-browser";
import { CatalogPriceSummary } from "../../components/catalog-price-summary";
import { ModelIdCopy } from "../../components/model-id-copy";
import { type PlazaQuote } from "../../lib/plaza-pricing";
import {
  usePlazaModels,
  usePlazaFilters,
  usePlazaRank,
  plazaToDirectoryModel,
  resolvePlazaVendor,
  type PlazaRankType,
} from "../../lib/api/plaza";
import "../../components/model-pricing.css";
import "../../components/access-center.css";
import "../../components/catalog-browser.css";

type CatalogView = "cards" | "list";
const viewPreference = "veyra-catalog-view-v1";
function savedView(): CatalogView {
  try {
    return localStorage.getItem(viewPreference) === "list" ? "list" : "cards";
  } catch {
    return "cards";
  }
}

/** Keep the real detail link for keyboard/browser actions and selectable copy. */
function useCatalogDetailTarget() {
  const gesture = useRef<{
    owner: HTMLElement;
    x: number;
    y: number;
    moved: boolean;
  }>();
  const interactiveTarget = (event: MouseEvent<HTMLElement>) =>
    event.target instanceof Element
      ? event.target.closest(
          "a, button, input, select, textarea, summary, [role='button'], [role='link'], [contenteditable='true']",
        )
      : null;
  const guardNavigation = (event: MouseEvent<HTMLElement>) => {
    const interactive = interactiveTarget(event);
    if (interactive && !interactive.matches(".catalog-model-name")) return;
    // Enter on a focused link is explicit navigation, even with selected text.
    if (interactive && event.detail === 0) return;
    const selection = window.getSelection();
    const selecting = Boolean(
      selection &&
      !selection.isCollapsed &&
      Array.from({ length: selection.rangeCount }, (_, i) =>
        selection.getRangeAt(i),
      ).some((range) => range.intersectsNode(event.currentTarget)),
    );
    const dragging =
      event.detail > 0 &&
      gesture.current?.owner === event.currentTarget &&
      gesture.current.moved;
    if (selecting || dragging) event.preventDefault();
  };
  const activateDetail = (event: MouseEvent<HTMLElement>) => {
    if (
      event.defaultPrevented ||
      interactiveTarget(event) ||
      (event.button !== 0 && event.button !== 1)
    )
      return;
    const link = event.currentTarget.querySelector<HTMLAnchorElement>(
      ".catalog-model-name",
    );
    if (!link) return;
    if (event.button === 1) {
      // Browsers do not activate an anchor for a synthetic auxclick. Open its
      // real href synchronously while this trusted middle click is active.
      event.preventDefault();
      window.open(link.href, "_blank", "noopener");
      return;
    }
    // Dispatch to the actual anchor so modifier clicks retain link behavior.
    link.dispatchEvent(
      new window.MouseEvent(event.type, {
        bubbles: true,
        cancelable: true,
        view: window,
        button: event.button,
        ctrlKey: event.ctrlKey,
        metaKey: event.metaKey,
        shiftKey: event.shiftKey,
        altKey: event.altKey,
      }),
    );
  };
  return {
    onPointerDownCapture: (event: PointerEvent<HTMLElement>) => {
      gesture.current = {
        owner: event.currentTarget,
        x: event.clientX,
        y: event.clientY,
        moved: false,
      };
    },
    onPointerMoveCapture: (event: PointerEvent<HTMLElement>) => {
      if (
        gesture.current?.owner === event.currentTarget &&
        Math.hypot(
          event.clientX - gesture.current.x,
          event.clientY - gesture.current.y,
        ) > 6
      ) {
        gesture.current.moved = true;
      }
    },
    onPointerCancelCapture: () => {
      if (gesture.current) gesture.current.moved = true;
    },
    onClickCapture: guardNavigation,
    onAuxClickCapture: guardNavigation,
    onClick: activateDetail,
    onAuxClick: activateDetail,
  };
}

export default function ModelSquare() {
  const detailTargetEvents = useCatalogDetailTarget();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const pricing = useModelPricing((s) => s.config);
  const [params, setParams] = useSearchParams();
  const [initialView] = useState(savedView);
  const viewParam = params.get("view");
  const view: CatalogView =
    viewParam === "list" || viewParam === "cards" ? viewParam : initialView;
  useEffect(() => {
    if (viewParam === "list" || viewParam === "cards") return;
    const next = new URLSearchParams(params);
    next.set("view", initialView);
    setParams(next, { replace: true });
  }, [params, setParams, viewParam, initialView]);
  const search = params.get("q") ?? "";
  const vendorParam = params.get("vendor") ?? "";
  const plazaFilters = usePlazaFilters();
  const vendor = resolvePlazaVendor(
    vendorParam,
    plazaFilters.data?.vendors ?? [],
  );
  useEffect(() => {
    if (vendor === vendorParam) return;
    const next = new URLSearchParams(params);
    next.set("vendor", vendor);
    setParams(next, { replace: true });
  }, [params, setParams, vendor, vendorParam]);
  const plazaQuery = usePlazaModels(
    {
      q: search || undefined,
      vendors: vendor || undefined,
      page_size: 200,
    },
    !vendorParam || Boolean(plazaFilters.data) || plazaFilters.isError,
  );
  const models = plazaQuery.isError
    ? []
    : (plazaQuery.data?.items ?? []).map(plazaToDirectoryModel);
  const vendors = plazaFilters.data?.vendors ?? [];
  const catalogTotal = plazaFilters.data?.total ?? plazaQuery.data?.total ?? 0;
  const [rankType, setRankType] = useState<PlazaRankType>("value");
  const rankQuery = usePlazaRank(rankType);
  const updateFilters = (values: Record<string, string>, replace = false) => {
    const next = new URLSearchParams(params);
    // Record view explicitly when navigating so refresh and return are exact.
    next.set("view", view);
    for (const [key, value] of Object.entries(values)) {
      if (!value) next.delete(key);
      else next.set(key, value);
    }
    setParams(next, { replace });
  };
  const setView = (next: CatalogView) => {
    try {
      localStorage.setItem(viewPreference, next);
    } catch {
      /* Optional preference. */
    }
    updateFilters({ view: next });
  };
  const returnParams = new URLSearchParams(params);
  returnParams.set("view", view);
  const returnTo = `/models?${returnParams.toString()}`;
  const detailUrl = (model: DirectoryModel) =>
    `/models/${encodeURIComponent(model.id)}?from=${encodeURIComponent(returnTo)}`;
  const vendorLabel = (value: string) =>
    value === "Veyra"
      ? text(...translatedMessage("veyra_platform_02fbaf741d"))
      : value;
  const capabilities: Record<string, string> = {
    text: text(...translatedMessage("text_0c246fb7e5")),
    code: text(...translatedMessage("code_6980ed049f")),
    vision: text(...translatedMessage("vision_2bb5870e7d")),
    reasoning: text(...translatedMessage("reasoning_bc2a4ebae1")),
    image: text(...translatedMessage("image_66fb7dd3a1")),
    video: text(...translatedMessage("video_e2fb3c780b")),
  };
  const renderKind = (model: DirectoryModel) =>
    model.kind === "smart" ? (
      <span className="access-kind catalog-smart-label">
        {text(...translatedMessage("smart_routing_34b9b06792"))}
      </span>
    ) : !model.protocols.length ? (
      <span className="access-kind catalog-specific-label">
        {text(...translatedMessage("client_connection_unavailable_be1d46f21e"))}
      </span>
    ) : null;
  const renderCapabilities = (model: DirectoryModel) => (
    <div className="catalog-capabilities">
      {model.capabilities.map((c) => (
        <span key={c}>{capabilities[c] ?? c}</span>
      ))}
    </div>
  );
  const quoteGroup = plazaQuery.data?.group ?? plazaFilters.data?.group ?? "";
  const entries = models.map((model) => ({
    model,
    quote: { model, group: quoteGroup } satisfies PlazaQuote,
  }));
  const renderPrice = ({ model, quote }: (typeof entries)[number]) => (
    <CatalogPriceSummary model={model} config={pricing} plazaQuote={quote} />
  );
  const renderAction = ({ model }: (typeof entries)[number]) => {
    const canConnect = model.online && model.protocols.length > 0;
    return (
      <Link
        className="catalog-try"
        to={
          canConnect
            ? `/console/connect?model=${encodeURIComponent(model.id)}`
            : detailUrl(model)
        }
      >
        {canConnect
          ? text(...translatedMessage("connect_model_84185df670"))
          : text(...translatedMessage("view_status_42390b743c"))}
        <ArrowUpRight size={13} />
      </Link>
    );
  };
  return (
    <div className="brand-home public-refined catalog-refined catalog-browser">
      <PublicHeader />
      <main className="models-page">
        <section className="catalog-heading">
          <div>
            <h1>
              {text(...translatedMessage("find_your_next_model_9cc7c25265"))}
            </h1>
            <p>
              {text(
                ...translatedMessage(
                  "one_key_switch_models_in_the_tools_you_alread_6fa498495f",
                ),
              )}
            </p>
          </div>
          <Link className="catalog-guide" to="/console/connect">
            <span className="catalog-guide-icon">
              <Cpu size={24} />
            </span>
            <span>
              <strong>
                {text(...translatedMessage("connect_a_tool_20565c835b"))}
              </strong>
              <small>
                {text(
                  ...translatedMessage(
                    "reuse_a_key_and_get_tool_configuration_b5fbe8dd14",
                  ),
                )}
              </small>
            </span>
            <ArrowUpRight size={18} />
          </Link>
        </section>
        <div
          className="catalog-tabs catalog-vendor-tabs"
          role="group"
          aria-label={text(...translatedMessage("filter_by_vendor_08dffcaea2"))}
        >
          {[{ value: "", label: "", count: catalogTotal }, ...vendors].map(
            (item) => (
              <button
                type="button"
                key={item.value}
                data-catalog-vendor={
                  item.value === "all" ? "vendor:all" : item.value || "all"
                }
                className={vendor === item.value ? "is-active" : ""}
                aria-pressed={vendor === item.value}
                onClick={() => updateFilters({ vendor: item.value })}
              >
                {!item.value
                  ? text(...translatedMessage("all_vendors_2beb299061"))
                  : vendorLabel(item.label || item.value)}
                <span>{item.count}</span>
              </button>
            ),
          )}
        </div>
        <div className="catalog-toolbar">
          <label className="catalog-search">
            <Search size={17} aria-hidden="true" />
            <input
              type="search"
              aria-label={text(
                ...translatedMessage("search_models_767e4edbf7"),
              )}
              placeholder={text(
                ...translatedMessage(
                  "search_name_model_id_or_vendor_7ca5ed9aa5",
                ),
              )}
              value={search}
              onChange={(e) => updateFilters({ q: e.target.value }, true)}
            />
            {search && (
              <button
                type="button"
                aria-label={text(
                  ...translatedMessage("clear_search_99d00f5436"),
                )}
                onClick={() => updateFilters({ q: "" }, true)}
              >
                <X size={15} />
              </button>
            )}
          </label>
          <div className="catalog-toolbar-end">
            <span className="catalog-result-count" role="status">
              {plazaQuery.isPending
                ? text(...translatedMessage("loading_a4e12025bb"))
                : plazaQuery.isError
                  ? text(...translatedMessage("load_failed_36068d34aa"))
                  : `${models.length} ${text(...translatedMessage("models_52c071580a"))}`}
            </span>
            <div
              className="catalog-view-switch"
              role="group"
              aria-label={text(...translatedMessage("display_mode_f20af7121b"))}
            >
              <button
                type="button"
                data-catalog-view="cards"
                aria-pressed={view === "cards"}
                onClick={() => setView("cards")}
              >
                <LayoutGrid size={16} />
                <span>{text(...translatedMessage("cards_ea74f92a8a"))}</span>
              </button>
              <button
                type="button"
                data-catalog-view="list"
                aria-pressed={view === "list"}
                onClick={() => setView("list")}
              >
                <List size={17} />
                <span>{text(...translatedMessage("list_fac72a3200"))}</span>
              </button>
            </div>
          </div>
        </div>
        <div className="catalog-price-context">
          <Info size={14} aria-hidden="true" />
          <p>
            {text(
              ...translatedMessage(
                "platform_quotes_are_for_backend_group_with_it_7a4ac5f3b9",
                { p0: quoteGroup || "—" },
              ),
            )}
          </p>
        </div>
        {plazaQuery.isPending || plazaQuery.isError || !models.length ? (
          <div
            className="catalog-empty"
            role={plazaQuery.isError ? "alert" : "status"}
            aria-busy={plazaQuery.isPending}
          >
            <Cpu size={36} />
            <h2>
              {plazaQuery.isPending
                ? text(...translatedMessage("loading_models_ac854a2d56"))
                : plazaQuery.isError
                  ? text(
                      ...translatedMessage("failed_to_load_models_1d0672f449"),
                    )
                  : text(...translatedMessage("no_matching_models_ab6a7f3e93"))}
            </h2>
            <p>
              {plazaQuery.isPending
                ? text(
                    ...translatedMessage(
                      "fetching_the_model_catalog_please_wait_de673b3c9b",
                    ),
                  )
                : plazaQuery.isError
                  ? text(
                      ...translatedMessage(
                        "the_model_catalog_is_unavailable_please_retry_a82d31d6dd",
                      ),
                    )
                  : text(
                      ...translatedMessage(
                        "try_another_vendor_or_search_term_37f1f0037a",
                      ),
                    )}
            </p>
            {!plazaQuery.isPending && (
              <Button
                variant="secondary"
                onClick={() => {
                  if (plazaQuery.isError) {
                    void plazaQuery.refetch();
                    if (plazaFilters.isError) void plazaFilters.refetch();
                  } else updateFilters({ q: "", vendor: "" });
                }}
              >
                {plazaQuery.isError
                  ? text(...translatedMessage("retry_c16455021c"))
                  : text(...translatedMessage("clear_filters_fb9b992311"))}
              </Button>
            )}
          </div>
        ) : view === "cards" ? (
          <div className="catalog-grid">
            {entries.map((entry) => {
              const { model } = entry;
              return (
                <article
                  className="catalog-card"
                  key={model.id}
                  data-model-id={model.id}
                  {...detailTargetEvents}
                >
                  <div className="catalog-card-top">
                    <span className="catalog-vendor-mark" aria-hidden="true">
                      {catalogVendor(model) === "Veyra" ? (
                        <Sparkles size={22} />
                      ) : (
                        catalogVendor(model).slice(0, 1)
                      )}
                    </span>
                    <span className="catalog-vendor">
                      {vendorLabel(catalogVendor(model))}
                    </span>
                    {renderKind(model)}
                  </div>
                  <Link className="catalog-model-name" to={detailUrl(model)}>
                    <h2>{model.name}</h2>
                    <ArrowUpRight size={17} />
                  </Link>
                  <ModelIdCopy id={model.id} />
                  <div className="catalog-model-meta">
                    {renderCapabilities(model)}
                    {model.protocols.length > 0 && model.kind !== "smart" && (
                      <span
                        className="catalog-context-info"
                        title={text(
                          ...translatedMessage("demo_context_d2808a6c99"),
                        )}
                      >
                        <bdi>{model.contextDisplay}</bdi>
                      </span>
                    )}
                  </div>
                  <div className="catalog-card-bottom">
                    {renderPrice(entry)}
                  </div>
                  {renderAction(entry)}
                </article>
              );
            })}
          </div>
        ) : (
          <div
            className="catalog-list"
            role="table"
            aria-label={text(...translatedMessage("model_list_aba1fb7a46"))}
          >
            <div className="catalog-list-head" role="row">
              <div role="columnheader">
                {text(...translatedMessage("model_id_aa489331aa"))}
              </div>
              <div role="columnheader">
                {text(...translatedMessage("capabilities_context_4521df5a70"))}
              </div>
              <div role="columnheader">
                {text(...translatedMessage("price_comparison_cea6ad3d85"))}
              </div>
              <div role="columnheader">
                {text(...translatedMessage("actions_d83f4ed14a"))}
              </div>
            </div>
            {entries.map((entry) => {
              const { model } = entry;
              return (
                <div
                  className="catalog-list-row"
                  role="row"
                  key={model.id}
                  data-model-id={model.id}
                  {...detailTargetEvents}
                >
                  <div className="catalog-list-identity" role="cell">
                    <div className="catalog-list-vendor">
                      <span>{vendorLabel(catalogVendor(model))}</span>
                      {renderKind(model)}
                    </div>
                    <Link className="catalog-model-name" to={detailUrl(model)}>
                      <span>{model.name}</span>
                      <ArrowUpRight size={14} />
                    </Link>
                    <ModelIdCopy id={model.id} />
                  </div>
                  <div className="catalog-list-capabilities" role="cell">
                    {renderCapabilities(model)}
                    {model.protocols.length > 0 && model.kind !== "smart" && (
                      <div className="catalog-context-info">
                        {text(...translatedMessage("context_9810531841"))}{" "}
                        <bdi>{model.contextDisplay}</bdi>
                      </div>
                    )}
                  </div>
                  <div className="catalog-list-price" role="cell">
                    {renderPrice(entry)}
                  </div>
                  <div className="catalog-list-actions" role="cell">
                    {renderAction(entry)}
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* Model ranking */}
        <section
          className="catalog-ranking"
          style={{ marginTop: "var(--space-8)" }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "var(--space-3)",
              marginBottom: "var(--space-4)",
            }}
          >
            <Sparkles size={18} />
            <h2
              style={{
                margin: 0,
                fontSize: "var(--text-lg)",
                fontWeight: "var(--weight-bold)",
              }}
            >
              {text(...translatedMessage("model_ranking_e8b97d360b"))}
            </h2>
            <div style={{ display: "flex", gap: 4 }}>
              {(
                [
                  {
                    id: "value",
                    label: text(...translatedMessage("value_e035bd0136")),
                  },
                  {
                    id: "hot",
                    label: text(...translatedMessage("popular_ba11f0a9f0")),
                  },
                  {
                    id: "new",
                    label: text(...translatedMessage("newest_61c766d0ce")),
                  },
                ] as { id: PlazaRankType; label: string }[]
              ).map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => setRankType(tab.id)}
                  style={{
                    padding: "4px 10px",
                    fontSize: "var(--text-xs)",
                    fontWeight: rankType === tab.id ? 600 : 400,
                    background:
                      rankType === tab.id ? "var(--brand)" : "var(--subtle)",
                    color: rankType === tab.id ? "#fff" : "var(--secondary)",
                    border: "none",
                    borderRadius: "var(--radius-sm)",
                    cursor: "pointer",
                  }}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          </div>
          {rankQuery.isLoading ? (
            <p style={{ color: "var(--muted)", fontSize: "var(--text-sm)" }}>
              {text(...translatedMessage("loading_4c20385375"))}
            </p>
          ) : rankQuery.data?.items?.length ? (
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
                gap: "var(--space-3)",
              }}
            >
              {rankQuery.data.items.map((item) => (
                <Link
                  key={item.model.id}
                  to={`/models/${encodeURIComponent(item.model.id)}`}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: "var(--space-3)",
                    padding: "var(--space-3) var(--space-4)",
                    background: "var(--surface)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-md)",
                    textDecoration: "none",
                    color: "inherit",
                  }}
                >
                  <span
                    className="mono"
                    style={{
                      fontSize: "var(--text-lg)",
                      fontWeight: 700,
                      color: item.rank <= 3 ? "var(--brand)" : "var(--muted)",
                      minWidth: 28,
                    }}
                  >
                    #{item.rank}
                  </span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div
                      style={{ fontWeight: 600, fontSize: "var(--text-sm)" }}
                    >
                      {item.model.name}
                    </div>
                    <div
                      style={{
                        fontSize: "var(--text-xs)",
                        color: "var(--muted)",
                      }}
                    >
                      {item.model.vendor_display || item.model.vendor}
                    </div>
                  </div>
                  <div style={{ textAlign: "end" }}>
                    <div
                      style={{
                        fontSize: "var(--text-xs)",
                        color: "var(--muted)",
                      }}
                    >
                      {item.metric_label}
                    </div>
                    <div
                      className="mono"
                      style={{
                        fontSize: "var(--text-sm)",
                        fontWeight: 600,
                        color: "var(--brand)",
                      }}
                    >
                      {item.metric_value}
                    </div>
                  </div>
                </Link>
              ))}
            </div>
          ) : (
            <p style={{ color: "var(--muted)", fontSize: "var(--text-sm)" }}>
              {text(...translatedMessage("no_ranking_data_06785284ff"))}
            </p>
          )}
        </section>
      </main>
    </div>
  );
}
