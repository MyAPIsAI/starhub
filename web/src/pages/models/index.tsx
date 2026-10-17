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
      ? text("Veyra（平台）", "Veyra platform", "منصة Veyra")
      : value;
  const capabilities: Record<string, string> = {
    text: text("文本", "Text", "نص"),
    code: text("编程", "Code", "برمجة"),
    vision: text("视觉", "Vision", "رؤية"),
    reasoning: text("推理", "Reasoning", "استدلال"),
    image: text("图片", "Image", "صور"),
    video: text("视频", "Video", "فيديو"),
  };
  const renderKind = (model: DirectoryModel) =>
    model.kind === "smart" ? (
      <span className="access-kind catalog-smart-label">
        {text("智能路由", "Smart routing", "توجيه ذكي")}
      </span>
    ) : !model.protocols.length ? (
      <span className="access-kind catalog-specific-label">
        {text(
          "客户端接入未提供",
          "Client connection unavailable",
          "اتصال العميل غير متوفر",
        )}
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
          ? text("接入此模型", "Connect model", "توصيل النموذج")
          : text("查看状态", "View status", "عرض الحالة")}
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
              {text(
                "找到适合你的模型",
                "Find your next model",
                "ابحث عن نموذجك التالي",
              )}
            </h1>
            <p>
              {text(
                "一把密钥，在熟悉的工具中切换多个模型。",
                "One key. Switch models in the tools you already use.",
                "مفتاح واحد للتبديل بين النماذج في أدواتك المعتادة.",
              )}
            </p>
          </div>
          <Link className="catalog-guide" to="/console/connect">
            <span className="catalog-guide-icon">
              <Cpu size={24} />
            </span>
            <span>
              <strong>
                {text("开始接入", "Connect a tool", "توصيل أداة")}
              </strong>
              <small>
                {text(
                  "复用密钥，获取工具配置",
                  "Reuse a key and get tool configuration",
                  "استخدم مفتاحًا قائمًا واحصل على الإعداد",
                )}
              </small>
            </span>
            <ArrowUpRight size={18} />
          </Link>
        </section>
        <div
          className="catalog-tabs catalog-vendor-tabs"
          role="group"
          aria-label={text(
            "按厂商筛选",
            "Filter by vendor",
            "التصفية حسب المورد",
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
                  ? text("全部厂商", "All vendors", "كل الموردين")
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
              aria-label={text("搜索模型", "Search models", "البحث عن نماذج")}
              placeholder={text(
                "搜索名称、模型标识或厂商",
                "Search name, model ID, or vendor",
                "ابحث بالاسم أو المعرف أو المورد",
              )}
              value={search}
              onChange={(e) => updateFilters({ q: e.target.value }, true)}
            />
            {search && (
              <button
                type="button"
                aria-label={text("清空搜索", "Clear search", "مسح البحث")}
                onClick={() => updateFilters({ q: "" }, true)}
              >
                <X size={15} />
              </button>
            )}
          </label>
          <div className="catalog-toolbar-end">
            <span className="catalog-result-count" role="status">
              {plazaQuery.isPending
                ? text("加载中…", "Loading…", "جار التحميل…")
                : plazaQuery.isError
                  ? text("加载失败", "Load failed", "تعذر التحميل")
                  : `${models.length} ${text("个模型", "models", "نماذج")}`}
            </span>
            <div
              className="catalog-view-switch"
              role="group"
              aria-label={text("展示方式", "Display mode", "طريقة العرض")}
            >
              <button
                type="button"
                data-catalog-view="cards"
                aria-pressed={view === "cards"}
                onClick={() => setView("cards")}
              >
                <LayoutGrid size={16} />
                <span>{text("卡片", "Cards", "بطاقات")}</span>
              </button>
              <button
                type="button"
                data-catalog-view="list"
                aria-pressed={view === "list"}
                onClick={() => setView("list")}
              >
                <List size={17} />
                <span>{text("列表", "List", "قائمة")}</span>
              </button>
            </div>
          </div>
        </div>
        <div className="catalog-price-context">
          <Info size={14} aria-hidden="true" />
          <p>
            {text(
              `平台报价来自后端当前分组 ${quoteGroup || "—"}，已包含分组倍率；实际按密钥分组计费。官方参考价的适用条件见详情。`,
              `Platform quotes are for backend group ${quoteGroup || "—"}, with its multiplier included; billing follows your key group. See details for official reference conditions.`,
              `أسعار الخادم للمجموعة ${quoteGroup || "—"} شاملة المعامل؛ الفوترة حسب مجموعة المفتاح. راجع شروط المرجع الرسمي في التفاصيل.`,
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
                ? text("正在加载模型", "Loading models", "جار تحميل النماذج")
                : plazaQuery.isError
                  ? text(
                      "模型加载失败",
                      "Failed to load models",
                      "تعذر تحميل النماذج",
                    )
                  : text(
                      "没有符合条件的模型",
                      "No matching models",
                      "لا توجد نماذج مطابقة",
                    )}
            </h2>
            <p>
              {plazaQuery.isPending
                ? text(
                    "正在获取模型目录，请稍候。",
                    "Fetching the model catalog. Please wait.",
                    "جار جلب دليل النماذج. يرجى الانتظار.",
                  )
                : plazaQuery.isError
                  ? text(
                      "暂时无法获取模型目录，请重试。",
                      "The model catalog is unavailable. Please retry.",
                      "دليل النماذج غير متاح حاليًا. يرجى إعادة المحاولة.",
                    )
                  : text(
                      "试试其他厂商或关键词。",
                      "Try another vendor or search term.",
                      "جرّب موردًا أو كلمة بحث أخرى.",
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
                  ? text("重试", "Retry", "إعادة المحاولة")
                  : text("清除筛选", "Clear filters", "مسح عوامل التصفية")}
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
                          "演示上下文",
                          "Demo context",
                          "السياق التجريبي",
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
            aria-label={text("模型列表", "Model list", "قائمة النماذج")}
          >
            <div className="catalog-list-head" role="row">
              <div role="columnheader">
                {text("模型 / 标识", "Model / ID", "النموذج / المعرف")}
              </div>
              <div role="columnheader">
                {text(
                  "能力 / 上下文",
                  "Capabilities / context",
                  "القدرات / السياق",
                )}
              </div>
              <div role="columnheader">
                {text("价格对照", "Price comparison", "مقارنة الأسعار")}
              </div>
              <div role="columnheader">
                {text("操作", "Actions", "الإجراءات")}
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
                        {text("上下文", "Context", "السياق")}{" "}
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
        <section className="catalog-ranking" style={{ marginTop: "var(--space-8)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)", marginBottom: "var(--space-4)" }}>
            <Sparkles size={18} />
            <h2 style={{ margin: 0, fontSize: "var(--text-lg)", fontWeight: "var(--weight-bold)" }}>
              {text("模型排行榜", "Model ranking", "ترتيب النماذج")}
            </h2>
            <div style={{ display: "flex", gap: 4 }}>
              {([
                { id: "value", label: text("性价比", "Value", "القيمة") },
                { id: "hot", label: text("热门", "Popular", "الشائع") },
                { id: "new", label: text("最新", "Newest", "الأحدث") },
              ] as { id: PlazaRankType; label: string }[]).map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => setRankType(tab.id)}
                  style={{
                    padding: "4px 10px",
                    fontSize: "var(--text-xs)",
                    fontWeight: rankType === tab.id ? 600 : 400,
                    background: rankType === tab.id ? "var(--brand)" : "var(--subtle)",
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
              {text("加载中…", "Loading…", "جارٍ التحميل…")}
            </p>
          ) : rankQuery.data?.items?.length ? (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))", gap: "var(--space-3)" }}>
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
                    <div style={{ fontWeight: 600, fontSize: "var(--text-sm)" }}>
                      {item.model.name}
                    </div>
                    <div style={{ fontSize: "var(--text-xs)", color: "var(--muted)" }}>
                      {item.model.vendor_display || item.model.vendor}
                    </div>
                  </div>
                  <div style={{ textAlign: "end" }}>
                    <div style={{ fontSize: "var(--text-xs)", color: "var(--muted)" }}>
                      {item.metric_label}
                    </div>
                    <div className="mono" style={{ fontSize: "var(--text-sm)", fontWeight: 600, color: "var(--brand)" }}>
                      {item.metric_value}
                    </div>
                  </div>
                </Link>
              ))}
            </div>
          ) : (
            <p style={{ color: "var(--muted)", fontSize: "var(--text-sm)" }}>
              {text("暂无排行榜数据", "No ranking data", "لا بيانات ترتيب")}
            </p>
          )}
        </section>
      </main>
    </div>
  );
}
