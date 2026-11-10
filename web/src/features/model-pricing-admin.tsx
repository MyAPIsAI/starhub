import { message as translatedMessage } from "../i18n/messages";
import {
  quotePricingRule,
  examplePricingAttributes,
} from "../lib/pricing-rules";
import { modelCatalog } from "../lib/platform-catalog";
import {
  PricingConditions,
  PricingConditionInputs,
} from "./pricing-conditions";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import {
  ArrowUpRight,
  Check,
  ChevronDown,
  CircleDollarSign,
  Clock3,
  FlaskConical,
  Info,
  Plus,
  Trash2,
} from "lucide-react";
import {
  type ModelPricing,
  type PricingAttributes,
  type PricingCatalogConfig,
  type PricingRule,
  type PricingTimeWindow,
} from "../config/models";
import {
  isValidPrice,
  pricingRuleError,
  quoteModelCharge,
} from "../lib/model-pricing";
import {
  initializeDemoPreview,
  useProductPreview,
} from "../lib/product-preview";
import { useBackendKeys } from "../lib/api/key-adapter";
import {
  useOptionStatus,
  useOptionHistory,
  useRollbackOption,
} from "../lib/api/option";
import { ApiError } from "../lib/api/http";
import { getKeyModels } from "../lib/key-model-policy";
import { formatMoney } from "../lib/money";
import { serviceGroupLabel } from "../lib/service-group-label";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { usePricingDraft, useSettingsText } from "./pricing-settings-shared";
import { OfficialReferenceEditor } from "./official-reference-editor";
import "../components/model-pricing.css";
import "../components/settings-pricing.css";

