import { useState } from "react";
import { useParams, Link, useSearchParams } from "react-router-dom";
import {
  ArrowLeft,
  ArrowRight,
  KeyRound,
  ShieldCheck,
  Sparkles,
  Terminal,
} from "lucide-react";
import { useModelPricing } from "../../lib/model-pricing";
import { PublicHeader } from "../../components/product-shell";
import { Button } from "../../components/ui/button";
import { type DirectoryRate } from "../../config/model-directory";
import { plazaAmounts } from "../../lib/plaza-pricing";
import { CatalogPriceComparison } from "../../components/catalog-price-comparison";
import { ModelIdCopy } from "../../components/model-id-copy";
import { DirectoryPrice } from "../../components/directory-price";
import { formatMoney } from "../../lib/money";
import { usePreferences } from "../../lib/preferences";
import { placeholders } from "../../config/placeholders";
import { useProductPreview } from "../../lib/product-preview";
import { useBackendKeys } from "../../lib/api/key-adapter";
import { resolveCatalogPricingContext } from "../../lib/catalog-browser";
import { getKeyModelOptions } from "../../lib/key-model-policy";
import { usePlazaModel, plazaToDirectoryModel } from "../../lib/api/plaza";
import { ApiError } from "../../lib/api/http";
import "../../components/model-pricing.css";
import "../../components/access-center.css";

