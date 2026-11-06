import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { useTranslation } from "react-i18next";
import type { StoredBillingDetail } from "../lib/request-metadata";
import "./settled-billing.css";

export function SettledBilling({
  detail,
  timezone = "Asia/Dubai",
}: {
  detail: StoredBillingDetail;
  timezone?: string;
}) {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language.startsWith("zh")
      ? zh
      : i18n.language.startsWith("ar")
        ? ar
        : en;
  const labels: Record<string, string> = {
    billable_units: c(...translatedMessage("billable_quantity_334cd7e05b")),
    input_tokens: c(...translatedMessage("uncached_input_6aa7dc96f3")),
    output_tokens: c(...translatedMessage("output_tokens_e7f8d19e9f")),
    cache_read_tokens: c(...translatedMessage("cache_read_tokens_312e947278")),
    cache_write_tokens: c(
      ...translatedMessage("cache_write_tokens_59ac22bb34"),
    ),
    input_audio_tokens: c(
      ...translatedMessage("audio_input_tokens_bb0687e630"),
    ),
    output_audio_tokens: c(
      ...translatedMessage("audio_output_tokens_2fe31e8a17"),
    ),
    requests: c(...translatedMessage("requests_2d568af110")),
    images: c(...translatedMessage("images_0693f36c63")),
    seconds: c(...translatedMessage("seconds_6ecd15ac6b")),
  };
  const units: Record<string, string> = {
    request: c(...translatedMessage("request_1084482b61")),
    image: c(...translatedMessage("image_3ac6426300")),
    second: c(...translatedMessage("second_458c9c7655")),
    unit: c(...translatedMessage("unit_9d80d4a6b3")),
  };
  return (
    <div className="settled-billing">
      <dt>{c(...translatedMessage("billing_at_request_time_28ce99eee6"))}</dt>
      <dd>
        <div className="usage-table-scroll">
          <table>
            <thead>
              <tr>
                <th>{c(...translatedMessage("meter_867b0c4eee"))}</th>
                <th>{c(...translatedMessage("quantity_ef0e0d42fd"))}</th>
                <th>
                  {c(...translatedMessage("captured_price_usd_a9c89d3921"))}
                </th>
              </tr>
            </thead>
            <tbody>
              {detail.parts.map((part, index) => (
                <tr key={index}>
                  <td>{labels[part.name] ?? part.name}</td>
                  <td className="mono">{part.quantity}</td>
                  <td className="mono">
                    {part.unit_price_usd ? `$${part.unit_price_usd}` : "—"}
                    {part.unit === "million_tokens"
                      ? c(...translatedMessage("1m_tokens_082e554cf2"))
                      : part.unit_price_usd
                        ? ` / ${units[part.unit] ?? part.unit}`
                        : ""}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <small className="request-rate-note">
          {c(
            ...translatedMessage(
              "rates_include_captured_multipliers_the_comple_2a2930e657",
            ),
          )}
        </small>
        <small>
          {new Date(detail.priced_at).toLocaleString(
            regionalLocale(i18n.language),
            {
              timeZone: timezone,
              timeZoneName: "short",
            },
          )}{" "}
          · {c(...translatedMessage("multiplier_20c44388ab"))}{" "}
          {detail.group_multiplier}
        </small>
        {detail.formula && (
          <code style={{ display: "block", overflowWrap: "anywhere" }}>
            {detail.formula}
          </code>
        )}
      </dd>
    </div>
  );
}
