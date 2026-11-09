import { CatalogPriceCopy } from "./catalog-price-copy";
import { message as translatedMessage } from "../i18n/messages";
import type { DirectoryModel } from "../config/model-directory";
import type { PricingCatalogConfig } from "../config/models";
import type { PublicCatalogPrice } from "../lib/catalog-public-pricing";
import { getOfficialReference } from "../lib/official-reference";
import { usePreferences } from "../lib/preferences";
import { describeDirectoryRate } from "./directory-price";
import "./catalog-price-summary.css";
import { TimePricingNote } from "./time-pricing-note";
import {
  describePlazaPrice,
  plazaAmounts,
  plazaCatalogOffer,
  plazaOfficialReference,
  type PlazaQuote,
} from "../lib/plaza-pricing";

/** Browse-level prices; sources, conditions and estimates belong to model detail. */
export function CatalogPriceSummary({
  model,
  config,
  publicPrice,
  plazaQuote,
}: {
  model: DirectoryModel;
  config: PricingCatalogConfig;
  publicPrice?: PublicCatalogPrice;
  plazaQuote?: PlazaQuote;
}) {
  const lang = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const offer = plazaQuote ? plazaCatalogOffer(plazaQuote) : publicPrice!;
  const reference = plazaQuote
    ? plazaOfficialReference(plazaQuote.model.pricing)
    : model.kind === "smart"
      ? null
      : getOfficialReference(model.id, config);
  const rate = offer.status === "ready" ? offer.price?.rates[0] : undefined;
  const info = plazaQuote
    ? describePlazaPrice(plazaQuote.model.pricing, lang)
    : rate
      ? describeDirectoryRate(rate, lang)
      : null;
  const amounts = plazaQuote ? plazaAmounts(plazaQuote.model.pricing) : rate;
  const officialLabel =
    reference?.status === "legacy"
      ? text(...translatedMessage("historical_c557e619e9"))
      : text(...translatedMessage("official_df6a837000"));
  const publicLabel =
    plazaQuote && !plazaQuote.model.publicCatalog
      ? text(...translatedMessage("platform_price_2386ba36c1"))
      : text(...translatedMessage("platform_from_6b1eaf2b5e"));
  const tokenUnit = text(...translatedMessage("usd_million_tokens_6f08728512"));
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
  const amount = (value: string | number | undefined) =>
    value === undefined ? "—" : `$${value}`;
  const unitLabel = (unit: "image" | "second" | "request") =>
    ({
      image: text(...translatedMessage("usd_image_efd0385640")),
      second: text(...translatedMessage("usd_second_b4496726da")),
      request: text(...translatedMessage("usd_request_5265075e68")),
    })[unit];
  const tokenComparison =
    (plazaQuote
      ? plazaQuote.model.pricing.mode === "per_token"
      : rate?.rule.mode === "tokens") &&
    (!reference || reference.rate.mode === "tokens");
  const officialTokens =
    reference?.rate.mode === "tokens" ? reference.rate.pricing : null;
  const specificationPrice =
    rate?.rule.mode === "unit" &&
    Boolean(
      rate.rule.conditionRates?.length ||
      Object.keys(rate.attributes ?? {}).length,
    );
  const platformValue = plazaQuote
    ? info?.value
    : rate?.rule.mode === "expression"
      ? text(...translatedMessage("usage_based_pricing_efcd71458e"))
      : specificationPrice
        ? text(...translatedMessage("specification_pricing_1af51af19c"))
        : (info?.value ??
          text(...translatedMessage("see_pricing_details_4b1fcb7b15")));
  const showPlatformAmount =
    (plazaQuote
      ? plazaQuote.model.pricing.mode !== "tiered"
      : rate?.rule.mode !== "expression") && !specificationPrice;

  return (
    <div
      className="catalog-model-price catalog-price-summary"
      data-public-group={plazaQuote?.group ?? offer.groupId ?? ""}
      data-public-status={offer.status}
    >
      {offer.status !== "ready" ? (
        <div className="catalog-price-summary-message" data-price-kind="public">
          <span className="catalog-price-summary-label">{publicLabel}</span>
          <span className="catalog-price-summary-value">
            {statusLabels[offer.status]}
          </span>
        </div>
      ) : model.kind === "smart" ? (
        <div className="catalog-price-summary-message" data-price-kind="public">
          <span className="catalog-price-summary-label">
            {text(...translatedMessage("routing_pricing_a4b1cdf202"))}
          </span>
          <span className="catalog-price-summary-value">
            {text(
              ...translatedMessage("billed_by_the_actual_model_a07b3e8d3d"),
            )}
          </span>
        </div>
      ) : plazaQuote?.model.pricing.mode === "tiered" ? (
        <div className="catalog-price-summary-message" data-price-kind="public">
          <span className="catalog-price-summary-label">
            {text(...translatedMessage("platform_pricing_524d3ef3f3"))}
          </span>
          <span className="catalog-price-summary-value">
            {text(...translatedMessage("conditional_pricing_8a72d20e3e"))}
          </span>
        </div>
      ) : tokenComparison ? (
        <>
          <span className="catalog-price-summary-unit">{tokenUnit}</span>
          <table className="catalog-price-summary-table">
            <caption className="sr-only">
              {text(
                ...translatedMessage("input_and_output_prices_5987a4aec3", {
                  p0: model.name,
                }),
              )}
            </caption>
            <thead>
              <tr>
                <th scope="col">
                  <span className="sr-only">
                    {text(...translatedMessage("usage_3404499089"))}
                  </span>
                </th>
                <th scope="col">{officialLabel}</th>
                <th scope="col">{publicLabel}</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <th scope="row">
                  {text(...translatedMessage("input_b79be3e58a"))}
                </th>
                <td data-price-kind="official">
                  <bdi dir="ltr">
                    {amount(officialTokens?.input_per_million_usd)}
                  </bdi>
                </td>
                <td data-price-kind="public">
                  <bdi dir="ltr">{amount(amounts?.input)}</bdi>
                </td>
              </tr>
              <tr>
                <th scope="row">
                  {text(...translatedMessage("output_a7b8383921"))}
                </th>
                <td data-price-kind="official">
                  <bdi dir="ltr">
                    {amount(officialTokens?.output_per_million_usd)}
                  </bdi>
                </td>
                <td data-price-kind="public">
                  <bdi dir="ltr">{amount(amounts?.output)}</bdi>
                </td>
              </tr>
            </tbody>
          </table>
          {plazaQuote && (
            <CatalogPriceCopy
              model={model.id}
              group={plazaQuote.group}
              pricing={plazaQuote.model.pricing}
            />
          )}
          {!reference && (
            <span className="catalog-price-summary-note">
              {text(
                ...translatedMessage("official_rate_unverified_e8b72d5163"),
              )}
            </span>
          )}
        </>
      ) : (
        <div className="catalog-price-summary-pair">
          <div data-price-kind="official">
            <span className="catalog-price-summary-label">{officialLabel}</span>
            <span className="catalog-price-summary-value">
              {reference ? (
                <bdi dir="ltr">
                  {reference.rate.mode === "tokens"
                    ? `${amount(reference.rate.pricing.input_per_million_usd)} / ${amount(reference.rate.pricing.output_per_million_usd)}`
                    : amount(reference.rate.priceUsd)}
                </bdi>
              ) : (
                "—"
              )}
            </span>
            <span className="catalog-price-summary-note">
              {reference
                ? reference.rate.mode === "tokens"
                  ? text(...translatedMessage("input_output_da9f3d8092")) +
                    tokenUnit
                  : unitLabel(reference.rate.unit)
                : text(...translatedMessage("unverified_6354adc7da"))}
            </span>
          </div>
          <div data-price-kind="public">
            <span className="catalog-price-summary-label">
              {showPlatformAmount
                ? publicLabel
                : text(...translatedMessage("platform_pricing_524d3ef3f3"))}
            </span>
            <span className="catalog-price-summary-value">
              {showPlatformAmount ? (
                <bdi dir="ltr">{platformValue}</bdi>
              ) : (
                platformValue
              )}
            </span>
            {showPlatformAmount && info?.unit && (
              <span className="catalog-price-summary-note">{info.unit}</span>
            )}
          </div>
        </div>
      )}
      {plazaQuote && (
        <TimePricingNote
          value={plazaQuote.model.pricing.time_pricing}
          compact
        />
      )}
    </div>
  );
}