type Usage = {
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  images: number;
  seconds: number;
  requests: number;
};
const sampleUsage: Usage = {
  input_tokens: 1000,
  output_tokens: 500,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  images: 1,
  seconds: 5,
  requests: 1,
};
const localDateTime = () => {
  const now = new Date();
  return new Date(now.getTime() - now.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
};
const modeName = (
  rule: PricingRule,
  text: (zh: string, en: string, ar: string) => string,
) =>
  rule.mode === "tokens"
    ? text(...translatedMessage("per_token_7f44ff66c4"))
    : rule.mode === "unit"
      ? {
          request: text(...translatedMessage("per_request_63e05e97c4")),
          image: text(...translatedMessage("per_image_5919d8daf4")),
          second: text(...translatedMessage("per_second_7be6dbed21")),
        }[rule.unit]
      : text(...translatedMessage("expression_396d42fc66"));

export function ModelPricingAdmin({
  role,
  modelId,
  embedded = false,
}: {
  role: number;
  modelId?: string;
  embedded?: boolean;
}) {
  useEffect(initializeDemoPreview, []);
  const state = usePricingDraft("models", role, modelId);
  const {
    config,
    base,
    draft,
    setDraft,
    dirty,
    conflict,
    valid: structurallyValid,
    canEdit,
    notice,
    reviewOpen,
    setReviewOpen,
    discard,
    publish,
    text,
  } = state;
  const catalogModels = modelCatalog(config);
  const { lang } = useSettingsText();
  const keys = useBackendKeys().keys;
  const balance = useProductPreview((s) => s.balance);
  const recordMeteredCall = useProductPreview((s) => s.recordMeteredCall);
  const optionStatusQuery = useOptionStatus();
  const optionHistoryQuery = useOptionHistory(20);
  const rollbackMut = useRollbackOption();
  const [historyOpen, setHistoryOpen] = useState(false);
  const [rollbackNotice, setRollbackNotice] = useState("");
  const [initialQuote] = useState(() => {
    try {
      const retry = sessionStorage.getItem("veyra-metered-retry-v1");
      const saved =
        retry ?? sessionStorage.getItem("veyra-metered-simulator-v1");
      if (retry) sessionStorage.removeItem("veyra-metered-retry-v1");
      if (saved) {
        const parsed = JSON.parse(saved) as {
          modelId?: string;
          keyId?: string;
          usage?: Partial<Usage>;
          at?: string;
          attributes?: PricingAttributes;
        };
        const timestamp = new Date(parsed.at ?? "");
        return {
          modelId: catalogModels.some((item) => item.id === parsed.modelId)
            ? parsed.modelId!
            : catalogModels[0].id,
          keyId: parsed.keyId ?? "",
          attributes: parsed.attributes ?? {},
          usage: { ...sampleUsage, ...parsed.usage },
          at: Number.isFinite(timestamp.getTime())
            ? new Date(
                timestamp.getTime() - timestamp.getTimezoneOffset() * 60000,
              )
                .toISOString()
                .slice(0, 16)
            : localDateTime(),
        };
      }
    } catch {
      /* Malformed simulator storage can be ignored without changing business state. */
    }
    return {
      modelId: catalogModels[0].id,
      keyId: "",
      attributes: {} as PricingAttributes,
      usage: sampleUsage,
      at: localDateTime(),
    };
  });
  const [selected, setSelected] = useState(modelId ?? initialQuote.modelId);
  const [selectedKey, setSelectedKey] = useState(initialQuote.keyId);
  const [usage, setUsage] = useState<Usage>(initialQuote.usage);
  const [attributes, setAttributes] = useState<PricingAttributes>(
    initialQuote.attributes,
  );
  const [at, setAt] = useState(initialQuote.at);
  useEffect(() => {
    try {
      sessionStorage.setItem(
        "veyra-metered-simulator-v1",
        JSON.stringify({
          modelId: selected,
          keyId: selectedKey,
          usage,
          at,
          attributes,
        }),
      );
    } catch {
      /* In-memory values remain usable. */
    }
  }, [selected, selectedKey, usage, at, attributes]);
  const [result, setResult] = useState<{
    trace: string;
    message: string;
    success: boolean;
  } | null>(null);
  const previousPricing = useRef(config);
  useEffect(() => {
    // A scenario reset replaces the sample config even when its revision stays
    // "sample-1". Mount/refresh and ordinary publications must retain the form.
    const scenarioReset =
      previousPricing.current !== config && config.updatedAt === null;
    previousPricing.current = config;
    if (!scenarioReset) return;
    setSelected(modelId ?? catalogModels[0].id);
    setSelectedKey("");
    setUsage({ ...sampleUsage });
    setAttributes({});
    setAt(localDateTime());
    setResult(null);
  }, [config]);
  const model =
    catalogModels.find((item) => item.id === selected) ?? catalogModels[0];
  const entry = draft.models[selected];
  const sourceError =
    entry.source === "manual-official" &&
    (!entry.sourceUrl || !entry.verifiedAt)
      ? text(
          ...translatedMessage(
            "an_official_reference_needs_a_source_url_and__dc09676778",
          ),
        )
      : [entry.verifiedAt, entry.effectiveAt].some(
            (date) => date && Date.parse(date) > Date.now(),
          )
        ? text(
            ...translatedMessage(
              "verification_and_reference_dates_cannot_be_in_4021289355",
            ),
          )
        : "";
  const valid =
    structurallyValid &&
    Object.values(draft.models).every(
      (item) =>
        (item.source !== "manual-official" ||
          Boolean(item.sourceUrl && item.verifiedAt)) &&
        ![item.verifiedAt, item.effectiveAt].some(
          (date) => date && Date.parse(date) > Date.now(),
        ),
    );
  const rule = entry.rule;
  const billableUsage = useMemo(() => {
    if (rule.mode === "tokens")
      return {
        input_tokens: usage.input_tokens,
        output_tokens: usage.output_tokens,
        cache_read_tokens: usage.cache_read_tokens,
        cache_write_tokens: usage.cache_write_tokens,
        requests: 1,
      };
    if (rule.mode === "unit")
      return rule.unit === "image"
        ? { images: usage.images, requests: 1 }
        : rule.unit === "second"
          ? { seconds: usage.seconds, requests: 1 }
          : { requests: usage.requests };
    return { ...usage };
  }, [rule, usage]);
  const setEntry = (patch: Partial<typeof entry>) => {
    setResult(null);
    setDraft((current) => ({
      ...current,
      models: {
        ...current.models,
        [selected]: { ...current.models[selected], ...patch },
      },
    }));
  };
  const setRule = (next: PricingRule) => setEntry({ rule: next });
  const labels: Record<keyof ModelPricing, string> = {
    input_per_million_usd: text(
      ...translatedMessage("uncached_input_a3745282fd"),
    ),
    output_per_million_usd: text(...translatedMessage("output_98e2ff69c4")),
    cache_read_per_million_usd: text(
      ...translatedMessage("cache_read_optional_0abf93cac3"),
    ),
    cache_write_per_million_usd: text(
      ...translatedMessage("cache_write_optional_db6ed0d992"),
    ),
  };
  const eligibleKeys = keys.filter(
    (key) =>
      key.active &&
      getKeyModels(key, draft).some((item) => item.id === selected),
  );
  const key =
    eligibleKeys.find((item) => item.id === selectedKey) ?? eligibleKeys[0];
  const quoteAt = Number.isFinite(new Date(at).getTime())
    ? new Date(at).toISOString()
    : "";
  const quote = useMemo(() => {
    if (!quoteAt)
      return {
        amount: "—",
        error: text(
          ...translatedMessage(
            "choose_an_eligible_key_and_a_valid_time_to_qu_a7cb71b184",
          ),
        ),
      };
    try {
      const quoted = key
        ? quoteModelCharge(
            {
              modelId: selected,
              tierId: key.tier,
              usage: billableUsage,
              attributes,
              at: quoteAt,
            },
            draft,
          )
        : (() => {
            const result = quotePricingRule(
              rule,
              entry.pricing,
              billableUsage,
              "1",
              quoteAt,
              attributes,
            );
            return { amountNano: result.amountNano, snapshot: result };
          })();
      return {
        amount: formatMoney(quoted.amountNano, "call"),
        error: "",
        quoted,
      };
    } catch (error) {
      return {
        amount: "—",
        error:
          error instanceof Error
            ? error.message
            : text(
                ...translatedMessage(
                  "unable_to_quote_check_the_rule_and_usage_c3c7a6483b",
                ),
              ),
      };
    }
  }, [draft, key, selected, billableUsage, quoteAt, lang, attributes]);
  const changedModels = catalogModels.filter((item) => {
    const withoutAvailability = (snapshot: PricingCatalogConfig) => {
      const { tierAvailability: _ignored, ...rest } = snapshot.models[item.id];
      return rest;
    };
    return (
      JSON.stringify(withoutAvailability(draft)) !==
      JSON.stringify(withoutAvailability(base))
    );
  });
  const billingChangedModels = changedModels.filter((item) => {
    const billing = (snapshot: PricingCatalogConfig) => ({
      pricing: snapshot.models[item.id].pricing,
      rule: snapshot.models[item.id].rule,
    });
    return JSON.stringify(billing(base)) !== JSON.stringify(billing(draft));
  });
  const affectedKeys = keys.filter(
    (key) =>
      key.active &&
      billingChangedModels.some((item) =>
        getKeyModels(key, draft).some((candidate) => candidate.id === item.id),
      ),
  );
  const impact = billingChangedModels.flatMap((item) =>
    affectedKeys
      .filter((key) =>
        getKeyModels(key, draft).some((candidate) => candidate.id === item.id),
      )
      .map((key) => {
        const exampleAttributes = examplePricingAttributes(
          draft.models[item.id].rule,
        );
        const amount = (snapshot: PricingCatalogConfig) => {
          try {
            return formatMoney(
              quoteModelCharge(
                {
                  modelId: item.id,
                  tierId: key.tier,
                  usage: sampleUsage,
                  attributes: exampleAttributes,
                  at: quoteAt || new Date().toISOString(),
                },
                snapshot,
              ).amountNano,
              "call",
            );
          } catch {
            return text(...translatedMessage("not_quotable_3d02114fd7"));
          }
        };
        return {
          model: item.name,
          key: key.name,
          attributes: exampleAttributes,
          before: amount(base),
          after: amount(draft),
        };
      }),
  );
  const ruleError = pricingRuleError(rule);
  const windows = rule.timeWindows ?? [];
  const updateWindow = (id: string, patch: Partial<PricingTimeWindow>) =>
    setRule({
      ...rule,
      timeWindows: windows.map((window) =>
        window.id === id ? { ...window, ...patch } : window,
      ),
    });
  const usageLabels: Record<keyof Usage, string> = {
    input_tokens: text(...translatedMessage("input_tokens_2470592d0d")),
    output_tokens: text(...translatedMessage("output_tokens_e8729227cd")),
    cache_read_tokens: text(
      ...translatedMessage("cache_read_tokens_7e8333794a"),
    ),
    cache_write_tokens: text(
      ...translatedMessage("cache_write_tokens_c1d36f2888"),
    ),
    images: text(...translatedMessage("images_0cc4309f02")),
    seconds: text(...translatedMessage("duration_seconds_7aa2eff257")),
    requests: text(...translatedMessage("requests_de8ae6d124")),
  };
  const usageFields: (keyof Usage)[] =
    rule.mode === "tokens"
      ? [
          "input_tokens",
          "output_tokens",
          "cache_read_tokens",
          "cache_write_tokens",
        ]
      : rule.mode === "unit"
        ? [
            (
              {
                image: "images",
                second: "seconds",
                request: "requests",
              } as const
            )[rule.unit],
          ]
        : (Object.keys(usageLabels) as (keyof Usage)[]);
  const submitUsage = () => {
    if (!canEdit || !key || dirty || !quoteAt) return;
    const record = recordMeteredCall(
      key.id,
      selected,
      billableUsage,
      quoteAt,
      attributes,
    );
    setResult({
      trace: record.trace_id,
      success: record.status === "success",
      message:
        record.status === "success"
          ? text(
              ...translatedMessage(
                "simulated_usage_submitted_charged_no_real_mod_1b6e631d2b",
                { p0: formatMoney(record.cost_nano, "call") },
              ),
            )
          : `${record.error_message ?? text(...translatedMessage("simulated_call_failed_91b73ad796"))} ${record.recovery_action ?? ""} ${text(...translatedMessage("not_charged_your_inputs_are_retained_7d7f89dffd"))}`,
    });
  };
  return (
    <div className="pricing-admin settings-pricing">
      <div className="pricing-page-heading" hidden={embedded}>
        <div>
          <span className="pricing-eyebrow">
            <CircleDollarSign size={15} />
            {text(...translatedMessage("system_settings_966a9e1755"))}
          </span>
          <h1>{text(...translatedMessage("model_pricing_4a65297d67"))}</h1>
          <p>
            {text(
              ...translatedMessage(
                "manage_this_browser_s_demo_pricing_and_time_r_751c26a7cd",
              ),
            )}
          </p>
        </div>
        <Button asChild variant="secondary">
          <Link to="/console/service-groups">
            {text(...translatedMessage("manage_service_groups_2b5ad01162"))}
            <ArrowUpRight size={15} />
          </Link>
        </Button>
      </div>
      <div className="pricing-policy">
        <Info size={18} />
        <div>
          <strong>
            {text(
              ...translatedMessage(
                "reference_sources_and_immutable_historical_ra_2f0b07f1da",
              ),
            )}
          </strong>
          <p>
            {text(
              ...translatedMessage(
                "rules_here_are_stored_in_this_browser_for_dem_a22a527a5b",
              ),
            )}
          </p>
        </div>
      </div>
      {notice && (
        <div className="pricing-notice" role="status">
          {notice}
        </div>
      )}
      {conflict && dirty && (
        <div className="pricing-notice settings-error" role="alert">
          {text(
            ...translatedMessage(
              "a_newer_configuration_exists_discard_this_ret_14465c3a98",
            ),
          )}
        </div>
      )}
      <section className="pricing-panel">
        <div
          hidden={embedded}
          className="pricing-model-tabs"
          aria-label={text(
            ...translatedMessage("choose_billing_model_32050d8ded"),
          )}
        >
          {catalogModels.map((item) => (
            <button
              type="button"
              key={item.id}
              className={selected === item.id ? "active" : ""}
              aria-pressed={selected === item.id}
              onClick={() => {
                setSelected(item.id);
                setResult(null);
              }}
            >
              {item.name}
            </button>
          ))}
        </div>
        <div className="pricing-model-title">
          <div>
            <h3>{model.name}</h3>
            <code>{model.id}</code>
          </div>
          <span>
            {text(...translatedMessage("demo_reference_price_4724362246"))} ·{" "}
            {modeName(rule, text)}
          </span>
        </div>
        <div className="settings-fields settings-source-fields">
          <label>
            <span>
              {text(...translatedMessage("reference_source_e502ebb6d4"))}
            </span>
            <select
              aria-label={text(
                ...translatedMessage("reference_source_e502ebb6d4"),
              )}
              value={entry.source}
              disabled={!canEdit}
              onChange={(event) =>
                setEntry({ source: event.target.value as typeof entry.source })
              }
            >
              <option value="sample">
                {text(
                  ...translatedMessage("demo_sample_unverified_e93da2d57c"),
                )}
              </option>
              <option value="manual-official">
                {text(
                  ...translatedMessage(
                    "base_informed_by_official_rates_manual_9768efeebb",
                  ),
                )}
              </option>
            </select>
          </label>
          <label>
            <span>
              {text(...translatedMessage("official_source_url_d26ccde037"))}
            </span>
            <input
              type="url"
              placeholder="https://"
              value={entry.sourceUrl}
              disabled={!canEdit}
              onChange={(event) => setEntry({ sourceUrl: event.target.value })}
            />
          </label>
          <label>
            <span>
              {text(...translatedMessage("verification_date_e695c40f81"))}
            </span>
            <input
              type="date"
              max={new Date().toISOString().slice(0, 10)}
              value={entry.verifiedAt?.slice(0, 10) ?? ""}
              disabled={!canEdit}
              onChange={(event) =>
                setEntry({ verifiedAt: event.target.value || null })
              }
            />
          </label>
          <label>
            <span>
              {text(
                ...translatedMessage("reference_effective_date_3d446faf1a"),
              )}
            </span>
            <input
              type="date"
              max={new Date().toISOString().slice(0, 10)}
              value={entry.effectiveAt?.slice(0, 10) ?? ""}
              disabled={!canEdit}
              onChange={(event) =>
                setEntry({ effectiveAt: event.target.value || null })
              }
            />
          </label>
        </div>
        <p className="pricing-help settings-version">
          {text(
            ...translatedMessage(
              "pricing_version_generated_on_publish_9478aa7e57",
            ),
          )}
          <code>{entry.version}</code>
        </p>
        {sourceError && (
          <p className="settings-error" role="alert">
            {sourceError}
          </p>
        )}
        <p className="pricing-help">
          {text(
            ...translatedMessage(
              "the_effective_date_documents_the_reference_pu_d878934b48",
            ),
          )}
        </p>
        <div className="settings-editor-grid">
          <div>
            <div className="settings-block-heading">
              <h3>{text(...translatedMessage("billing_method_cfacfdecf2"))}</h3>
              <span>{text(...translatedMessage("base_rule_768c2b7470"))}</span>
            </div>
            <div
              className="settings-segment"
              aria-label={text(
                ...translatedMessage("billing_method_cfacfdecf2"),
              )}
            >
              {(["tokens", "unit", "expression"] as const).map((mode) => (
                <button
                  key={mode}
                  type="button"
                  aria-pressed={rule.mode === mode}
                  disabled={!canEdit}
                  className={rule.mode === mode ? "active" : ""}
                  onClick={() => {
                    if (mode === rule.mode) return;
                    setRule(
                      mode === "tokens"
                        ? { mode, timeWindows: windows }
                        : mode === "unit"
                          ? {
                              mode,
                              unit: model.capabilities.includes("image")
                                ? "image"
                                : model.capabilities.includes("video")
                                  ? "second"
                                  : "request",
                              priceUsd: "0.01",
                              minimum: "0",
                              ceilTo: "1",
                              timeWindows: windows,
                            }
                          : {
                              mode,
                              expression:
                                "input_tokens * 0.000002 + output_tokens * 0.000008",
                              timeWindows: windows,
                            },
                    );
                  }}
                >
                  {mode === "tokens"
                    ? text(...translatedMessage("tokens_8ded6f9478"))
                    : mode === "unit"
                      ? text(...translatedMessage("units_11b53c4b95"))
                      : text(
                          ...translatedMessage(
                            "expression_advanced_4f8914ede0",
                          ),
                        )}
                </button>
              ))}
            </div>
            {rule.mode === "tokens" && (
              <>
                <div className="pricing-base-fields settings-token-fields">
                  {(Object.keys(labels) as (keyof ModelPricing)[]).map(
                    (field, index) => (
                      <label key={field}>
                        <span>{labels[field]}</span>
                        <div className="pricing-unit-input">
                          <span>$</span>
                          <input
                            aria-label={`${model.name} ${labels[field]}`}
                            inputMode="decimal"
                            disabled={!canEdit}
                            value={entry.pricing[field] ?? ""}
                            placeholder={
                              index > 1
                                ? text(
                                    ...translatedMessage(
                                      "not_configured_e0aac6ee6f",
                                    ),
                                  )
                                : "0.00"
                            }
                            aria-invalid={
                              !(
                                index > 1 && entry.pricing[field] === undefined
                              ) && !isValidPrice(entry.pricing[field])
                            }
                            onChange={(event) =>
                              setEntry({
                                pricing: {
                                  ...entry.pricing,
                                  [field]:
                                    index > 1 && !event.target.value
                                      ? undefined
                                      : event.target.value,
                                },
                              })
                            }
                          />
                        </div>
                      </label>
                    ),
                  )}
                </div>
                <p className="pricing-help">
                  {text(
                    ...translatedMessage(
                      "unit_usd_million_tokens_buckets_bill_separate_39d5b7f6ac",
                    ),
                  )}
                </p>
              </>
            )}
            {rule.mode === "unit" && (
              <>
                <div className="settings-fields">
                  <label>
                    <span>
                      {text(...translatedMessage("metering_unit_a0d15bd3d2"))}
                    </span>
                    <select
                      aria-label={text(
                        ...translatedMessage("metering_unit_a0d15bd3d2"),
                      )}
                      value={rule.unit}
                      disabled={!canEdit}
                      onChange={(event) =>
                        setRule({
                          ...rule,
                          unit: event.target.value as
                            "request" | "image" | "second",
                        })
                      }
                    >
                      <option value="request">
                        {text(...translatedMessage("request_81434bae0d"))}
                      </option>
                      <option value="image">
                        {text(...translatedMessage("image_e45355d245"))}
                      </option>
                      <option value="second">
                        {text(...translatedMessage("second_2554d8a6ff"))}
                      </option>
                    </select>
                  </label>
                  {(["priceUsd", "minimum", "ceilTo"] as const).map((field) => (
                    <label key={field}>
                      <span>
                        {field === "priceUsd"
                          ? text(
                              ...translatedMessage(
                                "price_per_unit_usd_531d9e583a",
                              ),
                            )
                          : field === "minimum"
                            ? text(
                                ...translatedMessage(
                                  "minimum_billed_quantity_3697a776cc",
                                ),
                              )
                            : text(
                                ...translatedMessage(
                                  "rounding_increment_48dac58947",
                                ),
                              )}
                      </span>
                      <input
                        inputMode="decimal"
                        value={rule[field]}
                        disabled={!canEdit}
                        onChange={(event) =>
                          setRule({ ...rule, [field]: event.target.value })
                        }
                      />
                    </label>
                  ))}
                </div>
                <p className="pricing-help">
                  {text(
                    ...translatedMessage(
                      "bills_the_selected_unit_not_tokens_apply_the__7056463681",
                    ),
                  )}
                </p>
              </>
            )}
            {rule.mode === "unit" && (
              <PricingConditions
                rule={rule}
                onChange={setRule}
                disabled={!canEdit}
              />
            )}
            {rule.mode === "expression" && (
              <div className="settings-expression">
                <label>
                  <span>
                    {text(
                      ...translatedMessage(
                        "billing_expression_result_usd_bcbd9e5318",
                      ),
                    )}
                  </span>
                  <textarea
                    aria-label={text(
                      ...translatedMessage(
                        "billing_expression_result_usd_bcbd9e5318",
                      ),
                    )}
                    dir="ltr"
                    rows={3}
                    value={rule.expression}
                    disabled={!canEdit}
                    onChange={(event) =>
                      setRule({ ...rule, expression: event.target.value })
                    }
                    spellCheck={false}
                  />
                </label>
                <p className="pricing-help">
                  {text(
                    ...translatedMessage(
                      "supports_arithmetic_parentheses_min_max_and_c_b462fdc941",
                    ),
                  )}
                </p>
                <details className="settings-details">
                  <summary>
                    {text(
                      ...translatedMessage("variables_and_units_1e247ecede"),
                    )}
                    <ChevronDown size={16} />
                  </summary>
                  <div className="settings-variable-list">
                    {Object.entries(usageLabels).map(([name, label]) => (
                      <div key={name}>
                        <code>{name}</code>
                        <span>{label}</span>
                      </div>
                    ))}
                  </div>
                  <p className="pricing-help">
                    {text(
                      ...translatedMessage(
                        "the_expression_returns_the_base_charge_time_a_b453cbcfb7",
                      ),
                    )}
                  </p>
                </details>
              </div>
            )}
            {ruleError && (
              <p className="settings-error" role="alert">
                {ruleError}
              </p>
            )}
          </div>
          <aside className="settings-quote">
            <div className="settings-block-heading">
              <h3>
                <FlaskConical size={16} />
                {text(...translatedMessage("quote_simulator_896d222d19"))}
              </h3>
              <span>
                {dirty
                  ? text(...translatedMessage("using_draft_2348a91d1a"))
                  : text(...translatedMessage("published_rules_2e66826ec2"))}
              </span>
            </div>
            <label>
              <span>
                {text(...translatedMessage("key_for_quotation_87eb68843d"))}
              </span>
              <select
                aria-label={text(
                  ...translatedMessage("key_for_quotation_87eb68843d"),
                )}
                value={key?.id ?? ""}
                onChange={(event) => {
                  setSelectedKey(event.target.value);
                  setResult(null);
                }}
              >
                <option value="" disabled>
                  {text(
                    ...translatedMessage("choose_an_eligible_key_7ade59c1c2"),
                  )}
                </option>
                {eligibleKeys.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name} · {serviceGroupLabel(item.tier, lang, draft)}
                  </option>
                ))}
              </select>
            </label>
            {!key && (
              <p className="pricing-help">
                {text(
                  ...translatedMessage(
                    "no_enabled_key_is_authorized_for_this_model_m_ee7da4c8e0",
                  ),
                )}{" "}
                <Link to="/console/keys">
                  {text(...translatedMessage("manage_keys_2620f30141"))}
                </Link>
              </p>
            )}
            <PricingConditionInputs
              rule={rule}
              attributes={attributes}
              onChange={(value) => {
                setAttributes(value);
                setResult(null);
              }}
            />
            <div className="settings-fields settings-usage-fields">
              {usageFields.map((field) => (
                <label key={field}>
                  <span>{usageLabels[field]}</span>
                  <input
                    type="number"
                    min="0"
                    step={field === "seconds" ? "0.1" : "1"}
                    value={usage[field]}
                    onChange={(event) => {
                      setUsage({
                        ...usage,
                        [field]: Number(event.target.value),
                      });
                      setResult(null);
                    }}
                  />
                </label>
              ))}
            </div>
            <label>
              <span>
                {text(
                  ...translatedMessage(
                    "pricing_simulation_time_local_timezone_abd01ea568",
                  ),
                )}
              </span>
              <input
                type="datetime-local"
                value={at}
                onChange={(event) => {
                  setAt(event.target.value);
                  setResult(null);
                }}
              />
            </label>
            <div className="settings-quote-total">
              <span>
                {text(
                  ...(key
                    ? translatedMessage("ui_estimated_demo_charge_de2d6fcf")
                    : translatedMessage(
                        "ui_model_base_charge_before_group_multiplier__fc478e11",
                      )),
                )}
              </span>
              <strong>{quote.amount}</strong>
            </div>
            {quote.quoted && (
              <small>
                {quote.quoted.snapshot.matchedWindow
                  ? text(
                      ...translatedMessage("time_window_5f0e372ee8", {
                        p0: quote.quoted.snapshot.matchedWindow.label,
                        p1: quote.quoted.snapshot.timeMultiplier,
                      }),
                    )
                  : text(
                      ...translatedMessage(
                        "no_time_window_matched_base_rules_apply_379920c018",
                      ),
                    )}
              </small>
            )}
            {quote.error && (
              <p role="alert" className="settings-error">
                {quote.error}
              </p>
            )}
            <small>
              {text(
                ...translatedMessage(
                  "demo_balance_quoting_does_not_charge_submitte_8c556a40e3",
                  { p0: formatMoney(balance, "balance") },
                ),
              )}
            </small>
            <Button
              variant="primary"
              disabled={
                !canEdit ||
                !key ||
                dirty ||
                conflict ||
                !valid ||
                Boolean(quote.error)
              }
              onClick={submitUsage}
            >
              {text(
                ...translatedMessage("submit_simulated_usage_bill_9644ff1933"),
              )}
            </Button>
            {dirty && (
              <small>
                {text(
                  ...translatedMessage(
                    "publish_first_to_submit_usage_at_the_publishe_3b19d1b25c",
                  ),
                )}
              </small>
            )}
            {result && (
              <div
                className={`settings-result ${result.success ? "" : "settings-error"}`}
                role="status"
              >
                <p>{result.message}</p>
                <Link
                  to={`/console/requests?trace=${encodeURIComponent(result.trace)}`}
                >
                  {text(...translatedMessage("view_linked_request_c351c5b092"))}
                </Link>
              </div>
            )}
            <small>
              {text(
                ...translatedMessage(
                  "this_bills_manually_entered_simulated_usage_a_c6843ac01d",
                ),
              )}
            </small>
          </aside>
        </div>
        <div className="settings-time-section">
          <div className="settings-block-heading">
            <h3>
              <Clock3 size={16} />
              {text(...translatedMessage("time_based_pricing_f0c570de16"))}
            </h3>
            <Button
              size="small"
              disabled={!canEdit}
              onClick={() =>
                setRule({
                  ...rule,
                  timeWindows: [
                    ...windows,
                    {
                      id: crypto.randomUUID(),
                      label: text(
                        ...translatedMessage("off_peak_window_f09dbb30f4"),
                      ),
                      timezone: "Asia/Shanghai",
                      days: [0, 1, 2, 3, 4, 5, 6],
                      start: "00:00",
                      end: "08:00",
                      multiplier: "0.5",
                    },
                  ],
                })
              }
            >
              <Plus size={15} />
              {text(...translatedMessage("add_time_window_95291dd8b0"))}
            </Button>
          </div>
          <p className="pricing-help">
            {text(
              ...translatedMessage(
                "time_rules_apply_to_any_billing_mode_outside__1be868205a",
              ),
            )}
          </p>
          {windows.length === 0 && (
            <div className="settings-empty">
              {text(
                ...translatedMessage(
                  "no_time_rules_same_base_price_all_day_e1c50d3289",
                ),
              )}
            </div>
          )}
          {windows.map((window, index) => (
            <div key={window.id} className="settings-window">
              <div className="settings-fields settings-window-fields">
                <label>
                  <span>
                    {text(...translatedMessage("window_name_54b8c619aa"))}
                  </span>
                  <input
                    aria-label={`${text(...translatedMessage("window_name_54b8c619aa"))} ${index + 1}`}
                    value={window.label}
                    disabled={!canEdit}
                    onChange={(event) =>
                      updateWindow(window.id, { label: event.target.value })
                    }
                  />
                </label>
                <label>
                  <span>
                    {text(...translatedMessage("timezone_589797f8d4"))}
                  </span>
                  <select
                    aria-label={text(
                      ...translatedMessage("timezone_589797f8d4"),
                    )}
                    value={window.timezone}
                    disabled={!canEdit}
                    onChange={(event) =>
                      updateWindow(window.id, { timezone: event.target.value })
                    }
                  >
                    {[
                      "Asia/Shanghai",
                      "Asia/Dubai",
                      "UTC",
                      "America/Los_Angeles",
                      "Europe/London",
                    ].map((zone) => (
                      <option key={zone} value={zone}>
                        {zone}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  <span>
                    {text(...translatedMessage("start_time_74b416f5bd"))}
                  </span>
                  <input
                    type="time"
                    value={window.start}
                    disabled={!canEdit}
                    onChange={(event) =>
                      updateWindow(window.id, { start: event.target.value })
                    }
                  />
                </label>
                <label>
                  <span>
                    {text(...translatedMessage("end_time_72a4d845fc"))}
                  </span>
                  <input
                    type="time"
                    value={window.end}
                    disabled={!canEdit}
                    onChange={(event) =>
                      updateWindow(window.id, { end: event.target.value })
                    }
                  />
                </label>
                <label>
                  <span>
                    {text(...translatedMessage("time_coefficient_c698caf1bc"))}
                  </span>
                  <input
                    inputMode="decimal"
                    value={window.multiplier}
                    disabled={!canEdit || Boolean(window.tokenPrices)}
                    onChange={(event) =>
                      updateWindow(window.id, {
                        multiplier: event.target.value,
                      })
                    }
                  />
                </label>
              </div>
              {rule.mode === "tokens" && (
                <div className="cm-callout">
                  <label className="cm-check">
                    <input
                      type="checkbox"
                      checked={Boolean(window.tokenPrices)}
                      disabled={!canEdit}
                      onChange={(e) =>
                        updateWindow(window.id, {
                          tokenPrices: e.target.checked
                            ? {
                                input_per_million_usd:
                                  entry.pricing.input_per_million_usd,
                              }
                            : undefined,
                          multiplier: "1",
                        })
                      }
                    />
                    {text(
                      ...translatedMessage(
                        "ui_set_separate_token_prices_for_this_time_windo_2a0e869c",
                      ),
                    )}
                  </label>
                  <p className="pricing-help">
                    {text(
                      ...translatedMessage(
                        "ui_use_for_peak_and_off_peak_input_output_or_cac_9c65f3cb",
                      ),
                    )}
                  </p>
                  {window.tokenPrices && (
                    <div className="settings-fields">
                      {(Object.keys(labels) as (keyof ModelPricing)[]).map(
                        (field) => (
                          <label key={field}>
                            {labels[field]}
                            {text(
                              ...translatedMessage(
                                "ui__usd_million_tokens__609e7acd",
                              ),
                            )}
                            <input
                              aria-label={`${window.label} ${labels[field]}`}
                              inputMode="decimal"
                              disabled={!canEdit}
                              value={window.tokenPrices![field] ?? ""}
                              placeholder={text(
                                ...translatedMessage(
                                  "ui_leave_blank_to_use_the_base_rate_26b64924",
                                ),
                              )}
                              onChange={(e) => {
                                const prices = { ...window.tokenPrices };
                                if (e.target.value === "") delete prices[field];
                                else prices[field] = e.target.value;
                                updateWindow(window.id, {
                                  tokenPrices: prices,
                                });
                              }}
                            />
                          </label>
                        ),
                      )}
                    </div>
                  )}
                </div>
              )}
              <div className="settings-window-footer">
                <div
                  className="settings-weekdays"
                  aria-label={text(
                    ...translatedMessage("applicable_days_5f47296f3c"),
                  )}
                >
                  {[1, 2, 3, 4, 5, 6, 0].map((day) => (
                    <button
                      key={day}
                      type="button"
                      aria-pressed={window.days.includes(day)}
                      disabled={!canEdit}
                      className={window.days.includes(day) ? "active" : ""}
                      onClick={() =>
                        updateWindow(window.id, {
                          days: window.days.includes(day)
                            ? window.days.filter((value) => value !== day)
                            : [...window.days, day],
                        })
                      }
                    >
                      {new Intl.DateTimeFormat(
                        lang === "ar" ? "ar" : lang === "zh" ? "zh-CN" : "en",
                        { weekday: "short", timeZone: "UTC" },
                      ).format(new Date(Date.UTC(2024, 0, 7 + day)))}
                    </button>
                  ))}
                </div>
                <Button
                  size="small"
                  variant="ghost"
                  disabled={!canEdit}
                  onClick={() =>
                    setRule({
                      ...rule,
                      timeWindows: windows.filter(
                        (candidate) => candidate.id !== window.id,
                      ),
                    })
                  }
                >
                  <Trash2 size={14} />
                  {text(...translatedMessage("remove_window_12fc906094"))}
                </Button>
              </div>
            </div>
          ))}
        </div>
        <OfficialReferenceEditor
          modelId={selected}
          config={draft}
          disabled={!canEdit}
          onChange={(officialReference) => setEntry({ officialReference })}
        />
      </section>
      <div className="pricing-save-bar" data-dirty={dirty}>
        <div>
          <strong>
            {dirty
              ? text(
                  ...translatedMessage(
                    "models_have_unpublished_changes_15b71f1fb5",
                    { p0: changedModels.length },
                  ),
                )
              : text(
                  ...translatedMessage("browser_demo_pricing_saved_6b5d9bd299"),
                )}
          </strong>
          <small>
            {text(
              ...translatedMessage(
                "drafts_survive_switching_settings_modules_rev_1f0a2f2c1e",
              ),
            )}
          </small>
          {!valid && (
            <small className="settings-error">
              {text(
                ...translatedMessage(
                  "correct_invalid_rules_or_reference_metadata_b_ea08c09882",
                ),
              )}
            </small>
          )}
        </div>
        <div>
          <Button disabled={!dirty} onClick={discard}>
            {text(...translatedMessage("discard_changes_042b3275d7"))}
          </Button>
          <Button
            variant="primary"
            disabled={!canEdit || !dirty || !valid || conflict}
            onClick={() => setReviewOpen(true)}
          >
            {text(...translatedMessage("review_publish_cab25aec1d"))}
          </Button>
        </div>
      </div>

      {/* Option config status & history */}
      {canEdit && (
        <div
          className="pricing-table-scroll"
          style={{ marginTop: "var(--space-6)" }}
        >
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              marginBottom: "var(--space-3)",
            }}
          >
            <div>
              <strong>
                {text(
                  ...translatedMessage(
                    "server_config_version_separate_from_demo_8bb72912c1",
                  ),
                )}
              </strong>
              {optionStatusQuery.data && (
                <span
                  style={{ marginInlineStart: 8, fontSize: "var(--text-sm)" }}
                >
                  v{optionStatusQuery.data.database_version}
                  {optionStatusQuery.data.in_sync
                    ? text(
                        ...translatedMessage(
                          "loaded_by_response_process_7063fd13c7",
                        ),
                      )
                    : text(
                        ...translatedMessage(
                          "response_process_behind_1ffc8bd8ab",
                        ),
                      )}
                </span>
              )}
            </div>
            <Button
              variant="secondary"
              onClick={() => {
                setRollbackNotice("");
                setHistoryOpen(true);
              }}
            >
              <Clock3 size={15} style={{ marginInlineEnd: 4 }} />
              {text(...translatedMessage("server_history_rollback_c1f73e6cfe"))}
            </Button>
          </div>
          {rollbackNotice && (
            <p
              role="alert"
              style={{ fontSize: "var(--text-sm)", color: "var(--success)" }}
            >
              {rollbackNotice}
            </p>
          )}
        </div>
      )}

      <Modal
        open={reviewOpen}
        onOpenChange={setReviewOpen}
        title={text(
          ...translatedMessage("save_browser_demo_pricing_d38e01e044"),
        )}
        description={text(
          ...translatedMessage(
            "affects_future_simulations_in_this_browser_se_01a390bdca",
          ),
        )}
        size="wide"
        preventOutsideClose
        footer={
          <>
            <Button onClick={() => setReviewOpen(false)}>
              {text(...translatedMessage("continue_editing_779481a437"))}
            </Button>
            <Button
              variant="primary"
              disabled={!canEdit || !valid || conflict}
              onClick={publish}
            >
              <Check size={16} />
              {text(...translatedMessage("publish_now_52d9a772d6"))}
            </Button>
          </>
        }
      >
        <div className="settings-review-summary">
          <strong>
            {text(
              ...translatedMessage("models_active_keys_affected_549427e7a7", {
                p0: changedModels.length,
                p1: affectedKeys.length,
              }),
            )}
          </strong>
          <p>
            {changedModels
              .map(
                (item) =>
                  `${item.name}（${modeName(base.models[item.id].rule, text)} → ${modeName(draft.models[item.id].rule, text)}）`,
              )
              .join("、")}
          </p>
          <p>
            {text(
              ...translatedMessage(
                "models_have_billing_changes_official_referenc_9906cc7dc7",
                { p0: billingChangedModels.length },
              ),
            )}
          </p>
        </div>
        <p className="pricing-help">
          {text(
            ...translatedMessage(
              "example_usage_1_000_input_500_output_tokens_n_141e20030c",
            ),
          )}
        </p>
        <div className="pricing-table-scroll">
          <table>
            <thead>
              <tr>
                <th>{text(...translatedMessage("model_9c2e5b5065"))}</th>
                <th>{text(...translatedMessage("affected_key_8b626ab6e6"))}</th>
                <th>
                  {text(
                    ...translatedMessage(
                      "example_charge_current_published_3126739753",
                    ),
                  )}
                </th>
              </tr>
            </thead>
            <tbody>
              {impact.map((row) => (
                <tr key={`${row.model}-${row.key}`}>
                  <th>
                    {row.model}
                    {row.attributes && (
                      <small
                        className="pricing-help"
                        style={{ display: "block" }}
                      >
                        {text(
                          ...translatedMessage(
                            "ui_sample_specification__5bf9ae0f",
                          ),
                        )}
                        {JSON.stringify(row.attributes)}
                      </small>
                    )}
                  </th>
                  <td>{row.key}</td>
                  <td dir="ltr">
                    {row.before} → {row.after}
                  </td>
                </tr>
              ))}
              {impact.length === 0 && (
                <tr>
                  <td colSpan={3}>
                    {text(
                      ...translatedMessage(
                        "no_active_keys_affected_future_authorized_req_d748fab9e2",
                      ),
                    )}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        {notice && <p role="alert">{notice}</p>}
      </Modal>

      {/* Option history & rollback modal */}
      <Modal
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        title={text(
          ...translatedMessage("server_config_change_history_95c88532a9"),
        )}
        description={text(
          ...translatedMessage(
            "these_actions_change_real_server_configuratio_fd6fc2d527",
          ),
        )}
        size="wide"
      >
        {optionHistoryQuery.isLoading ? (
          <p style={{ color: "var(--muted)" }}>
            {text(...translatedMessage("loading_4c20385375"))}
          </p>
        ) : (
          <table className="pricing-table-scroll">
            <thead>
              <tr>
                <th>{text(...translatedMessage("version_d07f8f123e"))}</th>
                <th>{text(...translatedMessage("key_ed5ae126a5"))}</th>
                <th>{text(...translatedMessage("old_value_bce1550409"))}</th>
                <th>{text(...translatedMessage("new_value_815ed9e198"))}</th>
                <th>{text(...translatedMessage("time_d70a9315f6"))}</th>
                <th>{text(...translatedMessage("action_2a8e78b55a"))}</th>
              </tr>
            </thead>
            <tbody>
              {optionHistoryQuery.data?.map((change) => (
                <tr key={change.id}>
                  <td
                    className="mono"
                    style={{ padding: "8px 10px", fontSize: 12 }}
                  >
                    v{change.version}
                  </td>
                  <td style={{ padding: "8px 10px" }}>{change.key}</td>
                  <td
                    className="mono"
                    style={{
                      padding: "8px 10px",
                      fontSize: 11,
                      maxWidth: 200,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                    dir="ltr"
                  >
                    {change.old_value || "—"}
                  </td>
                  <td
                    className="mono"
                    style={{
                      padding: "8px 10px",
                      fontSize: 11,
                      maxWidth: 200,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                    dir="ltr"
                  >
                    {change.new_value || "—"}
                  </td>
                  <td
                    className="mono"
                    style={{ padding: "8px 10px", fontSize: 12 }}
                  >
                    {new Date(change.created_at * 1000).toLocaleString()}
                  </td>
                  <td style={{ padding: "8px 10px" }}>
                    <Button
                      size="small"
                      variant="secondary"
                      disabled={
                        rollbackMut.isPending ||
                        !optionStatusQuery.data ||
                        change.version ===
                          optionStatusQuery.data.database_version
                      }
                      onClick={() => {
                        const currentVersion =
                          optionStatusQuery.data?.database_version;
                        if (!currentVersion) return;
                        setRollbackNotice("");
                        rollbackMut.mutate(
                          {
                            version: change.version,
                            expected_version: currentVersion,
                          },
                          {
                            onSuccess: () =>
                              setRollbackNotice(
                                text(
                                  ...translatedMessage(
                                    "rolled_back_eae267c75c",
                                  ),
                                ),
                              ),
                            onError: (err) =>
                              setRollbackNotice(
                                err instanceof ApiError
                                  ? err.message
                                  : text(
                                      ...translatedMessage(
                                        "rollback_failed_975389f448",
                                      ),
                                    ),
                              ),
                          },
                        );
                      }}
                    >
                      {text(...translatedMessage("rollback_08f071d9f7"))}
                    </Button>
                  </td>
                </tr>
              ))}
              {optionHistoryQuery.data &&
                optionHistoryQuery.data.length === 0 && (
                  <tr>
                    <td
                      colSpan={6}
                      style={{
                        padding: 16,
                        textAlign: "center",
                        color: "var(--muted)",
                      }}
                    >
                      {text(...translatedMessage("no_changes_d1b732d4c2"))}
                    </td>
                  </tr>
                )}
            </tbody>
          </table>
        )}
      </Modal>
    </div>
  );
}
