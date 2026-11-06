import { message as translatedMessage } from "../i18n/messages";
import type { PricingQuoteSnapshot } from "../lib/model-pricing";
import { usePreferences } from "../lib/preferences";

export function PricingSnapshot({
  snapshot,
}: {
  snapshot: PricingQuoteSnapshot;
}) {
  const language = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const { rule, usage, pricing } = snapshot;
  const labels = {
    input_tokens: text(
      ...translatedMessage("uncached_input_tokens_c001c308cc"),
    ),
    output_tokens: text(...translatedMessage("output_tokens_e8729227cd")),
    cache_read_tokens: text(
      ...translatedMessage("cache_read_tokens_ea19d27041"),
    ),
    cache_write_tokens: text(
      ...translatedMessage("cache_write_tokens_2e87c1619e"),
    ),
    images: text(...translatedMessage("images_0cc4309f02")),
    seconds: text(...translatedMessage("duration_seconds_7aa2eff257")),
    requests: text(...translatedMessage("requests_de8ae6d124")),
  };
  const unit =
    rule.mode === "unit"
      ? {
          request: text(...translatedMessage("request_1084482b61")),
          image: text(...translatedMessage("image_3ac6426300")),
          second: text(...translatedMessage("second_458c9c7655")),
        }[rule.unit]
      : "";
  return (
    <>
      <div>
        <dt>
          {text(...translatedMessage("metered_usage_snapshot_959973b235"))}
        </dt>
        <dd>
          {Object.entries(usage).map(([field, amount]) => (
            <div key={field}>
              {labels[field as keyof typeof labels]}：{amount?.toLocaleString()}
            </div>
          ))}
        </dd>
      </div>
      <div>
        <dt>
          {text(...translatedMessage("pricing_rule_at_request_c6e98f204c"))}
        </dt>
        <dd>
          {rule.mode === "tokens" ? (
            <>
              {text(
                ...translatedMessage("per_token_base_usd_1m_tokens_8fe5eace89"),
              )}
              <small className="request-rate-note">
                {text(...translatedMessage("input_output_8eeba908fe"))}：$
                {pricing.input_per_million_usd} / $
                {pricing.output_per_million_usd}
              </small>
              <small className="request-rate-note">
                {text(...translatedMessage("cache_read_write_84134346f7"))}：
                {pricing.cache_read_per_million_usd === undefined
                  ? "—"
                  : `$${pricing.cache_read_per_million_usd}`}{" "}
                /{" "}
                {pricing.cache_write_per_million_usd === undefined
                  ? "—"
                  : `$${pricing.cache_write_per_million_usd}`}
              </small>
            </>
          ) : rule.mode === "unit" ? (
            <>
              ${snapshot.matchedCondition?.priceUsd ?? rule.priceUsd} / {unit}
              {snapshot.matchedCondition && (
                <small className="request-rate-note">
                  {text(...translatedMessage("conditional_price_cac8576a56"))}：
                  {snapshot.matchedCondition.label} ·{" "}
                  {JSON.stringify(snapshot.attributes)}
                </small>
              )}
              <small className="request-rate-note">
                {text(...translatedMessage("minimum_rounding_step_88a65094d4"))}
                ：{rule.minimum} / {rule.ceilTo}
              </small>
              <small className="request-rate-note">
                {text(...translatedMessage("billed_quantity_458df63459"))}：
                {snapshot.billedQuantity} {unit}
              </small>
            </>
          ) : (
            <>
              <span>
                {text(
                  ...translatedMessage("expression_base_amount_usd_90797b4572"),
                )}
              </span>
              <code
                style={{ display: "block", overflowWrap: "anywhere" }}
                dir="ltr"
              >
                {rule.expression}
              </code>
            </>
          )}
        </dd>
      </div>
      <div>
        <dt>
          {text(...translatedMessage("settlement_coefficients_838c25bbae"))}
        </dt>
        <dd>
          {text(...translatedMessage("group_528d77dde5"))}{" "}
          {snapshot.groupMultiplier}× ·{" "}
          {text(...translatedMessage("time_window_ea1ddd4afa"))}{" "}
          {snapshot.timeMultiplier}×
          <small className="request-rate-note">
            {text(
              ...translatedMessage(
                "base_amount_group_time_coefficient_each_appli_dbe348c610",
              ),
            )}
          </small>
        </dd>
      </div>
      <div>
        <dt>
          {text(...translatedMessage("pricing_time_and_timezone_e5c5a08732"))}
        </dt>
        <dd>
          {new Date(snapshot.quotedAt).toLocaleString()}
          <small className="request-rate-note">
            {snapshot.matchedWindow
              ? `${snapshot.matchedWindow.label} · ${snapshot.matchedWindow.timezone} · ${snapshot.matchedWindow.start}–${snapshot.matchedWindow.end}`
              : text(
                  ...translatedMessage(
                    "no_window_matched_standard_rate_42c04ea42d",
                  ),
                )}
          </small>
        </dd>
      </div>
      <div>
        <dt>{text(...translatedMessage("price_source_1fbffadc9a"))}</dt>
        <dd>
          {snapshot.source === "sample"
            ? text(
                ...translatedMessage(
                  "demo_sample_not_verified_official_pricing_c59f3923b7",
                ),
              )
            : text(
                ...translatedMessage(
                  "official_reference_entered_by_admin_not_indep_44f186bba7",
                ),
              )}
          <small className="request-rate-note">{snapshot.version}</small>
        </dd>
      </div>
    </>
  );
}
