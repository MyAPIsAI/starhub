import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Copy } from "lucide-react";
import { message as translatedMessage } from "../../i18n/messages";
import { usePreferences } from "../../lib/preferences";
import {
  type PlazaDetailResponse,
  plazaToDirectoryModel,
} from "../../lib/api/plaza";
import { CatalogPriceComparison } from "../../components/catalog-price-comparison";
import { CatalogPriceCopy } from "../../components/catalog-price-copy";
import { DirectoryPrice } from "../../components/directory-price";
import { ServiceLevelValue } from "../../components/service-level-value";
import { copyText } from "../../features/code-example";
import { catalogText } from "./public-content";
import { catalogExamples } from "./public-examples";
import "./public-catalog.css";

export function PublicModelContent({
  detail,
}: {
  detail: PlazaDetailResponse;
}) {
  const { t } = useTranslation();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const copy = catalogText(text);
  const row = detail.model;
  const groups = row.group_prices ?? [];
  const [selection, select] = useState("");
  const selected =
    groups.find((g) => g.id === selection) ??
    groups.find((g) => g.id === row.price_group) ??
    groups[0];
  const selectedModel = plazaToDirectoryModel({
    ...row,
    pricing: selected?.pricing ?? row.pricing,
  });
  const status = detail.status;
  const statusLabel =
    status?.state === "normal"
      ? copy.normal
      : status?.state === "degraded"
        ? copy.degraded
        : status?.state === "outage"
          ? copy.outage
          : copy.insufficient;
  const examples = catalogExamples(row);
  const [exampleId, setExample] = useState("");
  const example = examples.find((item) => item.id === exampleId) ?? examples[0];
  const [language, setLanguage] = useState<"curl" | "python" | "javascript">(
    "curl",
  );
  const source =
    row.official_source && /^https?:\/\//i.test(row.official_source)
      ? row.official_source
      : undefined;
  const unit = text(...translatedMessage("usd_million_tokens_6f08728512"));
  return (
    <>
      {!row.is_available && (
        <div className="access-notice" role="status">
          {copy.unavailable}
        </div>
      )}
      <section className="access-detail-meta">
        <div>
          <small>{text(...translatedMessage("protocols_3a5fb4e26a"))}</small>
          <strong>{row.protocols.join(" / ") || "—"}</strong>
        </div>
        <div>
          <small>{text(...translatedMessage("context_9810531841"))}</small>
          <strong>
            <bdi>
              {row.context_length > 0
                ? row.context_length.toLocaleString()
                : "—"}
            </bdi>
          </strong>
        </div>
        <div>
          <small>{copy.status}</small>
          <strong data-catalog-status={status?.state ?? "insufficient_data"}>
            {statusLabel}
          </strong>
        </div>
      </section>
      <section className="access-panel catalog-public-status">
        <h2>{copy.status}</h2>
        <div className="catalog-status-values">
          <span>
            {text(...translatedMessage("success_5bc7e895a7"))}:{" "}
            <bdi>
              {status?.success_rate == null
                ? "—"
                : `${(status.success_rate * 100).toFixed(2)}%`}
            </bdi>
          </span>
          <span>
            TTFT P95:{" "}
            <bdi>
              {status?.ttft_p95_ms == null ? "—" : `${status.ttft_p95_ms} ms`}
            </bdi>
          </span>
          <span>
            {text(...translatedMessage("requests_fe89af9421"))}:{" "}
            <bdi>{status?.samples ?? "—"}</bdi>
          </span>
        </div>
        <p className="access-muted">{copy.statusHelp}</p>
      </section>
      <section className="access-panel catalog-group-prices">
        <h2>{copy.groups}</h2>
        <p className="access-muted">{copy.priceHelp}</p>
        <div
          className="access-table-wrap"
          role="region"
          tabIndex={0}
          aria-label={copy.groups}
        >
          <table className="access-table">
            <thead>
              <tr>
                <th>{copy.group}</th>
                <th>{copy.multiplier}</th>
                <th>{copy.price}</th>
                <th>{copy.availability}</th>
                <th>{copy.serviceLevel}</th>
              </tr>
            </thead>
            <tbody>
              {groups.map((group) => (
                <tr
                  key={group.id}
                  data-group-id={group.id}
                  className={!group.available ? "detail-group-unavailable" : ""}
                >
                  <th scope="row">
                    <strong>{group.name}</strong>
                    <code dir="ltr">{group.id}</code>
                  </th>
                  <td data-label={copy.multiplier}>
                    <bdi dir="ltr">× {group.multiplier}</bdi>
                  </td>
                  <td data-label={copy.price}>
                    {row.kind === "smart" ? (
                      copy.smartBilling
                    ) : (
                      <>
                        <DirectoryPrice
                          plazaPricing={group.pricing}
                          lang={lang}
                          compact
                        />
                        {group.pricing.mode === "per_token" && (
                          <div className="catalog-cache-prices">
                            <span>
                              {text(
                                ...translatedMessage("cache_read_718a34a974"),
                              )}
                              :{" "}
                              <bdi>
                                {group.pricing.cache_read_per_m == null
                                  ? "—"
                                  : `$${group.pricing.cache_read_per_m}`}
                              </bdi>
                            </span>
                            <span>
                              {text(
                                ...translatedMessage("cache_write_953608f06c"),
                              )}
                              :{" "}
                              <bdi>
                                {group.pricing.cache_write_per_m == null
                                  ? "—"
                                  : `$${group.pricing.cache_write_per_m}`}
                              </bdi>
                            </span>
                            {group.pricing.audio_input_per_m != null && (
                              <span>
                                {text(
                                  ...translatedMessage(
                                    "audio_input_1ed2703c7e",
                                  ),
                                )}
                                : <bdi>${group.pricing.audio_input_per_m}</bdi>
                              </span>
                            )}
                            {group.pricing.audio_output_per_m != null && (
                              <span>
                                {text(
                                  ...translatedMessage(
                                    "audio_output_f7c8200fa1",
                                  ),
                                )}
                                : <bdi>${group.pricing.audio_output_per_m}</bdi>
                              </span>
                            )}
                            <small>{unit}</small>
                          </div>
                        )}
                        <CatalogPriceCopy
                          model={row.id}
                          group={group.id}
                          pricing={group.pricing}
                        />
                      </>
                    )}
                  </td>
                  <td data-label={copy.availability}>
                    <span>
                      {group.available ? copy.available : copy.unavailable}
                    </span>
                  </td>
                  <td data-label={copy.serviceLevel}>
                    <ServiceLevelValue
                      value={group.service_level}
                      text={text}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      {row.kind !== "smart" && selected && (
        <section className="access-panel access-public-comparison">
          <div className="access-panel-heading">
            <h2>{copy.official}</h2>
            <label className="catalog-group-select">
              <span>{copy.currentGroup}</span>
              <select
                value={selected.id}
                onChange={(e) => select(e.target.value)}
              >
                {groups.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <p className="access-muted">{copy.officialHelp}</p>
          <CatalogPriceComparison
            model={selectedModel}
            plazaQuote={{ model: selectedModel, group: selected.id }}
            detailed
          />
          {source && (
            <p>
              <a href={source} target="_blank" rel="noopener noreferrer">
                {source}
              </a>
              {row.official_verified && (
                <span>
                  {" "}
                  · <bdi>{row.official_verified}</bdi>
                </span>
              )}
            </p>
          )}
          {row.official_historical && (
            <p>
              {text(...translatedMessage("historical_official_2796a345c9"))}
            </p>
          )}
          {row.official_conditions && <p>{row.official_conditions}</p>}
          {selected.pricing.mode === "tiered" && (
            <details className="access-details">
              <summary>{copy.conditional}</summary>
              <p>{copy.conditional}</p>
              {selected.pricing.expression && (
                <pre dir="ltr" tabIndex={0}>
                  <code>{selected.pricing.expression}</code>
                </pre>
              )}
            </details>
          )}
        </section>
      )}
      {example && (
        <section className="access-panel catalog-examples">
          <div className="access-panel-heading">
            <h2>{copy.examples}</h2>
            <Link to="/docs">{copy.documentation}</Link>
          </div>
          <p className="access-muted">{copy.exampleHelp}</p>
          <div className="catalog-example-controls">
            <label>
              <span>{copy.protocol}</span>
              <select
                value={example.id}
                onChange={(e) => setExample(e.target.value)}
              >
                {examples.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span>{copy.language}</span>
              <select
                value={language}
                onChange={(e) => setLanguage(e.target.value as typeof language)}
              >
                <option value="curl">cURL</option>
                <option value="python">Python</option>
                <option value="javascript">JavaScript</option>
              </select>
            </label>
            <button
              type="button"
              onClick={() =>
                void copyText(example[language], t("copied"), t("copyFailed"))
              }
            >
              <Copy size={14} />
              {t("copy")}
            </button>
          </div>
          <pre dir="ltr" tabIndex={0}>
            <code>{example[language]}</code>
          </pre>
        </section>
      )}
    </>
  );
}
