import type { DirectoryModel } from "../config/model-directory";
import type { PricingCatalogConfig } from "../config/models";
import {
  getPublicCatalogPrice,
  type PublicCatalogPrice,
} from "../lib/catalog-public-pricing";
import { getOfficialReference } from "../lib/official-reference";
import { usePreferences } from "../lib/preferences";
import { serviceGroupLabel } from "../lib/service-group-label";
import { formatMoney } from "../lib/money";
import { describeDirectoryRate } from "./directory-price";
import { ExternalLink } from "lucide-react";
import "./catalog-price-comparison.css";
import {
  describePlazaPrice,
  plazaAmounts,
  plazaCatalogOffer,
  plazaOfficialReference,
  type PlazaQuote,
} from "../lib/plaza-pricing";

export function CatalogPriceComparison({
  model,
  config,
  publicPrice,
  plazaQuote,
  detailed = false,
}: {
  model: DirectoryModel;
  config: PricingCatalogConfig;
  publicPrice?: PublicCatalogPrice;
  plazaQuote?: PlazaQuote;
  detailed?: boolean;
}) {
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const offer = plazaQuote
    ? plazaCatalogOffer(plazaQuote)
    : (publicPrice ?? getPublicCatalogPrice(model, config));
  const reference = plazaQuote
    ? plazaOfficialReference(plazaQuote.model.pricing)
    : model.kind === "smart"
      ? null
      : getOfficialReference(model.id, config);
  const sample = model.id.startsWith("veyra-demo-");
  const officialLabel =
    reference?.status === "legacy"
      ? text("历史官方参考", "Historical official", "مرجع رسمي تاريخي")
      : text("官方参考价", "Official reference", "المرجع الرسمي");
  const publicLabel = plazaQuote
    ? text("当前分组报价", "Current group price", "سعر المجموعة الحالية")
    : offer.mode === "expression"
      ? text("公开最低示例费用", "Lowest public example", "أقل تكلفة مثال عامة")
      : model.kind === "smart"
        ? text("公开路由估算", "Public routing estimate", "تقدير التوجيه العام")
        : text("平台公开最低价", "Lowest public price", "أقل سعر عام");
  const noOfficial =
    model.kind === "smart"
      ? text("无独立官价", "No standalone rate", "لا يوجد سعر مستقل")
      : sample
        ? text("平台计费样例", "Platform billing sample", "مثال فوترة المنصة")
        : text("尚未核实", "Not verified", "غير موثق");
  const statusLabels = {
    unknown: text("模型不存在", "Model not found", "النموذج غير موجود"),
    offline: text("已下架", "Retired", "متوقف"),
    "no-public-group": text(
      "暂无公开报价",
      "No public offer",
      "لا يوجد عرض عام",
    ),
    "no-resource": text(
      "暂无公开可用资源",
      "No public resource",
      "لا يوجد مورد عام متاح",
    ),
    "quote-error": text(
      plazaQuote ? "报价暂不可用" : "暂无法确定最低价",
      plazaQuote ? "Quote unavailable" : "Minimum unavailable",
      plazaQuote ? "السعر غير متاح" : "تعذر تحديد أقل سعر",
    ),
    ready: "",
  };
  const rate = offer.status === "ready" ? offer.price?.rates[0] : undefined;
  const info = plazaQuote
    ? describePlazaPrice(plazaQuote.model.pricing, lang)
    : rate
      ? describeDirectoryRate(rate, lang)
      : null;
  const amounts = plazaQuote ? plazaAmounts(plazaQuote.model.pricing) : rate;
  const tokenRate = plazaQuote
    ? plazaQuote.model.pricing.mode === "per_token"
    : rate?.rule.mode === "tokens";
  const referenceTokens =
    reference?.rate.mode === "tokens" ? reference.rate.pricing : null;
  const tokens =
    model.kind !== "smart" &&
    (!reference || reference.rate.mode === "tokens") &&
    (tokenRate || (!plazaQuote && !rate && referenceTokens));
  const smartTokenExample = offer.price?.rates.every(
    (candidate) =>
      candidate.rule.mode === "tokens" &&
      candidate.usage.input_tokens === 1000 &&
      candidate.usage.output_tokens === 300 &&
      !candidate.usage.images &&
      !candidate.usage.seconds,
  );
  const officialNumber = (value: string | number | undefined) =>
    value === undefined ? "—" : `$${value}`;
  const rows = [
    {
      label: text("输入", "Input", "إدخال"),
      official: referenceTokens?.input_per_million_usd,
      platform: tokenRate ? amounts?.input : undefined,
    },
    {
      label: text("输出", "Output", "إخراج"),
      official: referenceTokens?.output_per_million_usd,
      platform: tokenRate ? amounts?.output : undefined,
    },
    ...(referenceTokens?.cache_read_per_million_usd !== undefined ||
    (tokenRate && amounts?.cacheRead !== undefined)
      ? [
          {
            label: text("缓存读", "Cache read", "قراءة الذاكرة"),
            official: referenceTokens?.cache_read_per_million_usd,
            platform: tokenRate ? amounts?.cacheRead : undefined,
          },
        ]
      : []),
    ...(detailed &&
    (referenceTokens?.cache_write_per_million_usd !== undefined ||
      (tokenRate && amounts?.cacheWrite !== undefined))
      ? [
          {
            label: text("缓存写", "Cache write", "كتابة الذاكرة"),
            official: referenceTokens?.cache_write_per_million_usd,
            platform: tokenRate ? amounts?.cacheWrite : undefined,
          },
        ]
      : []),
  ];
  const officialSummary =
    reference?.rate.mode === "tokens"
      ? `$${reference.rate.pricing.input_per_million_usd} / $${reference.rate.pricing.output_per_million_usd}`
      : reference?.rate.mode === "unit"
        ? `$${reference.rate.priceUsd}`
        : noOfficial;
  const unit = (value: "request" | "image" | "second") =>
    ({
      request: text("美元 / 次", "USD / request", "دولار / طلب"),
      image: text("美元 / 张", "USD / image", "دولار / صورة"),
      second: text("美元 / 秒", "USD / second", "دولار / ثانية"),
    })[value];
  const officialUnit =
    reference?.rate.mode === "tokens"
      ? text(
          "输入 / 输出 · 美元 / 百万词元",
          "Input / output · USD / million tokens",
          "إدخال / إخراج · دولار / مليون رمز",
        )
      : reference?.rate.mode === "unit"
        ? unit(reference.rate.unit)
        : "";
  const isModeMismatch = Boolean(
    reference &&
    rate &&
    (reference.rate.mode !== rate.rule.mode ||
      (reference.rate.mode === "unit" &&
        rate.rule.mode === "unit" &&
        reference.rate.unit !== rate.rule.unit)),
  );
  return (
    <div
      className="catalog-model-price price-comparison"
      data-public-group={plazaQuote?.group ?? offer.groupId ?? ""}
      data-public-status={offer.status}
    >
      {model.kind === "smart" ? (
        <div className="price-routing">
          <span className="price-comparison-label" data-price-kind="official">
            {noOfficial} ·{" "}
            {text(
              "按实际模型计费",
              "Billed at actual model rates",
              "بسعر النموذج الفعلي",
            )}
          </span>
          <div data-price-kind="public">
            <small>{publicLabel}</small>
            <strong dir={offer.status === "ready" ? "ltr" : undefined}>
              {offer.status === "ready"
                ? `${formatMoney(offer.price!.minAmountNano!, "call")}–${formatMoney(offer.price!.maxAmountNano!, "call")}`
                : statusLabels[offer.status]}
            </strong>
          </div>
          <small>
            {text(
              offer.status !== "ready"
                ? "公开分组报价暂不可用，可进入详情检查。"
                : smartTokenExample
                  ? "估算 / 次：1,000 输入 + 300 输出词元，无缓存。"
                  : "混合计价样例；每个候选的用量条件见下方。",
              offer.status !== "ready"
                ? "Public quote unavailable; inspect details."
                : smartTokenExample
                  ? "Estimate / call: 1,000 input + 300 output tokens, no cache."
                  : "Mixed billing examples; candidate usage assumptions below.",
              offer.status !== "ready"
                ? "التسعير العام غير متاح؛ راجع التفاصيل."
                : smartTokenExample
                  ? "تقدير لكل طلب: 1000 إدخال + 300 إخراج بلا تخزين."
                  : "أمثلة فوترة مختلطة؛ افتراضات استخدام المرشحين أدناه.",
            )}
          </small>
        </div>
      ) : tokens ? (
        <>
          <div className="price-comparison-unit">
            {text(
              "美元 / 百万词元",
              "USD / million tokens",
              "دولار / مليون رمز",
            )}
          </div>
          <table className="price-comparison-table">
            <caption className="sr-only">
              {text(
                `${model.name} 价格对照`,
                `${model.name} price comparison`,
                `مقارنة أسعار ${model.name}`,
              )}
            </caption>
            <thead>
              <tr>
                <th scope="col">{text("用量", "Usage", "الاستخدام")}</th>
                <th scope="col">{officialLabel}</th>
                <th scope="col">{publicLabel}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.label}>
                  <th scope="row">{row.label}</th>
                  <td data-price-kind="official">
                    <bdi dir="ltr">{officialNumber(row.official)}</bdi>
                  </td>
                  <td data-price-kind="public">
                    <bdi dir="ltr">{officialNumber(row.platform)}</bdi>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      ) : (
        <div className="price-comparison-pair">
          <section data-price-kind="official">
            <h3>{officialLabel}</h3>
            <strong dir={reference ? "ltr" : undefined}>
              {officialSummary}
            </strong>
            <small>{officialUnit}</small>
          </section>
          <section data-price-kind="public">
            <h3>{publicLabel}</h3>
            <strong dir={info ? "ltr" : undefined}>
              {info?.value ?? statusLabels[offer.status]}
            </strong>
            <small>{info?.unit}</small>
            {info?.mode && <small>{info.mode}</small>}
          </section>
        </div>
      )}
      {offer.status !== "ready" && tokens && (
        <p className="price-public-status">{statusLabels[offer.status]}</p>
      )}
      <div className="price-comparison-meta">
        {model.kind !== "smart" && (
          <div className="price-official-meta">
            {reference && "sourceUrl" in reference ? (
              <>
                <a href={reference.sourceUrl} target="_blank" rel="noreferrer">
                  {text("官方来源", "Official source", "المصدر الرسمي")}
                  <ExternalLink size={10} />
                </a>
                <span>
                  {text("核验", "Verified", "تم التحقق")}{" "}
                  <time dateTime={reference.verifiedAt}>
                    {reference.verifiedAt}
                  </time>
                </span>
                {reference.status === "legacy" && (
                  <span className="price-legacy">
                    {text(
                      "已退役 · 历史报价",
                      "Retired · historical rate",
                      "متوقف · سعر تاريخي",
                    )}
                  </span>
                )}
              </>
            ) : (
              <span>
                {text("官方参考：", "Official reference: ", "المرجع الرسمي: ")}
                {plazaQuote && reference
                  ? text(
                      "管理员维护，适用条件请向平台确认。",
                      "Maintained by the platform; confirm applicable conditions.",
                      "يديره المسؤول؛ تحقق من الشروط مع المنصة.",
                    )
                  : noOfficial}
              </span>
            )}
          </div>
        )}
        {(plazaQuote?.group || offer.groupId) && (
          <p className="price-public-source">
            {plazaQuote?.group ??
              serviceGroupLabel(offer.groupId!, lang, config)}{" "}
            ·{" "}
            {text(
              plazaQuote ? "后端报价 · 已包含分组倍率" : "当前时段 · 模拟售价",
              plazaQuote
                ? "Backend quote · group multiplier included"
                : "Current window · demo rate",
              plazaQuote
                ? "سعر الخادم شامل معامل المجموعة"
                : "الفترة الحالية · سعر تجريبي",
            )}
          </p>
        )}
        {reference &&
          "scope" in reference &&
          (detailed ? (
            <p className="price-reference-scope">{reference.scope[lang]}</p>
          ) : (
            <details className="price-conditions">
              <summary>
                {text(
                  "官价适用条件",
                  "Official rate conditions",
                  "شروط السعر الرسمي",
                )}
              </summary>
              <p className="price-reference-scope">{reference.scope[lang]}</p>
            </details>
          ))}
        {model.kind === "smart" &&
          offer.price &&
          (!smartTokenExample ||
            offer.price.rates.some(
              (candidate) => candidate.rule.timeWindows?.length,
            )) && (
            <details className="price-conditions">
              <summary>
                {text(
                  "候选估算条件",
                  "Candidate assumptions",
                  "افتراضات المرشحين",
                )}
              </summary>
              {offer.price.rates.map((candidate) => {
                const candidateInfo = describeDirectoryRate(candidate, lang);
                return (
                  <p key={candidate.model.id}>
                    {candidate.model.name} · {candidateInfo.assumption} ·{" "}
                    {candidateInfo.estimate} {candidateInfo.schedule}
                  </p>
                );
              })}
            </details>
          )}
        {isModeMismatch && (
          <p className="price-reference-scope">
            {text(
              "两侧计费方式或单位不同，仅分别参考，不计算折扣。",
              "Different billing modes or units; shown separately, no discount comparison.",
              "طرق أو وحدات فوترة مختلفة؛ للمراجعة المنفصلة دون مقارنة خصم.",
            )}
          </p>
        )}
        {info &&
          "assumption" in info &&
          rate?.rule.mode !== "tokens" &&
          model.kind !== "smart" && (
            <p className="price-example-scope">
              {info.assumption} {rate?.rule.mode === "unit" ? info.details : ""}
            </p>
          )}
        {info && "schedule" in info && info.schedule && (
          <p className="price-time-scope">{info.schedule}</p>
        )}
        {model.kind === "smart" && offer.groupId && (
          <p className="price-reference-scope">
            {text(
              "区间仅含上述分组候选；按示例费用选择公开分组，实际还受密钥授权与请求能力约束。",
              "Range covers candidates in this group, selected by example cost. Key permissions and request capabilities still apply.",
              "النطاق لمرشحي هذه المجموعة المختارة بتكلفة المثال؛ تخضع النتيجة لصلاحيات المفتاح وقدرات الطلب.",
            )}
          </p>
        )}
        {offer.status === "quote-error" && (
          <details className="price-error-details">
            <summary>
              {text("查看报价原因", "Quote details", "تفاصيل التسعير")}
            </summary>
            <ul>
              {offer.errors.map((error, index) => (
                <li key={index}>
                  {error.modelId} · {error.message}
                </li>
              ))}
            </ul>
          </details>
        )}
        {tokens && detailed && (
          <p className="price-reference-scope">
            {text(
              plazaQuote
                ? "— 表示未配置该项报价，不代表免费。平台价格取自后端当前分组，已包含分组倍率；不自动切换当前密钥分组。"
                : "— 表示未配置该项报价，不代表免费。平台同一列取自同一公开分组，倍率仅计算一次；不自动切换当前密钥分组。",
              plazaQuote
                ? "— means no configured rate, not free. Backend prices include the current group multiplier; your key group is not changed."
                : "— means no configured rate, not free. Platform rates come from one public group, with its multiplier applied once; your key group is not changed.",
              plazaQuote
                ? "— تعني عدم وجود سعر وليست مجانية. أسعار الخادم شاملة معامل المجموعة الحالية؛ لا تتغير مجموعة مفتاحك."
                : "— تعني عدم وجود سعر وليست مجانية. أسعار المنصة من مجموعة عامة واحدة ومعامل واحد؛ لا تتغير مجموعة مفتاحك.",
            )}
          </p>
        )}
      </div>
    </div>
  );
}
