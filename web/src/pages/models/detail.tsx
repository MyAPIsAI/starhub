import { useState } from "react";
import { useParams, Link, useSearchParams } from "react-router-dom";
import {
  ArrowLeft,
  ArrowRight,
  KeyRound,
  Sparkles,
  Terminal,
} from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { Button } from "../../components/ui/button";
import { type DirectoryRate } from "../../config/model-directory";
import { plazaAmounts } from "../../lib/plaza-pricing";
import { CatalogPriceComparison } from "../../components/catalog-price-comparison";
import { ModelIdCopy } from "../../components/model-id-copy";
import { DirectoryPrice } from "../../components/directory-price";
import { formatMoney } from "../../lib/money";
import { usePreferences } from "../../lib/preferences";
import { useModelPricing } from "../../lib/model-pricing";
import { useProductPreview } from "../../lib/product-preview";
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
  const keys = useProductPreview((s) => s.keys);
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
                          <th>{text("辅助倍率", "Multiplier", "المعامل")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {tierPrices.map(
                          ({ tier: group, price: groupPrice }) => (
                            <tr key={group.id}>
                              <td>{group.id}</td>
                              <td>
                                {groupPrice?.available ? (
                                  <DirectoryPrice
                                    plazaPricing={model.pricing}
                                    lang={lang}
                                    compact
                                  />
                                ) : (
                                  text("不可用", "Unavailable", "غير متاح")
                                )}
                              </td>
                              <td>{model.pricing.group_ratio}×</td>
                            </tr>
                          ),
                        )}
                      </tbody>
                    </table>
                  </div>
                </details>
              )}
            </section>
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
