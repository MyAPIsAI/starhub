import "./public-catalog.css";
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
  plazaToDirectoryModel,
  resolvePlazaVendor,
} from "../../lib/api/plaza";
import "../../components/model-pricing.css";
import "../../components/access-center.css";
import "../../components/catalog-browser.css";
import "./catalog-refresh.css";
import { catalogText } from "./public-content";

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
  const copy = catalogText(text);
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
  const capability = params.get("capability") ?? "";
  const pageNumber = Math.max(1, Math.floor(Number(params.get("page")) || 1));
  const plazaFilters = usePlazaFilters(true);
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
      tags: capability || undefined,
      scope: "public",
      page: pageNumber > 1 ? pageNumber : undefined,
      page_size: 200,
    },
    !vendorParam || Boolean(plazaFilters.data) || plazaFilters.isError,
  );
  const models = plazaQuery.isError
    ? []
    : (plazaQuery.data?.items ?? []).map(plazaToDirectoryModel);
  const vendors = plazaFilters.data?.vendors ?? [];
  const catalogTotal = plazaFilters.data?.total ?? plazaQuery.data?.total ?? 0;
  const updateFilters = (values: Record<string, string>, replace = false) => {
    const next = new URLSearchParams(params);
    // Record view explicitly when navigating so refresh and return are exact.
    next.set("view", view);
    if (["q", "vendor", "capability"].some((key) => key in values))
      next.delete("page");
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
  const vendorLabel = (value: string) => value;
  const capabilities: Record<string, string> = {
    audio: copy.audio,
    embedding: copy.embedding,
    rerank: copy.rerank,
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
    quote: {
      model,
      group: model.priceGroup ?? quoteGroup,
    } satisfies PlazaQuote,
  }));
  const renderPrice = ({ model, quote }: (typeof entries)[number]) => (
    <CatalogPriceSummary model={model} config={pricing} plazaQuote={quote} />
  );
  const renderAction = ({ model }: (typeof entries)[number]) => (
    <Link className="catalog-try" to={detailUrl(model)}>
      {copy.details}
      <ArrowUpRight size={13} />
    </Link>
  );
  return (
    <div className="brand-home public-refined catalog-refined catalog-browser catalog-demo-layout">
      <PublicHeader />
      <main className="models-page">
        <section className="catalog-heading">
          <div>
            <h1>{copy.title}</h1>
            <p>{copy.subtitle}</p>
          </div>
          <Button asChild variant="primary" size="small">
            <Link to="/console/keys?create=1">
              {copy.createKey}
              <ArrowUpRight size={15} />
            </Link>
          </Button>
        </section>
        <section className="catalog-filter-panel" aria-label={copy.capability}>
          <div className="catalog-filter-main">
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
            <label className="catalog-capability-filter">
              <span className="sr-only">{copy.capability}</span>
              <select
                value={capability}
                onChange={(event) =>
                  updateFilters({ capability: event.target.value })
                }
              >
                <option value="">{copy.allCapabilities}</option>
                {(plazaFilters.data?.tags ?? [])
                  .filter((tag) => tag.value in capabilities)
                  .map((tag) => (
                    <option key={tag.value} value={tag.value}>
                      {capabilities[tag.value]}
                    </option>
                  ))}
              </select>
            </label>
          </div>
          <div
            className="catalog-tabs catalog-vendor-tabs"
            role="group"
            aria-label={text(
              ...translatedMessage("filter_by_vendor_08dffcaea2"),
            )}
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
        </section>
        <div className="catalog-results-bar">
          <span className="catalog-result-count" role="status">
            {plazaQuery.isPending
              ? text(...translatedMessage("loading_a4e12025bb"))
              : plazaQuery.isError
                ? text(...translatedMessage("load_failed_36068d34aa"))
                : `${plazaQuery.data?.total ?? models.length} ${text(...translatedMessage("models_52c071580a"))}`}
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

        <div className="catalog-price-context">
          <Info size={14} aria-hidden="true" />
          <p>{copy.priceHelp}</p>
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
                  } else updateFilters({ q: "", vendor: "", capability: "" });
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
                      {catalogVendor(model) === "MyAPIs" ? (
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

        {(pageNumber > 1 || (plazaQuery.data?.total ?? 0) > 200) && (
          <nav
            className="catalog-pagination"
            aria-label={text(...translatedMessage("models_52c071580a"))}
          >
            <Button
              variant="secondary"
              disabled={pageNumber <= 1}
              onClick={() => updateFilters({ page: String(pageNumber - 1) })}
            >
              {text(...translatedMessage("previous_2672646f39"))}
            </Button>
            <bdi>
              {pageNumber} /{" "}
              {Math.max(1, Math.ceil((plazaQuery.data?.total ?? 0) / 200))}
            </bdi>
            <Button
              variant="secondary"
              disabled={pageNumber * 200 >= (plazaQuery.data?.total ?? 0)}
              onClick={() => updateFilters({ page: String(pageNumber + 1) })}
            >
              {text(...translatedMessage("next_c0fd39244f"))}
            </Button>
          </nav>
        )}
      </main>
    </div>
  );
}
