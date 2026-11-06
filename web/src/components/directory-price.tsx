import {
  localizedMessage,
  message as translatedMessage,
} from "../i18n/messages";

import type { DirectoryRate } from "../config/model-directory";
import type { Language } from "../config/placeholders";
import { multiplyDecimalPrice } from "../config/models";
import { formatMoney } from "../lib/money";
import { describePlazaPrice } from "../lib/plaza-pricing";
import "./directory-price.css";

export function describeDirectoryRate(rate: DirectoryRate, lang: Language) {
  const t = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const rule = rate.rule;
  const mode =
    rule.mode === "tokens"
      ? t(...translatedMessage("token_pricing_3a08771a10"))
      : rule.mode === "expression"
        ? t(...translatedMessage("expression_pricing_8bfbbcc34f"))
        : {
            request: t(...translatedMessage("per_request_f6e4e2b1f7")),
            image: t(...translatedMessage("per_image_c4bd6d3c05")),
            second: t(...translatedMessage("per_second_cb7de61b04")),
          }[rule.unit];
  const usage = rate.usage;
  const basicAssumption =
    rule.mode === "expression"
      ? t(
          ...translatedMessage(
            "example_usage_input_output_0_cache_tokens_ima_1da50a5415",
            {
              p0: usage.input_tokens,
              p1: usage.output_tokens,
              p2: usage.images,
              p3: usage.seconds,
            },
          ),
        )
      : rule.mode === "tokens"
        ? t(
            ...translatedMessage(
              "example_1_000_input_300_output_tokens_no_cach_c112c85e31",
            ),
          )
        : rule.unit === "request"
          ? t(...translatedMessage("example_1_request_7389bd8c3c"))
          : rule.unit === "image"
            ? t(...translatedMessage("example_1_image_6ec2c85f40"))
            : t(...translatedMessage("example_1_second_of_video_ee29dfafe6"));
  const assumption =
    basicAssumption +
    (rate.attributes
      ? t(
          ...translatedMessage(
            "example_specification_other_specifications_ma_77718740ac",
            { p0: JSON.stringify(rate.attributes) },
          ),
        )
      : "");
  if (!rate.quote)
    return {
      mode,
      value: t(...translatedMessage("estimate_unavailable_56a941063d")),
      unit:
        rate.error || t(...translatedMessage("pricing_unavailable_af643eccc2")),
      assumption,
      estimate: "—",
      schedule: "",
      details: "",
    };
  const quote = rate.quote;
  const schedule = quote.snapshot.matchedWindow
    ? t(
        ...translatedMessage("current_window_included_70985afe5d", {
          p0: quote.snapshot.matchedWindow.label,
          p1: quote.snapshot.matchedWindow.timezone,
          p2: quote.snapshot.timeMultiplier,
        }),
      )
    : rule.timeWindows?.length
      ? t(
          ...translatedMessage(
            "current_time_uses_the_standard_rate_1a60eccedb",
          ),
        )
      : "";
  const unitPrice =
    rule.mode === "unit"
      ? multiplyDecimalPrice(
          multiplyDecimalPrice(
            quote.snapshot.matchedCondition?.priceUsd ?? rule.priceUsd,
            quote.snapshot.groupMultiplier,
          ),
          quote.snapshot.timeMultiplier,
        )
      : "";
  return {
    mode,
    value:
      rule.mode === "tokens"
        ? `$${quote.rate.input} / $${quote.rate.output}`
        : rule.mode === "unit"
          ? `$${unitPrice}`
          : formatMoney(quote.amountNano, "call"),
    unit:
      rule.mode === "tokens"
        ? t(...translatedMessage("input_output_usd_million_tokens_f00f399943"))
        : rule.mode === "expression"
          ? t(
              ...translatedMessage(
                "example_cost_not_a_fixed_unit_price_f337d62414",
              ),
            )
          : {
              request: t(...translatedMessage("usd_request_5265075e68")),
              image: t(...translatedMessage("usd_image_efd0385640")),
              second: t(...translatedMessage("usd_second_b4496726da")),
            }[rule.unit],
    assumption,
    estimate: formatMoney(quote.amountNano, "call"),
    schedule,
    details:
      rule.mode === "unit"
        ? t(
            ...translatedMessage(
              "minimum_quantity_rounding_step_example_billed_ac6c79030a",
              {
                p0: rule.minimum,
                p1: rule.ceilTo,
                p2: quote.snapshot.billedQuantity,
              },
            ),
          )
        : rule.mode === "expression"
          ? rule.expression
          : "",
  };
}

export function DirectoryPrice({
  rate,
  plazaPricing,
  lang,
  compact = false,
}: {
  rate?: DirectoryRate;
  plazaPricing?: import("../lib/api/plaza").PlazaPricing;
  lang: Language;
  compact?: boolean;
}) {
  const info = plazaPricing
    ? describePlazaPrice(plazaPricing, lang)
    : describeDirectoryRate(rate!, lang);
  return (
    <div className={`directory-price${rate?.error ? " has-error" : ""}`}>
      <small>{info.mode}</small>
      <strong
        dir={
          (plazaPricing ? "hasAmount" in info && info.hasAmount : rate?.quote)
            ? "ltr"
            : undefined
        }
      >
        <bdi>{info.value}</bdi>
      </strong>
      <small>{info.unit}</small>
      {!compact &&
        (plazaPricing ? (
          <small>{info.details}</small>
        ) : (
          "assumption" in info && (
            <>
              <small>
                {info.assumption}{" "}
                {localizedMessage("example_cost_b219a924d8", lang)}
                {info.estimate}
              </small>
              {info.details && (
                <small
                  className={
                    rate?.rule.mode === "expression"
                      ? "directory-price-expression"
                      : ""
                  }
                >
                  {info.details}
                </small>
              )}
              {info.schedule && <small>{info.schedule}</small>}
            </>
          )
        ))}
      {compact && rate?.rule.mode === "expression" && "assumption" in info && (
        <small>{info.assumption}</small>
      )}
    </div>
  );
}
