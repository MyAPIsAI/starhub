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
  LayoutGrid,
  List,
  Info,
  X,
} from "lucide-react";
import { DemoControlBar } from "../../components/demo-control-bar";
import { PublicHeader } from "../../components/product-shell";
import { Button } from "../../components/ui/button";
import { usePreferences } from "../../lib/preferences";
import { PlazaPrice } from "../../components/plaza-price";
import { ModelIdCopy } from "../../components/model-id-copy";
import {
  usePlazaModels,
  usePlazaFilters,
  plazaToDirectoryModel,
  type PlazaDirectoryModel,
} from "../../lib/api/plaza";
import "../../components/model-pricing.css";
import "../../components/access-center.css";
import "../../components/catalog-browser.css";
import "../../components/plaza.css";

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
  const vendor = params.get("vendor") ?? "";
  const requestedPage = Number(params.get("page") ?? "1");
  const page =
    Number.isSafeInteger(requestedPage) && requestedPage > 0
      ? Math.min(requestedPage, 1_000_000)
      : 1;
  const plazaQuery = usePlazaModels({
    q: search || undefined,
    vendors: vendor || undefined,
    page,
    page_size: 24,
  });
  const filtersQuery = usePlazaFilters();
  const models = plazaQuery.data?.items.map(plazaToDirectoryModel) ?? [];
  const vendors = filtersQuery.data?.vendors ?? [];
  const total = plazaQuery.data?.total ?? 0;
  const totalPages = Math.max(
    1,
    Math.ceil(total / (plazaQuery.data?.page_size ?? 24)),
  );
  const updateFilters = (values: Record<string, string>, replace = false) => {
    const next = new URLSearchParams(params);
    // Record view explicitly when navigating so refresh and return are exact.
    next.set("view", view);
    if ("q" in values || "vendor" in values) next.delete("page");
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
  const detailUrl = (model: PlazaDirectoryModel) =>
    `/models/${encodeURIComponent(model.id)}?from=${encodeURIComponent(returnTo)}`;
  const capabilities: Record<string, string> = {
    text: text("文本", "Text", "نص"),
    code: text("编程", "Code", "برمجة"),
    vision: text("视觉", "Vision", "رؤية"),
    reasoning: text("推理", "Reasoning", "استدلال"),
    image: text("图片", "Image", "صور"),
    video: text("视频", "Video", "فيديو"),
    function_call: text("工具调用", "Tool calling", "استدعاء الأدوات"),
  };
  const renderKind = (model: PlazaDirectoryModel) =>
    !model.isAvailable ? (
      <span className="access-kind catalog-specific-label">
        {text(
          "当前分组不可用",
          "Unavailable in this group",
          "غير متاح في المجموعة الحالية",
        )}
      </span>
    ) : null;
  const renderCapabilities = (model: PlazaDirectoryModel) => (
    <div className="catalog-capabilities">
      {model.capabilities.map((c) => (
        <span key={c}>{capabilities[c] ?? c}</span>
      ))}
    </div>
  );
  const entries = models.map((model) => ({ model }));
  const renderPrice = ({ model }: (typeof entries)[number]) => (
    <PlazaPrice pricing={model.pricing} />
  );
  const renderAction = ({ model }: (typeof entries)[number]) => {
    const canConnect = model.isAvailable && model.protocols.length > 0;
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
          {[
            { value: "", label: "", count: filtersQuery.data?.total },
            ...vendors,
          ].map((item) => (
            <button
              type="button"
              key={item.value}
              data-catalog-vendor={item.value || "all"}
              className={vendor === item.value ? "is-active" : ""}
              aria-pressed={vendor === item.value}
              onClick={() => updateFilters({ vendor: item.value })}
            >
              {!item.value
                ? text("全部厂商", "All vendors", "كل الموردين")
                : item.label}
              <span>{item.count ?? "—"}</span>
            </button>
          ))}
        </div>
        <div className="catalog-toolbar">
          <label className="catalog-search">
            <Search size={17} aria-hidden="true" />
            <input
              type="search"
              aria-label={text("搜索模型", "Search models", "البحث عن نماذج")}
              placeholder={text(
                "搜索名称、模型标识、厂商或说明",
                "Search name, model ID, vendor, or description",
                "ابحث بالاسم أو المعرف أو المورد أو الوصف",
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
              {plazaQuery.isPending ? "—" : total}{" "}
              {text("个模型", "models", "نماذج")}
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
              `当前分组：${plazaQuery.data?.group ?? "—"}。报价已包含该分组倍率，官方参考价仅用于对照。`,
              `Current group: ${plazaQuery.data?.group ?? "—"}. Quotes include the group multiplier; official prices are for comparison.`,
              `المجموعة الحالية: ${plazaQuery.data?.group ?? "—"}. تشمل الأسعار معامل المجموعة والأسعار الرسمية للمقارنة.`,
            )}
          </p>
        </div>
        {filtersQuery.isError && (
          <div className="access-notice" role="alert">
            {text(
              "厂商选项加载失败。",
              "Vendor filters could not be loaded.",
              "تعذر تحميل مرشحات الموردين.",
            )}
            <Button size="small" onClick={() => void filtersQuery.refetch()}>
              {text("重试", "Retry", "إعادة المحاولة")}
            </Button>
          </div>
        )}
        {plazaQuery.isPending ? (
          <div className="catalog-empty" role="status">
            {text("正在加载模型…", "Loading models…", "جار تحميل النماذج…")}
          </div>
        ) : plazaQuery.isError ? (
          <div className="catalog-empty" role="alert">
            <h2>
              {text(
                "模型加载失败",
                "Models could not be loaded",
                "تعذر تحميل النماذج",
              )}
            </h2>
            <Button onClick={() => void plazaQuery.refetch()}>
              {text("重试", "Retry", "إعادة المحاولة")}
            </Button>
          </div>
        ) : !models.length ? (
          <div className="catalog-empty">
            <Cpu size={36} />
            <h2>
              {text(
                "没有符合条件的模型",
                "No matching models",
                "لا توجد نماذج مطابقة",
              )}
            </h2>
            <p>
              {text(
                "试试其他厂商或关键词。",
                "Try another vendor or search term.",
                "جرّب موردًا أو كلمة بحث أخرى.",
              )}
            </p>
            <Button
              variant="secondary"
              onClick={() => updateFilters({ q: "", vendor: "" })}
            >
              {text("清除筛选", "Clear filters", "مسح عوامل التصفية")}
            </Button>
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
                      {model.vendor.slice(0, 1)}
                    </span>
                    <span className="catalog-vendor">{model.vendor}</span>
                    {renderKind(model)}
                  </div>
                  <Link className="catalog-model-name" to={detailUrl(model)}>
                    <h2>{model.name}</h2>
                    <ArrowUpRight size={17} />
                  </Link>
                  <ModelIdCopy id={model.id} />
                  <div className="catalog-model-meta">
                    {renderCapabilities(model)}
                    {model.contextLength > 0 && (
                      <span
                        className="catalog-context-info"
                        title={text("上下文上限", "Context limit", "حد السياق")}
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
                      <span>{model.vendor}</span>
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
                    {model.contextLength > 0 && (
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
        {plazaQuery.isSuccess && totalPages > 1 && (
          <nav
            className="plaza-pagination"
            aria-label={text("模型分页", "Model pagination", "صفحات النماذج")}
          >
            <Button
              disabled={page <= 1}
              onClick={() =>
                updateFilters({ page: String(Math.min(page - 1, totalPages)) })
              }
            >
              {text("上一页", "Previous", "السابق")}
            </Button>
            <span>
              {text(
                `第 ${page} / ${totalPages} 页`,
                `Page ${page} / ${totalPages}`,
                `الصفحة ${page} / ${totalPages}`,
              )}
            </span>
            <Button
              disabled={page >= totalPages}
              onClick={() => updateFilters({ page: String(page + 1) })}
            >
              {text("下一页", "Next", "التالي")}
            </Button>
          </nav>
        )}
        <div className="demo-inline-footer">
          <DemoControlBar inline />
        </div>
      </main>
    </div>
  );
}
