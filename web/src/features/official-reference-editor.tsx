import { message as translatedMessage } from "../i18n/messages";
import { ChevronDown, ExternalLink } from "lucide-react";
import type {
  ModelPricing,
  OfficialPriceReference,
  PricingCatalogConfig,
} from "../config/models";
import {
  getOfficialReference,
  officialReferenceError,
} from "../lib/official-reference";
import { useSettingsText } from "./settings-text";
import "../components/official-reference-editor.css";

export function OfficialReferenceEditor({
  modelId,
  config,
  disabled,
  onChange,
}: {
  modelId: string;
  config: PricingCatalogConfig;
  disabled: boolean;
  onChange: (reference: OfficialPriceReference | null) => void;
}) {
  const { text, lang } = useSettingsText();
  const stored = config.models[modelId]?.officialReference;
  const reference =
    stored === undefined ? getOfficialReference(modelId, config) : stored;
  const patch = (value: Partial<OfficialPriceReference>) =>
    reference && onChange({ ...reference, ...value });
  const labels: [keyof ModelPricing, string][] = [
    [
      "input_per_million_usd",
      text(...translatedMessage("official_input_2b23659fc8")),
    ],
    [
      "output_per_million_usd",
      text(...translatedMessage("official_output_6dcc5c2b44")),
    ],
    [
      "cache_read_per_million_usd",
      text(...translatedMessage("official_cache_read_2cf63dbb84")),
    ],
    [
      "cache_write_per_million_usd",
      text(...translatedMessage("official_cache_write_72ad659923")),
    ],
  ];
  const error = reference ? officialReferenceError(reference) : null;
  const errors = {
    shape: text(
      ...translatedMessage(
        "check_currency_reference_status_and_billing_u_78f90a1364",
      ),
    ),
    source: text(
      ...translatedMessage(
        "enter_a_valid_https_official_source_url_db98543c1b",
      ),
    ),
    date: text(
      ...translatedMessage(
        "enter_a_valid_verification_date_no_later_than_fa00a22dd8",
      ),
    ),
    scope: text(
      ...translatedMessage("enter_the_scope_up_to_1_200_characters_3223e7ab2a"),
    ),
    price: text(
      ...translatedMessage(
        "enter_non_negative_prices_with_up_to_6_decima_a2385d2c85",
      ),
    ),
  };
  const enable = () => {
    const rate: OfficialPriceReference["rate"] =
      config.models[modelId].rule.mode === "unit"
        ? { mode: "unit", unit: config.models[modelId].rule.unit, priceUsd: "" }
        : {
            mode: "tokens",
            pricing: { input_per_million_usd: "", output_per_million_usd: "" },
          };
    // Empty values force verification; never copy platform samples into an official reference.
    onChange({
      currency: "USD",
      status: "current",
      rate,
      scope: { zh: "", en: "", ar: "" },
      sourceUrl: "",
      verifiedAt: "",
    });
  };
  return (
    <details
      className="settings-details official-reference-editor"
      data-official-reference-editor={modelId}
    >
      <summary>
        <span>
          {text(...translatedMessage("official_provider_reference_fe8af7214a"))}
          <small>
            {reference
              ? reference.status === "legacy"
                ? text(...translatedMessage("historical_reference_7d6c6acd22"))
                : text(...translatedMessage("configured_3b32afda4a"))
              : text(...translatedMessage("unverified_hidden_b778b63a1e"))}
          </small>
        </span>
        <ChevronDown size={16} />
      </summary>
      <div className="official-reference-body">
        <div className="official-reference-toggle">
          <div>
            <strong>
              {text(
                ...translatedMessage(
                  "show_the_official_reference_in_the_catalog_e17d4d121c",
                ),
              )}
            </strong>
            <p>
              {text(
                ...translatedMessage(
                  "maintained_separately_from_platform_billing_n_cf5b14697c",
                ),
              )}
            </p>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={Boolean(reference)}
            aria-label={text(
              ...translatedMessage("show_official_reference_e03a764e99"),
            )}
            disabled={disabled}
            className="official-reference-switch"
            onClick={() => (reference ? onChange(null) : enable())}
          >
            <span />
          </button>
        </div>
        {reference && (
          <fieldset disabled={disabled}>
            <legend className="sr-only">
              {text(
                ...translatedMessage("official_reference_settings_7ed1ad2bf3"),
              )}
            </legend>
            <div className="settings-fields">
              <label>
                <span>
                  {text(...translatedMessage("reference_status_da6ebb3f4a"))}
                </span>
                <select
                  aria-label={text(
                    ...translatedMessage("reference_status_da6ebb3f4a"),
                  )}
                  value={reference.status}
                  onChange={(event) =>
                    patch({
                      status: event.target
                        .value as OfficialPriceReference["status"],
                    })
                  }
                >
                  <option value="current">
                    {text(
                      ...translatedMessage(
                        "available_at_verification_dad21bf32b",
                      ),
                    )}
                  </option>
                  <option value="legacy">
                    {text(
                      ...translatedMessage("historical_retired_d7e8413ef0"),
                    )}
                  </option>
                </select>
              </label>
              <label>
                <span>
                  {text(
                    ...translatedMessage("official_pricing_unit_3f5d41a5dc"),
                  )}
                </span>
                <select
                  aria-label={text(
                    ...translatedMessage("official_pricing_unit_3f5d41a5dc"),
                  )}
                  value={
                    reference.rate.mode === "tokens"
                      ? "tokens"
                      : reference.rate.unit
                  }
                  onChange={(event) =>
                    patch({
                      rate:
                        event.target.value === "tokens"
                          ? {
                              mode: "tokens",
                              pricing: {
                                input_per_million_usd: "",
                                output_per_million_usd: "",
                              },
                            }
                          : {
                              mode: "unit",
                              unit: event.target.value as
                                "request" | "image" | "second",
                              priceUsd: "",
                            },
                    })
                  }
                >
                  <option value="tokens">
                    {text(
                      ...translatedMessage("usd_million_tokens_6f08728512"),
                    )}
                  </option>
                  <option value="image">
                    {text(...translatedMessage("usd_image_efd0385640"))}
                  </option>
                  <option value="second">
                    {text(...translatedMessage("usd_second_b4496726da"))}
                  </option>
                  <option value="request">
                    {text(...translatedMessage("usd_request_5265075e68"))}
                  </option>
                </select>
              </label>
            </div>
            <div className="settings-fields">
              {reference.rate.mode === "tokens" ? (
                labels.map(([name, label], index) => {
                  const rate = reference.rate;
                  if (rate.mode !== "tokens") return null;
                  return (
                    <label key={name}>
                      <span>{label}</span>
                      <input
                        inputMode="decimal"
                        value={rate.pricing[name] ?? ""}
                        placeholder={
                          index > 1
                            ? text(
                                ...translatedMessage(
                                  "not_applicable_unquoted_6ff90a2245",
                                ),
                              )
                            : "0.00"
                        }
                        onChange={(event) =>
                          patch({
                            rate: {
                              ...rate,
                              pricing: {
                                ...rate.pricing,
                                [name]:
                                  index > 1 && event.target.value === ""
                                    ? undefined
                                    : event.target.value,
                              },
                            },
                          })
                        }
                      />
                    </label>
                  );
                })
              ) : (
                <label>
                  <span>
                    {text(
                      ...translatedMessage(
                        "official_unit_price_usd_6b29c183a6",
                      ),
                    )}
                  </span>
                  <input
                    inputMode="decimal"
                    value={reference.rate.priceUsd}
                    onChange={(event) =>
                      reference.rate.mode === "unit" &&
                      patch({
                        rate: {
                          ...reference.rate,
                          priceUsd: event.target.value,
                        },
                      })
                    }
                  />
                </label>
              )}
            </div>
            <div className="settings-fields">
              <label>
                <span>
                  {text(
                    ...translatedMessage(
                      "official_reference_source_url_70716d5fbc",
                    ),
                  )}
                </span>
                <input
                  type="url"
                  value={reference.sourceUrl}
                  placeholder="https://"
                  onChange={(event) => patch({ sourceUrl: event.target.value })}
                />
              </label>
              <label>
                <span>
                  {text(
                    ...translatedMessage(
                      "official_reference_verification_date_e5b1c98044",
                    ),
                  )}
                </span>
                <input
                  type="date"
                  value={reference.verifiedAt}
                  max={new Date().toISOString().slice(0, 10)}
                  onChange={(event) =>
                    patch({ verifiedAt: event.target.value })
                  }
                />
              </label>
            </div>
            <label className="official-reference-scope">
              <span>
                {text(
                  ...translatedMessage(
                    "scope_edits_are_displayed_as_entered_64213469f4",
                  ),
                )}
              </span>
              <textarea
                rows={3}
                maxLength={1200}
                value={reference.scope[lang]}
                placeholder={text(
                  ...translatedMessage(
                    "for_example_standard_paid_tier_prompts_200_00_895029e4f5",
                  ),
                )}
                onChange={(event) =>
                  patch({
                    scope: {
                      zh: event.target.value,
                      en: event.target.value,
                      ar: event.target.value,
                    },
                  })
                }
              />
            </label>
            <p className="pricing-help">
              {text(
                ...translatedMessage(
                  "show_one_verified_scope_do_not_mix_context_le_126b5b6337",
                ),
              )}
            </p>
            {!error && (
              <a
                className="official-reference-source"
                href={reference.sourceUrl}
                target="_blank"
                rel="noreferrer"
              >
                {text(...translatedMessage("check_official_source_f0d96b58bd"))}
                <ExternalLink size={13} />
              </a>
            )}
            {error && (
              <p className="settings-error" role="alert">
                {errors[error]}
              </p>
            )}
          </fieldset>
        )}
        {!reference && (
          <p className="pricing-help">
            {text(
              ...translatedMessage(
                "when_unconfigured_the_catalog_says_unverified_9e405d3275",
              ),
            )}
          </p>
        )}
      </div>
    </details>
  );
}
