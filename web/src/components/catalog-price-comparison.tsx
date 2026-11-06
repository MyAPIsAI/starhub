import { message as translatedMessage } from "../i18n/messages";
import type { DirectoryModel } from "../config/model-directory";
import type { PricingCatalogConfig } from "../config/models";
import {
  getPublicCatalogPrice,
  type PublicCatalogPrice,
} from "../lib/catalog-public-pricing";
import { getOfficialReference } from "../lib/official-reference";
import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import { serviceGroupLabel } from "../lib/service-group-label";
import { formatMoney } from "../lib/money";
import { describeDirectoryRate } from "./directory-price";
import { ExternalLink } from "lucide-react";
import "./catalog-price-comparison.css";
import { TimePricingNote } from "./time-pricing-note";
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
  detailed?: boolean;
} & (
  | { plazaQuote: PlazaQuote; config?: never; publicPrice?: never }
  | {
      plazaQuote?: undefined;
      config: PricingCatalogConfig;
      publicPrice?: PublicCatalogPrice;
    }
)) {
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
      ? text(...translatedMessage("historical_official_2796a345c9"))
      : text(...translatedMessage("official_reference_7896199b9d"));
  const publicLabel = plazaQuote
    ? text(...translatedMessage("current_group_price_b2976b5609"))
    : offer.mode === "expression"
      ? text(...translatedMessage("lowest_public_example_07e49dac28"))
      : model.kind === "smart"
        ? text(...translatedMessage("public_routing_estimate_209ae9c5ed"))
        : text(...translatedMessage("lowest_public_price_711c110703"));
  const noOfficial =
    model.kind === "smart"
      ? text(...translatedMessage("no_standalone_rate_77f7d40322"))
      : sample
        ? text(...translatedMessage("platform_billing_sample_8018811053"))
        : text(...translatedMessage("not_verified_ef7f7c9dda"));
  const statusLabels = {
    unknown: text(...translatedMessage("model_not_found_d96fc188df")),
    offline: text(...translatedMessage("retired_e4771f7b44")),
    "no-public-group": text(...translatedMessage("no_public_offer_0104dbd32f")),
    "no-resource": text(...translatedMessage("no_public_resource_c1206fa683")),
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
      label: text(...translatedMessage("input_b79be3e58a")),
      official: referenceTokens?.input_per_million_usd,
      platform: tokenRate ? amounts?.input : undefined,
    },
    {
      label: text(...translatedMessage("output_a7b8383921")),
      official: referenceTokens?.output_per_million_usd,
      platform: tokenRate ? amounts?.output : undefined,
    },
    ...(referenceTokens?.cache_read_per_million_usd !== undefined ||
    (tokenRate && amounts?.cacheRead !== undefined)
      ? [
          {
            label: text(...translatedMessage("cache_read_718a34a974")),
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
            label: text(...translatedMessage("cache_write_953608f06c")),
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
      request: text(...translatedMessage("usd_request_5265075e68")),
      image: text(...translatedMessage("usd_image_efd0385640")),
      second: text(...translatedMessage("usd_second_b4496726da")),
    })[value];
  const officialUnit =
    reference?.rate.mode === "tokens"
      ? text(...translatedMessage("input_output_usd_million_tokens_f00f399943"))
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
              ...translatedMessage("billed_at_actual_model_rates_78e112a0e8"),
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
            {text(...translatedMessage("usd_million_tokens_6f08728512"))}
          </div>
          <table className="price-comparison-table">
            <caption className="sr-only">
              {text(
                ...translatedMessage("price_comparison_2b07fe7ea5", {
                  p0: model.name,
                }),
              )}
            </caption>
            <thead>
              <tr>
                <th scope="col">
                  {text(...translatedMessage("usage_3404499089"))}
                </th>
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
                  {text(...translatedMessage("official_source_8d6474e2d0"))}
                  <ExternalLink size={10} />
                </a>
                <span>
                  {text(...translatedMessage("verified_04056797df"))}{" "}
                  <time dateTime={reference.verifiedAt}>
                    {reference.verifiedAt}
                  </time>
                </span>
                {reference.status === "legacy" && (
                  <span className="price-legacy">
                    {text(
                      ...translatedMessage(
                        "retired_historical_rate_3f3a2df50c",
                      ),
                    )}
                  </span>
                )}
              </>
            ) : (
              <span>
                {text(...translatedMessage("official_reference_9e908bc965"))}
                {plazaQuote && reference
                  ? text(
                      ...translatedMessage(
                        "maintained_by_the_platform_confirm_applicable_576bbf7554",
                      ),
                    )
                  : noOfficial}
              </span>
            )}
          </div>
        )}
        {(plazaQuote?.group || offer.groupId) && (
          <p className="price-public-source">
            {plazaQuote
              ? plazaQuote.group
              : serviceGroupLabel(offer.groupId!, lang, config)}{" "}
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
            <p className="price-reference-scope">
              {reference.scope[writtenLanguage(lang)]}
            </p>
          ) : (
            <details className="price-conditions">
              <summary>
                {text(
                  ...translatedMessage("official_rate_conditions_d5097dbaf5"),
                )}
              </summary>
              <p className="price-reference-scope">
                {reference.scope[writtenLanguage(lang)]}
              </p>
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
                {text(...translatedMessage("candidate_assumptions_b94277c5de"))}
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
              ...translatedMessage(
                "different_billing_modes_or_units_shown_separa_0db9759778",
              ),
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
              ...translatedMessage(
                "range_covers_candidates_in_this_group_selecte_49a1ffe4e1",
              ),
            )}
          </p>
        )}
        {offer.status === "quote-error" && (
          <details className="price-error-details">
            <summary>
              {text(...translatedMessage("quote_details_32de235f56"))}
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
        {plazaQuote && (
          <TimePricingNote
            value={plazaQuote.model.pricing.time_pricing}
            compact={!detailed}
          />
        )}
      </div>
    </div>
  );
}