export default function ModelDetail() {
  const { id } = useParams<{ id: string }>();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const pricing = useModelPricing((s) => s.config);
  const accountId = useProductPreview((s) => s.user.id);
  const keys = useBackendKeys().keys;
  const selectedKey = useProductPreview((s) => s.selectedKey);
  const [selectedTier, setTier] = useState<string | null>(null);
  const [params] = useSearchParams();
  const from = params.get("from");
  // Only a relative marketplace path is a supported return destination.
  const returnTo =
    from && (from === "/models" || from.startsWith("/models?"))
      ? from
      : "/models";
  const plazaDetail = usePlazaModel(id);
  const model =
    !plazaDetail.isError && plazaDetail.data?.model
      ? plazaToDirectoryModel(plazaDetail.data.model)
      : undefined;
  const quoteGroup = plazaDetail.data?.group ?? "";
  // Only the current backend group has a quote; never copy it to other groups.
  const groups = model && quoteGroup ? [{ id: quoteGroup }] : [];
  const tier = groups.some((group) => group.id === selectedTier)
    ? selectedTier
    : quoteGroup;
  const amounts = model ? plazaAmounts(model.pricing) : undefined;
  const isLoading = plazaDetail.isPending;
  const hasLoadError =
    plazaDetail.isError &&
    !(
      plazaDetail.error instanceof ApiError && plazaDetail.error.status === 404
    );
  const compatibleKey =
    model &&
    [...keys]
      .sort(
        (a, b) => Number(b.id === selectedKey) - Number(a.id === selectedKey),
      )
      .find(
        (key) =>
          resolveCatalogPricingContext(pricing, accountId, [key], key.id)
            .source === "key" &&
          getKeyModelOptions(key, undefined, pricing).some(
            (option) => option.id === model.id,
          ),
      );
  const connectUrl = model
    ? `/console/connect?model=${encodeURIComponent(model.id)}${compatibleKey ? `&key=${encodeURIComponent(compatibleKey.id)}` : ""}`
    : "/models";
  const price = model
    ? {
        available: model.online,
        rates: [] as DirectoryRate[],
        canEstimate: false,
        minAmountNano: undefined,
        maxAmountNano: undefined,
      }
    : null;
  const supportsClient = Boolean(model?.protocols.length);
  const canConnect = Boolean(model?.online && supportsClient);
  const tierPrices =
    model && model.kind !== "smart"
      ? groups.map((item) => ({
          tier: item,
          price,
        }))
      : [];
  return (
    <div className="brand-home public-refined model-detail-refined">
      <PublicHeader />
      <main className="access-model-detail">
        <Button asChild variant="secondary" size="small">
          <Link to={returnTo}>
            <ArrowLeft size={14} />
            {text("返回模型目录", "Back to models", "العودة إلى النماذج")}
          </Link>
        </Button>
        {!model ? (
          <section
            className="access-empty"
            role={hasLoadError ? "alert" : "status"}
            aria-busy={isLoading}
          >
            <h1>
              {isLoading
                ? text("正在加载模型", "Loading model", "جار تحميل النموذج")
                : hasLoadError
                  ? text(
                      "模型加载失败",
                      "Failed to load model",
                      "تعذر تحميل النموذج",
                    )
                  : text("模型不存在", "Model not found", "النموذج غير موجود")}
            </h1>
            <p>
              {isLoading
                ? text(
                    "正在获取模型详情，请稍候。",
                    "Fetching model details. Please wait.",
                    "جار جلب تفاصيل النموذج. يرجى الانتظار.",
                  )
                : hasLoadError
                  ? text(
                      "暂时无法获取模型详情，请重试。",
                      "Model details are unavailable. Please retry.",
                      "تفاصيل النموذج غير متاحة حاليًا. يرجى إعادة المحاولة.",
                    )
                  : text(
                      "没有找到此模型标识，请返回目录选择可用模型。不会替换成其他模型。",
                      "This model ID was not found. Return to the catalog to choose a model; no substitution was made.",
                      "لم يتم العثور على معرف النموذج. اختر نموذجًا من الدليل؛ لم يتم استبداله.",
                    )}
            </p>
            <code>{id}</code>
            {hasLoadError ? (
              <Button onClick={() => void plazaDetail.refetch()}>
                {text("重试", "Retry", "إعادة المحاولة")}
              </Button>
            ) : !isLoading ? (
              <Button asChild>
                <Link to={returnTo}>
                  {text(
                    "查看模型目录",
                    "View model directory",
                    "عرض دليل النماذج",
                  )}
                </Link>
              </Button>
            ) : null}
          </section>
        ) : (
          <>
            <section className="access-detail-heading">
              <div>
                <span className="access-kind">
                  {model.kind === "smart"
                    ? text("智能路由", "Smart routing", "توجيه ذكي")
                    : text("指定模型", "Specific model", "نموذج محدد")}
                </span>
                <h1>
                  {model.name}
                  {lang === "zh" && model.kind === "smart"
                    ? "（智能路由）"
                    : ""}
                </h1>
                <ModelIdCopy id={model.id} />
                <p>{model.description[lang]}</p>
              </div>
              <div className="access-actions">
                {canConnect ? (
                  <>
                    <Button asChild variant="primary">
                      <Link to={connectUrl}>
                        <ArrowRight size={15} />
                        {compatibleKey
                          ? text(
                              "复用密钥并接入",
                              "Reuse key & connect",
                              "استخدام المفتاح والتوصيل",
                            )
                          : text("开始接入", "Connect a tool", "توصيل أداة")}
                      </Link>
                    </Button>
                    <Button asChild variant="secondary">
                      <Link
                        to={`/console/playground?model=${encodeURIComponent(model.id)}${compatibleKey ? `&key=${compatibleKey.id}` : ""}`}
                      >
                        <Terminal size={15} />
                        {text(
                          "可选：模拟验证",
                          "Optional: demo validation",
                          "اختياري: تحقق تجريبي",
                        )}
                      </Link>
                    </Button>
                  </>
                ) : (
                  <Button variant="secondary" disabled>
                    {!supportsClient
                      ? text(
                          "客户端接入未提供",
                          "Client connection unavailable",
                          "اتصال العميل غير متوفر",
                        )
                      : text(
                          "当前分组不可接入",
                          "Unavailable in this group",
                          "غير متاح في المجموعة",
                        )}
                  </Button>
                )}
              </div>
            </section>
            {!model.online && (
              <div className="access-notice access-error">
                {text(
                  "当前分组没有启用的渠道覆盖此模型，暂不可接入；历史调用和账单继续保留。",
                  "No enabled channel serves this model in the current group. Historical requests and bills are retained.",
                  "لا توجد قناة مفعلة لهذا النموذج في المجموعة الحالية. تبقى الطلبات والفواتير السابقة محفوظة.",
                )}
              </div>
            )}
            {!supportsClient && (
              <div className="access-notice">
                {text(
                  "当前分组尚未提供可用的客户端协议，请检查渠道配置。",
                  "No supported client protocol is available in this group. Check channel configuration.",
                  "لا يوجد بروتوكول عميل مدعوم في المجموعة الحالية. تحقق من إعدادات القناة.",
                )}
              </div>
            )}
            {supportsClient && (
              <div className="access-notice">
                <KeyRound size={18} />
                <span>
                  {compatibleKey
                    ? text(
                        `可复用「${compatibleKey.name}」。切换模型无需更换密钥；仍受原授权及额度约束。`,
                        `Reuse “${compatibleKey.name}”. Changing models does not require a new key; existing permissions and limits apply.`,
                        `يمكن إعادة استخدام «${compatibleKey.name}» مع الالتزام بالصلاحيات والحدود.`,
                      )
                    : text(
                        "创建密钥时只选服务分组与额度。此模型仅作为接入示例，不会自动设置单模型白名单。",
                        "New keys select a service group and budget. This model is only a configuration example, not an automatic single-model allowlist.",
                        "اختر المجموعة والميزانية عند إنشاء المفتاح. هذا النموذج مثال للإعداد فقط ولا يقيد الصلاحيات تلقائيًا.",
                      )}
                </span>
              </div>
            )}
            <section className="access-facts">
              <div>
                <small>{text("支持协议", "Protocols", "البروتوكولات")}</small>
                <strong>
                  {supportsClient
                    ? model.protocols.join(" / ")
                    : text(
                        "当前分组未提供",
                        "Unavailable in this group",
                        "غير متوفرة في المجموعة الحالية",
                      )}
                </strong>
              </div>
              <div>
                <small>{text("上下文", "Context", "السياق")}</small>
                <strong>{model.contextDisplay}</strong>
              </div>
              <div>
                <small>
                  {text("调用方式", "Request behavior", "سلوك الطلب")}
                </small>
                <strong>
                  {model.kind === "smart"
                    ? text(
                        "授权候选内选择",
                        "Authorized candidate selection",
                        "اختيار ضمن الصلاحيات",
                      )
                    : text(
                        "严格使用请求模型",
                        "Exact requested model",
                        "النموذج المطلوب نفسه",
                      )}
                </strong>
              </div>
              <div>
                <small>{text("服务质量", "Service quality", "جودة الخدمة")}</small>
                <Link
                  to={`/console/service-quality?model=${encodeURIComponent(model.id)}`}
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 4,
                    color: "var(--brand)",
                    fontSize: "var(--text-sm)",
                    textDecoration: "none",
                  }}
                >
                  <ShieldCheck size={14} />
                  {text(
                    "查看成功率与延迟",
                    "View success rate & latency",
                    "عرض النجاح والاستجابة",
                  )}
                </Link>
              </div>
            </section>
            {model.kind === "smart" && (
              <section className="access-panel">
                <h2>
                  <Sparkles size={20} />
                  {text(
                    "费用优先路由",
                    "Cost-first routing",
                    "التوجيه حسب التكلفة",
                  )}
                </h2>
                <p>
                  {text(
                    "按本次请求的预计输入、输出用量，在满足能力和协议的已授权候选中选择估算费用最低的模型。工具调用链固定实际模型，后续不兼容时明确报错。",
                    "Select the lowest estimated cost among authorized candidates that satisfy the request protocol and capabilities, using estimated request usage. Tool chains retain the actual model; incompatible continuation fails explicitly.",
                    "يختار أقل تكلفة مقدرة ضمن النماذج المصرح بها والمتوافقة مع الطلب. تحتفظ سلسلة الأدوات بالنموذج الفعلي ويرفض الاستمرار غير المتوافق بوضوح.",
                  )}
                </p>
                <p>
                  {text(
                    "这是一种调用策略，不是额外服务分组。只按实际模型计费一次，不另收路由费用。文本、编程、视觉与推理能力按候选过滤；本演示仅模拟文本输入。",
                    "This is a routing strategy, not another service group. Charge once at the actual model rate, with no routing surcharge. Candidates are filtered for text, code, vision, and reasoning; this demo simulates text input only.",
                    "هذه استراتيجية توجيه وليست مجموعة خدمة إضافية. تحسب التكلفة مرة واحدة بسعر النموذج الفعلي بلا رسم توجيه. العرض يحاكي إدخال النص فقط.",
                  )}
                </p>
              </section>
            )}
            <section className="access-panel access-public-comparison">
              <h2>
                {text(
                  "官方参考与平台报价",
                  "Official reference and platform prices",
                  "المرجع الرسمي وأسعار المنصة",
                )}
              </h2>
              <p className="access-muted">
                {text(
                  "平台报价来自后端当前分组，已包含分组倍率，实际按密钥分组计费。官方参考由平台维护，不自动改变平台计费。",
                  "Backend quotes include the current group multiplier; billing follows your key group. Official references are maintained by the platform and do not change billing.",
                  "أسعار الخادم شاملة معامل المجموعة الحالية؛ الفوترة حسب مجموعة المفتاح. يدير المسؤول المرجع الرسمي دون تغيير الفوترة.",
                )}
              </p>
              <CatalogPriceComparison
                model={model}
                config={pricing}
                plazaQuote={{ model, group: quoteGroup }}
                detailed
              />
            </section>
            <section className="access-panel">
              <div className="access-panel-heading">
                <h2>
                  {text(
                    "当前分组价格",
                    "Your service group rates",
                    "أسعار مجموعة الخدمة",
                  )}
                </h2>
                <label className="access-inline-label">
                  {text("服务分组", "Service group", "مجموعة الخدمة")}
                  <select
                    value={tier ?? ""}
                    disabled={!groups.length}
                    onChange={(e) => setTier(e.target.value)}
                  >
                    {!groups.length && (
                      <option value="">
                        {text(
                          "暂无可用分组",
                          "No available group",
                          "لا توجد مجموعة متاحة",
                        )}
                      </option>
                    )}
                    {groups.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.id}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <p className="access-muted">
                {text(
                  "后端报价 · 按模型计价方式及单位展示 · 已计入当前分组倍率",
                  "Backend quotes · Configured billing units · Current group multiplier included",
                  "أسعار الخادم حسب وحدات النموذج وشاملة معامل المجموعة الحالية",
                )}
              </p>
              {!price?.available ? (
                <div className="access-notice access-error">
                  {text(
                    "当前服务分组暂无可用模型。请选择其他分组查看，不会自动换模。",
                    "No model is available in this group. Choose another group to inspect rates.",
                    "لا يوجد نموذج متاح في هذه المجموعة. اختر مجموعة أخرى لعرض الأسعار.",
                  )}
                </div>
              ) : (
                <>
                  {model.kind === "smart" && (
                    <div className="access-price-estimate">
                      <strong dir={price.canEstimate ? "ltr" : undefined}>
                        {price.canEstimate
                          ? `${formatMoney(price.minAmountNano!, "call")}–${formatMoney(price.maxAmountNano!, "call")}`
                          : text(
                              "暂不可估算",
                              "Estimate unavailable",
                              "التقدير غير متاح",
                            )}
                      </strong>
                      <span>
                        {text(
                          price.rates.every(
                            (rate) => rate.rule.mode === "tokens",
                          )
                            ? "每次模拟请求估算：普通输入 1,000 + 输出 300 词元，不含缓存。报价失败的候选不计入区间；具体原因见下表。下方仅含支持客户端协议的文本候选，实际范围还需满足密钥白名单及请求能力。"
                            : "候选采用不同计价规则；区间按下方各自示例用量估算，不是统一词元单价。报价失败候选未计入区间；实际仍受密钥授权、协议和能力约束。",
                          price.rates.every(
                            (rate) => rate.rule.mode === "tokens",
                          )
                            ? "Estimate per request: 1,000 uncached input + 300 output tokens, no cache. Candidates below are further limited by the key allowlist and request capabilities."
                            : "Candidates use different pricing rules and the example usage shown below. This is not a common token unit rate. Failed quotes are excluded; key permissions, protocols and capabilities still apply.",
                          price.rates.every(
                            (rate) => rate.rule.mode === "tokens",
                          )
                            ? "تقدير لكل طلب: 1000 رمز إدخال + 300 إخراج بلا تخزين. تخضع القائمة لصلاحيات المفتاح وقدرات الطلب."
                            : "للمرشحين قواعد فوترة واستخدام أمثلة مختلفة كما هو مبين أدناه، وليس سعرا موحدا للرموز. تستثنى عروض الأسعار الفاشلة وتبقى قيود المفتاح والبروتوكولات والقدرات.",
                        )}
                      </span>
                    </div>
                  )}
                  {model.kind === "specified" ? (
                    <div className="directory-detail-single">
                      <DirectoryPrice
                        plazaPricing={model.pricing}
                        lang={lang}
                      />
                      {model.pricing.mode === "per_token" && (
                        <p className="access-muted">
                          {text(
                            "缓存读取 / 写入（美元 / 百万词元）：",
                            "Cache read / write (USD / million tokens): ",
                            "قراءة / كتابة التخزين (دولار / مليون رمز): ",
                          )}
                          {amounts?.cacheRead !== undefined
                            ? `$${amounts.cacheRead}`
                            : "—"}{" "}
                          /{" "}
                          {amounts?.cacheWrite !== undefined
                            ? `$${amounts.cacheWrite}`
                            : "—"}
                        </p>
                      )}
                    </div>
                  ) : (
                    <>
                      <div
                        className="access-table-wrap"
                        tabIndex={0}
                        role="region"
                        aria-label={text(
                          "模型价格表，可横向滚动",
                          "Model price table, horizontally scrollable",
                          "جدول الأسعار قابل للتمرير أفقيًا",
                        )}
                      >
                        <table className="access-table">
                          <thead>
                            <tr>
                              <th>{text("模型", "Model", "النموذج")}</th>
                              <th>
                                {text(
                                  "当前报价与估算假设",
                                  "Current quote and assumptions",
                                  "التسعير الحالي وافتراضات التقدير",
                                )}
                              </th>
                              <th>
                                {text(
                                  "缓存单价",
                                  "Cache unit rates",
                                  "أسعار التخزين",
                                )}
                              </th>
                            </tr>
                          </thead>
                          <tbody>
                            {price.rates.map((rate) => (
                              <tr key={rate.model.id}>
                                <td>
                                  <Link
                                    to={`/models/${encodeURIComponent(rate.model.id)}?from=${encodeURIComponent(returnTo)}`}
                                  >
                                    {rate.model.name}
                                  </Link>
                                </td>
                                <td>
                                  <DirectoryPrice rate={rate} lang={lang} />
                                </td>
                                <td>
                                  {rate.rule.mode === "tokens" && rate.quote ? (
                                    <>
                                      {text(
                                        "读取 / 写入",
                                        "Read / write",
                                        "قراءة / كتابة",
                                      )}
                                      <br />
                                      {rate.cacheRead !== undefined
                                        ? `$${rate.cacheRead}`
                                        : "—"}{" "}
                                      /{" "}
                                      {rate.cacheWrite !== undefined
                                        ? `$${rate.cacheWrite}`
                                        : "—"}
                                      <br />
                                      <small>
                                        {text(
                                          "美元 / 百万词元",
                                          "USD / million tokens",
                                          "دولار / مليون رمز",
                                        )}
                                      </small>
                                    </>
                                  ) : (
                                    text(
                                      "以左侧规则为准",
                                      "See configured rule",
                                      "حسب القاعدة المحددة",
                                    )
                                  )}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </>
                  )}
                  <p className="access-muted">
                    {text(
                      `分组倍率 ${model.pricing.group_ratio}× 已包含在上述售价中，不重复计算。未配置缓存价格显示 —，不代表免费。历史账单保留请求当时费率。`,
                      `Group multiplier ${model.pricing.group_ratio}× is already included. An unconfigured cache rate is —, not free. Historical bills retain their original rates.`,
                      `معامل المجموعة ${model.pricing.group_ratio}× محسوب بالفعل. — لا تعني المجانية. تحتفظ الفواتير السابقة بالأسعار الأصلية.`,
                    )}
                  </p>
                </>
              )}
              {tierPrices.length > 0 && (
                <details className="access-details">
                  <summary>
                    {text(
                      "查看当前分组价格",
                      "Current group rates",
                      "أسعار المجموعة الحالية",
                    )}
                  </summary>
                  <div
                    className="access-table-wrap"
                    tabIndex={0}
                    role="region"
                    aria-label={text(
                      "模型价格表，可横向滚动",
                      "Model price table, horizontally scrollable",
                      "جدول الأسعار قابل للتمرير أفقيًا",
                    )}
                  >
                    <table className="access-table">
                      <thead>
                        <tr>
                          <th>{text("分组", "Group", "المجموعة")}</th>
                          <th>
                            {text(
                              "当前实际价格",
                              "Current effective price",
                              "السعر الفعلي الحالي",
                            )}
                          </th>
                          <th>{text("成功率/延迟", "Success/P95", "النجاح/P95")}</th>
                          <th>{text("SLA 目标", "SLA targets", "أهداف SLA")}</th>
                          <th>{text("辅助倍率", "Multiplier", "المعامل")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {tierPrices.map(
                          ({ tier: group, price: groupPrice }) => (
                            <tr key={group.id} className={!groupPrice?.available ? "detail-group-unavailable" : ""}>
                              <td>{group.id}</td>
                              <td>
                                {groupPrice?.available ? (
                                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                                    <DirectoryPrice
                                      plazaPricing={model.pricing}
                                      lang={lang}
                                      compact
                                    />
                                    <Link
                                      to={`/console/keys?create=1&model=${encodeURIComponent(model.id)}&group=${encodeURIComponent(group.id)}`}
                                      style={{
                                        fontSize: "var(--text-xs)",
                                        color: "var(--brand)",
                                        textDecoration: "none",
                                        whiteSpace: "nowrap",
                                      }}
                                    >
                                      {text("用此分组", "Use group", "استخدم")}
                                    </Link>
                                  </div>
                                ) : (
                                  <span style={{ color: "var(--muted)" }}>
                                    {text("暂不可用", "Unavailable", "غير متاح")}
                                  </span>
                                )}
                              </td>
                              <td>{model.pricing.group_ratio}×</td>
                              <td>
                                {(() => {
                                  const tierDef = placeholders.tiers.find((t) => t.id === group.id);
                                  if (!tierDef) return "—";
                                  return (
                                    <span className="mono" style={{ fontSize: "var(--text-xs)" }} dir="ltr">
                                      {tierDef.success}% / {tierDef.p95}ms
                                    </span>
                                  );
                                })()}
                              </td>
                              <td>
                                {(() => {
                                  const sqKey = `sq-${model.id}-${group.id}`;
                                  try {
                                    const raw = localStorage.getItem(sqKey);
                                    if (raw) {
                                      const sq = JSON.parse(raw);
                                      const success = sq.success != null ? `${(sq.success * 100).toFixed(1)}%` : "—";
                                      const p95 = sq.p95 != null ? `${sq.p95}ms` : "—";
                                      return (
                                        <span className="mono" style={{ fontSize: "var(--text-xs)" }} dir="ltr">
                                          {success} / {p95}
                                        </span>
                                      );
                                    }
                                  } catch { /* ignore */ }
                                  return (
                                    <Link
                                      to={`/console/service-quality?model=${encodeURIComponent(model.id)}&group=${encodeURIComponent(group.id)}`}
                                      style={{
                                        fontSize: "var(--text-xs)",
                                        color: "var(--brand)",
                                        textDecoration: "none",
                                      }}
                                    >
                                      {text("查看", "View", "عرض")}
                                    </Link>
                                  );
                                })()}
                              </td>
                            </tr>
                          ),
                        )}
                      </tbody>
                    </table>
                  </div>
                </details>
              )}
            </section>
            <section className="access-panel">
              <h2>
                {text("定价详情", "Pricing details", "تفاصيل التسعير")}
              </h2>
              <p className="access-muted">
                {text(
                  "以下展示模型的定价结构与缓存折扣。实际费用取决于请求时的用量、缓存命中和分组倍率。",
                  "The following shows the model's pricing structure and cache discounts. Actual cost depends on request usage, cache hits, and group multiplier.",
                  "يعرض ما يلي هيكل تسعير النموذج وخصومات التخزين. التكلفة الفعلية تعتمد على الاستخدام وذاكرة التخزين المؤقتة ومعامل المجموعة.",
                )}
              </p>
              <div className="access-table-wrap" tabIndex={0} role="region">
                <table className="access-table">
                  <thead>
                    <tr>
                      <th>{text("计费项", "Billing item", "بند الفوترة")}</th>
                      <th>{text("单价（美元/百万 Token）", "Rate (USD/M tokens)", "السعر (دولار/مليون رمز)")}</th>
                      <th>{text("说明", "Description", "الوصف")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>{text("输入 Token", "Input tokens", "رموز الإدخال")}</td>
                      <td className="mono">
                        ${amounts?.input ?? "—"}
                      </td>
                      <td>
                        {text(
                          "请求中的提示词和对话内容",
                          "Prompt and conversation content in the request",
                          "المحتوى التلقيني والمحادثة في الطلب",
                        )}
                      </td>
                    </tr>
                    <tr>
                      <td>{text("输出 Token", "Output tokens", "رموز الإخراج")}</td>
                      <td className="mono">
                        ${amounts?.output ?? "—"}
                      </td>
                      <td>
                        {text(
                          "模型生成的回复内容",
                          "Model-generated response content",
                          "محتوى الاستجابة المُنشأ بواسطة النموذج",
                        )}
                      </td>
                    </tr>
                    {amounts?.cacheRead !== undefined && (
                      <tr>
                        <td>{text("缓存读取", "Cache read", "قراءة التخزين")}</td>
                        <td className="mono">
                          ${amounts.cacheRead}
                        </td>
                        <td>
                          {text(
                            "命中缓存的输入按折扣价计费",
                            "Cached input is billed at a discounted rate",
                            "الإدخال المخزن يُحاسب بسعر مخفض",
                          )}
                        </td>
                      </tr>
                    )}
                    {amounts?.cacheWrite !== undefined && (
                      <tr>
                        <td>{text("缓存写入", "Cache write", "كتابة التخزين")}</td>
                        <td className="mono">
                          ${amounts.cacheWrite}
                        </td>
                        <td>
                          {text(
                            "首次写入缓存的输入内容",
                            "Initial write of input to cache",
                            "الكتابة الأولى للإدخال إلى التخزين",
                          )}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
              <div className="pricing-mobile-cards" style={{ display: "none" }}>
                {[
                  { label: text("输入 Token", "Input tokens", "رموز الإدخال"), value: amounts?.input },
                  { label: text("输出 Token", "Output tokens", "رموز الإخراج"), value: amounts?.output },
                  ...(amounts?.cacheRead !== undefined ? [{ label: text("缓存读取", "Cache read", "قراءة التخزين"), value: amounts.cacheRead }] : []),
                  ...(amounts?.cacheWrite !== undefined ? [{ label: text("缓存写入", "Cache write", "كتابة التخزين"), value: amounts.cacheWrite }] : []),
                ].map((item) => (
                  <div key={item.label} className="pricing-mobile-card">
                    <span className="pricing-mobile-label">{item.label}</span>
                    <span className="mono pricing-mobile-value">${item.value ?? "—"}</span>
                  </div>
                ))}
              </div>
              <p className="access-muted">
                {text(
                  `当前分组倍率：${model.pricing.group_ratio}×（已包含在上方售价中）。历史账单保留请求当时费率。`,
                  `Current group multiplier: ${model.pricing.group_ratio}× (included above). Historical bills retain their original rates.`,
                  `معامل المجموعة الحالي: ${model.pricing.group_ratio}× (مضمّن أعلاه). تحتفظ الفواتير السابقة بأسعارها الأصلية.`,
                )}
              </p>
            </section>
            {model.pricing.mode === "expression" && model.pricing.expression && (
              <section className="access-panel">
                <h2>
                  {text("表达式定价", "Expression pricing", "تسعير بالتعبير")}
                </h2>
                <p className="access-muted">
                  {text(
                    "此模型使用表达式定价规则，实际费用取决于请求时的用量变量。下方为平台配置的表达式。",
                    "This model uses expression-based pricing. Actual cost depends on request usage variables. Below is the platform-configured expression.",
                    "يستخدم هذا النموذج تسعيرًا قائمًا على التعبيرات. التكلفة الفعلية تعتمد على متغيرات الاستخدام. أدناه التعبير المكون.",
                  )}
                </p>
                <div
                  className="docs-code"
                  style={{ marginBottom: "var(--space-4)" }}
                >
                  <div>
                    <span>
                      {text("计价表达式", "Pricing expression", "تعبير التسعير")}
                    </span>
                  </div>
                  <pre dir="ltr">
                    <code className="mono">
                      {model.pricing.expression}
                    </code>
                  </pre>
                </div>
              </section>
            )}
            {model.pricing.mode === "per_token" && model.pricing.per_request != null && model.pricing.per_request > 0 && (
              <section className="access-panel">
                <h2>
                  {text("阶梯定价", "Tiered pricing", "تسعير متدرج")}
                </h2>
                <p className="access-muted">
                  {text(
                    "此模型支持阶梯定价：输入 Token 超过阈值时按更低费率计费。下方为平台配置的阶梯分界与对应单价。",
                    "This model supports tiered pricing: input tokens beyond a threshold are billed at a lower rate. Below are the tier boundaries and corresponding rates.",
                    "يدعم هذا النموذج التسعير المتدرج: الرموز التي تتجاوز الحد تُحاسب بسعر أقل. أدناه حدود المراحل والأسعار المقابلة.",
                  )}
                </p>
                <div className="access-table-wrap" tabIndex={0} role="region">
                  <table className="access-table">
                    <thead>
                      <tr>
                        <th>{text("阶梯", "Tier", "المرحلة")}</th>
                        <th>{text("输入阈值", "Input threshold", "حد الإدخال")}</th>
                        <th>{text("单价（美元/百万 Token）", "Rate (USD/M tokens)", "السعر (دولار/مليون رمز)")}</th>
                        <th>{text("说明", "Description", "الوصف")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr>
                        <td>{text("标准", "Standard", "قياسي")}</td>
                        <td className="mono" dir="ltr">0 – 32K</td>
                        <td className="mono">${amounts?.input ?? "—"}</td>
                        <td>{text("输入不超过 32K Token", "Input up to 32K tokens", "الإدخال حتى 32K رمز")}</td>
                      </tr>
                      <tr>
                        <td>{text("超额", "Excess", "الفائض")}</td>
                        <td className="mono" dir="ltr">&gt; 32K</td>
                        <td className="mono">${model.pricing.per_request.toFixed(6)}</td>
                        <td>{text("输入超过 32K Token 后按更低费率", "Reduced rate beyond 32K tokens", "سعر مخفض يتجاوز 32K رمز")}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p className="access-muted">
                  {text(
                    "阶梯定价仅在模型配置了 per_request 价格时生效。实际阶梯阈值和费率以后端返回的定价配置为准。",
                    "Tiered pricing applies only when the model has a per_request price configured. Actual thresholds and rates are determined by backend pricing configuration.",
                    "يسري التسعير المتدرج فقط عندما يكون للنموذج سعر per_request مكون. تحدد إعدادات تسعير الخادم الحدود والأسعار الفعلية.",
                  )}
                </p>
              </section>
            )}
            {(() => {
              const cfg = useModelPricing.getState().config;
              const entry = cfg?.models?.[model.id];
              const windows = entry?.rule?.timeWindows;
              if (!windows || windows.length === 0) return null;
              const dayNames = text("周日,周一,周二,周三,周四,周五,周六", "Sun,Mon,Tue,Wed,Thu,Fri,Sat", "أحد,إثنين,ثلاثاء,أربعاء,خميس,جمعة,سبت").split(",");
              return (
                <section className="access-panel">
                  <h2>
                    {text("分时定价", "Time-based pricing", "تسعير زمني")}
                  </h2>
                  <p className="access-muted">
                    {text(
                      "此模型配置了分时定价窗口，不同时段的费率倍率不同。下方为平台配置的分时窗口及其倍率。",
                      "This model has time-based pricing windows with different rate multipliers. Below are the configured windows and their multipliers.",
                      "يحتوي هذا النموذج على نوافذ تسعير زمنية بمضاعفات أسعار مختلفة. أدناه النوافذ المكونة ومضاعفاتها.",
                    )}
                  </p>
                  <div className="access-table-wrap" tabIndex={0} role="region">
                    <table className="access-table">
                      <thead>
                        <tr>
                          <th>{text("时段名称", "Window label", "اسم النافذة")}</th>
                          <th>{text("适用日", "Days", "الأيام")}</th>
                          <th>{text("时间", "Time", "الوقت")}</th>
                          <th>{text("时区", "Timezone", "المنطقة الزمنية")}</th>
                          <th>{text("倍率", "Multiplier", "المضاعف")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {windows.map((w, i) => (
                          <tr key={i}>
                            <td>{w.label}</td>
                            <td className="mono" style={{ fontSize: "var(--text-xs)" }}>
                              {w.days.map((d) => dayNames[d] || d).join(", ")}
                            </td>
                            <td className="mono">{w.start} – {w.end}</td>
                            <td className="mono" style={{ fontSize: "var(--text-xs)" }}>{w.timezone}</td>
                            <td className="mono">{w.multiplier}×</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <p className="access-muted">
                    {text(
                      "分时定价仅在模型配置了 timeWindows 时生效。实际费率以平台配置为准。",
                      "Time-based pricing applies only when the model has timeWindows configured. Actual rates are determined by platform configuration.",
                      "يسري التسعير الزمني فقط عندما يكون للنموذج timeWindows مكونة. تحدد إعدادات المنصة الأسعار الفعلية.",
                    )}
                  </p>
                </section>
              );
            })()}
            {model.pricing.mode === "per_request" && model.pricing.per_request !== null && model.pricing.per_request > 0 && (
              <section className="access-panel">
                <h2>
                  {text("计量定价", "Per-request pricing", "تسعير لكل طلب")}
                </h2>
                <p className="access-muted">
                  {text(
                    "此模型按次计费，每次请求收取固定费用。实际费用取决于请求次数。",
                    "This model is billed per request at a fixed rate. Actual cost depends on the number of requests.",
                    "يُحاسب هذا النموذج لكل طلب بسعر ثابت. التكلفة الفعلية تعتمد على عدد الطلبات.",
                  )}
                </p>
                <div className="access-table-wrap" tabIndex={0} role="region">
                  <table className="access-table">
                    <thead>
                      <tr>
                        <th>{text("计费项", "Billing item", "بند الفوترة")}</th>
                        <th>{text("单价（美元/次）", "Rate (USD/request)", "السعر (دولار/طلب)")}</th>
                        <th>{text("说明", "Description", "الوصف")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr>
                        <td>{text("每次请求", "Per request", "لكل طلب")}</td>
                        <td className="mono">
                          ${model.pricing.per_request.toFixed(6)}
                        </td>
                        <td>
                          {text(
                            "无论用量多少，每次 API 请求收取固定费用",
                            "A fixed charge per API request regardless of usage",
                            "رسم ثابت لكل طلب API بغض النظر عن الاستخدام",
                          )}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </section>
            )}
            {supportsClient && (
              <section className="access-panel">
                <h2>
                  {text(
                    "在你的工具中选择模型",
                    "Select the model in your tool",
                    "اختر النموذج في أداتك",
                  )}
                </h2>
                <p>
                  {text(
                    "接入中心会根据目标工具生成正确的协议、地址和模型默认值。复制配置后，可发起模拟请求验证，再到调用明细查看实际模型与费用。",
                    "The connection center generates protocol, endpoint, and model defaults for your target tool. Validate with a simulated request and inspect the actual model and charge in request details.",
                    "ينشئ مركز الاتصال البروتوكول والعنوان والنموذج لأداتك. تحقق بطلب تجريبي ثم راجع النموذج الفعلي والتكلفة.",
                  )}
                </p>
                <Button asChild variant="primary" disabled={!canConnect}>
                  <Link
                    to={connectUrl}
                    aria-disabled={!canConnect}
                    tabIndex={canConnect ? 0 : -1}
                    onClick={(e) => {
                      if (!canConnect) e.preventDefault();
                    }}
                  >
                    {text(
                      "前往接入中心",
                      "Open connection center",
                      "فتح مركز الاتصال",
                    )}
                    <ArrowRight size={16} />
                  </Link>
                </Button>
              </section>
            )}
          </>
        )}
      </main>
    </div>
  );
}
