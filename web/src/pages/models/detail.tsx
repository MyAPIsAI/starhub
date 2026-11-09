import { PublicModelContent } from "./public-detail";
import { message as translatedMessage } from "../../i18n/messages";
import { useState } from "react";
import { useParams, Link, useSearchParams } from "react-router-dom";
import {
  ArrowLeft,
  ArrowRight,
  KeyRound,
  ShieldCheck,
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
import { writtenLanguage } from "../../lib/localize";
import { useTokens } from "../../lib/api/tokens";
import { useSystemAccess } from "../../lib/system-access";
import { reusableModelToken } from "../../lib/token-model-access";
import { usePlazaModel, plazaToDirectoryModel } from "../../lib/api/plaza";
import { useServiceQuality } from "../../lib/api/service-quality";
import { ModelRequestTrend } from "../../components/model-request-trend";
import { ServiceLevelValue } from "../../components/service-level-value";
import { ApiError } from "../../lib/api/http";
import "../../components/model-pricing.css";
import "../../components/access-center.css";

export default function ModelDetail() {
  const { id } = useParams<{ id: string }>();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const session = useSystemAccess((s) => s.session);
  const tokenQuery = useTokens();
  const [selectedTier, setTier] = useState<string | null>(null);
  const [params] = useSearchParams();
  const from = params.get("from");
  // Only a relative marketplace path is a supported return destination.
  const returnTo =
    from && (from === "/models" || from.startsWith("/models?"))
      ? from
      : "/models";
  const plazaDetail = usePlazaModel(id, true);
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
  // Fetch service quality summary to show a per-model metric panel inline.
  // The backend returns per-model aggregates in by_model.
  const qualityQuery = useServiceQuality(168, id, false);
  const modelQuality =
    model && !qualityQuery.isError
      ? qualityQuery.data?.by_model.find((m) => m.model === model.id)
      : undefined;
  const measuredP95 =
    modelQuality &&
    Number.isFinite(modelQuality.p95_duration_ms) &&
    modelQuality.p95_duration_ms > 0
      ? modelQuality.p95_duration_ms
      : null;
  const noLatencyData = text(
    ...translatedMessage("no_latency_data_3d1e009af8"),
  );
  const isLoading = plazaDetail.isPending;
  const hasLoadError =
    plazaDetail.isError &&
    !(
      plazaDetail.error instanceof ApiError && plazaDetail.error.status === 404
    );
  const compatibleKey =
    model?.online && !tokenQuery.isError
      ? reusableModelToken(tokenQuery.data ?? [], session, model.id, quoteGroup)
      : undefined;
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
            {text(...translatedMessage("back_to_models_42606e4b5d"))}
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
                ? text(...translatedMessage("loading_model_11eece6964"))
                : hasLoadError
                  ? text(
                      ...translatedMessage("failed_to_load_model_db396ca311"),
                    )
                  : text(...translatedMessage("model_not_found_d96fc188df"))}
            </h1>
            <p>
              {isLoading
                ? text(
                    ...translatedMessage(
                      "fetching_model_details_please_wait_1207650b23",
                    ),
                  )
                : hasLoadError
                  ? text(
                      ...translatedMessage(
                        "model_details_are_unavailable_please_retry_5932e8f7e0",
                      ),
                    )
                  : text(
                      ...translatedMessage(
                        "this_model_id_was_not_found_return_to_the_cat_b423eeb1f4",
                      ),
                    )}
            </p>
            <code>{id}</code>
            {hasLoadError ? (
              <Button onClick={() => void plazaDetail.refetch()}>
                {text(...translatedMessage("retry_c16455021c"))}
              </Button>
            ) : !isLoading ? (
              <Button asChild>
                <Link to={returnTo}>
                  {text(
                    ...translatedMessage("view_model_directory_a2b8887bee"),
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
                    ? text(...translatedMessage("smart_routing_34b9b06792"))
                    : text(...translatedMessage("specific_model_020bc6e3c7"))}
                </span>
                <h1>
                  {model.name}
                  {lang === "zh" && model.kind === "smart"
                    ? "（智能路由）"
                    : ""}
                </h1>
                <ModelIdCopy id={model.id} />
                <p>{model.description[writtenLanguage(lang)]}</p>
              </div>
              <div className="access-actions">
                {canConnect ? (
                  <>
                    <Button asChild variant="primary">
                      <Link to={connectUrl}>
                        <ArrowRight size={15} />
                        {compatibleKey
                          ? text(
                              ...translatedMessage(
                                "reuse_key_connect_0934bebb4a",
                              ),
                            )
                          : text(
                              ...translatedMessage("connect_a_tool_20565c835b"),
                            )}
                      </Link>
                    </Button>
                    <Button asChild variant="secondary">
                      <Link
                        to={`/console/playground?model=${encodeURIComponent(model.id)}${compatibleKey ? `&key=${compatibleKey.id}` : ""}`}
                      >
                        <Terminal size={15} />
                        {text(
                          ...translatedMessage(
                            "optional_demo_validation_30a421fd28",
                          ),
                        )}
                      </Link>
                    </Button>
                  </>
                ) : (
                  <Button variant="secondary" disabled>
                    {!supportsClient
                      ? text(
                          ...translatedMessage(
                            "client_connection_unavailable_be1d46f21e",
                          ),
                        )
                      : text(
                          ...translatedMessage(
                            "unavailable_in_this_group_7646f7ac65",
                          ),
                        )}
                  </Button>
                )}
              </div>
            </section>
            {model.publicCatalog && plazaDetail.data ? (
              <PublicModelContent detail={plazaDetail.data} />
            ) : (
              <>
                {!model.online && (
                  <div className="access-notice access-error">
                    {text(
                      ...translatedMessage(
                        "no_enabled_channel_serves_this_model_in_the_c_4689e9bf70",
                      ),
                    )}
                  </div>
                )}
                {!supportsClient && (
                  <div className="access-notice">
                    {text(
                      ...translatedMessage(
                        "no_supported_client_protocol_is_available_in__3855cb203a",
                      ),
                    )}
                  </div>
                )}
                {supportsClient && (
                  <div className="access-notice">
                    <KeyRound size={18} />
                    <span>
                      {compatibleKey
                        ? text(
                            ...translatedMessage(
                              "reuse_changing_models_does_not_require_a_new__c93b5717b0",
                              { p0: compatibleKey.name },
                            ),
                          )
                        : text(
                            ...translatedMessage(
                              "new_keys_select_a_service_group_and_budget_th_e2b2c99e8c",
                            ),
                          )}
                    </span>
                  </div>
                )}
                <section className="access-facts">
                  <div>
                    <small>
                      {text(...translatedMessage("protocols_3a5fb4e26a"))}
                    </small>
                    <strong>
                      {supportsClient
                        ? model.protocols.join(" / ")
                        : text(
                            ...translatedMessage(
                              "unavailable_in_this_group_ed5c15898a",
                            ),
                          )}
                    </strong>
                  </div>
                  <div>
                    <small>
                      {text(...translatedMessage("context_9810531841"))}
                    </small>
                    <strong>{model.contextDisplay}</strong>
                  </div>
                  <div>
                    <small>
                      {text(
                        ...translatedMessage("request_behavior_2ca6c11487"),
                      )}
                    </small>
                    <strong>
                      {model.kind === "smart"
                        ? text(
                            ...translatedMessage(
                              "authorized_candidate_selection_e2cc163eb8",
                            ),
                          )
                        : text(
                            ...translatedMessage(
                              "exact_requested_model_445f685eed",
                            ),
                          )}
                    </strong>
                  </div>
                  <div>
                    <small>
                      {text(...translatedMessage("service_quality_87d4d242c9"))}
                    </small>
                    <div
                      style={{
                        display: "flex",
                        flexWrap: "wrap",
                        gap: "var(--space-2)",
                        alignItems: "center",
                        marginTop: 4,
                      }}
                    >
                      {modelQuality && modelQuality.total_requests > 0 ? (
                        <>
                          <span
                            className="mono"
                            style={{
                              fontSize: "var(--text-xs)",
                              color: "var(--muted)",
                            }}
                            dir="ltr"
                          >
                            {text(...translatedMessage("success_5bc7e895a7"))}:{" "}
                            <strong
                              style={{
                                color:
                                  modelQuality.success_rate >= 0.99
                                    ? "var(--success)"
                                    : modelQuality.success_rate >= 0.97
                                      ? "var(--warning, #e0a030)"
                                      : "var(--danger)",
                              }}
                            >
                              {(modelQuality.success_rate * 100).toFixed(1)}%
                            </strong>
                          </span>
                          <span
                            className="mono"
                            style={{
                              fontSize: "var(--text-xs)",
                              color: "var(--muted)",
                            }}
                            dir="ltr"
                          >
                            {text(
                              ...translatedMessage("p95_latency_dc1e193e2a"),
                            )}
                            :{" "}
                            <strong
                              style={{
                                color:
                                  measuredP95 === null
                                    ? "var(--muted)"
                                    : measuredP95 < 3000
                                      ? "var(--success)"
                                      : "var(--danger)",
                              }}
                            >
                              {measuredP95 === null
                                ? noLatencyData
                                : `${measuredP95}ms`}
                            </strong>
                          </span>
                          <span
                            className="mono"
                            style={{
                              fontSize: "var(--text-xs)",
                              color: "var(--muted)",
                            }}
                            dir="ltr"
                          >
                            {text(...translatedMessage("requests_fe89af9421"))}:{" "}
                            <strong>{modelQuality.total_requests}</strong>
                          </span>
                        </>
                      ) : (
                        <span
                          style={{
                            fontSize: "var(--text-xs)",
                            color: "var(--muted)",
                          }}
                        >
                          {text(
                            ...translatedMessage(
                              "no_request_data_yet_61bf0d64a8",
                            ),
                          )}
                        </span>
                      )}
                      <Link
                        to={`/console/service-quality?model=${encodeURIComponent(model.id)}`}
                        style={{
                          display: "inline-flex",
                          alignItems: "center",
                          gap: 4,
                          color: "var(--brand)",
                          fontSize: "var(--text-xs)",
                          textDecoration: "none",
                        }}
                      >
                        <ShieldCheck size={13} />
                        {text(...translatedMessage("view_trends_4f4ace09ff"))}
                      </Link>
                    </div>
                  </div>
                </section>
                <ModelRequestTrend model={model.id} />
                {model.kind === "smart" && (
                  <section className="access-panel">
                    <h2>
                      <Sparkles size={20} />
                      {text(
                        ...translatedMessage("cost_first_routing_ba73d27fea"),
                      )}
                    </h2>
                    <p>
                      {text(
                        ...translatedMessage(
                          "select_the_lowest_estimated_cost_among_author_74f86a558e",
                        ),
                      )}
                    </p>
                    <p>
                      {text(
                        ...translatedMessage(
                          "this_is_a_routing_strategy_not_another_servic_a670c495ae",
                        ),
                      )}
                    </p>
                  </section>
                )}
                <section className="access-panel access-public-comparison">
                  <h2>
                    {text(
                      ...translatedMessage(
                        "official_reference_and_platform_prices_562ddae32f",
                      ),
                    )}
                  </h2>
                  <p className="access-muted">
                    {text(
                      ...translatedMessage(
                        "backend_quotes_include_the_current_group_mult_8bd30ae6de",
                      ),
                    )}
                  </p>
                  <CatalogPriceComparison
                    model={model}
                    plazaQuote={{ model, group: quoteGroup }}
                    detailed
                  />
                </section>
                <section className="access-panel">
                  <div className="access-panel-heading">
                    <h2>
                      {text(
                        ...translatedMessage(
                          "your_service_group_rates_5803a74853",
                        ),
                      )}
                    </h2>
                    <label className="access-inline-label">
                      {text(...translatedMessage("service_group_1c6f8adf41"))}
                      <select
                        value={tier ?? ""}
                        disabled={!groups.length}
                        onChange={(e) => setTier(e.target.value)}
                      >
                        {!groups.length && (
                          <option value="">
                            {text(
                              ...translatedMessage(
                                "no_available_group_1f7ede1b8c",
                              ),
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
                      ...translatedMessage(
                        "backend_quotes_configured_billing_units_curre_ed009cde16",
                      ),
                    )}
                  </p>
                  {!price?.available ? (
                    <div className="access-notice access-error">
                      {text(
                        ...translatedMessage(
                          "no_model_is_available_in_this_group_choose_an_40df90acba",
                        ),
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
                                  ...translatedMessage(
                                    "estimate_unavailable_56a941063d",
                                  ),
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
                                ...translatedMessage(
                                  "cache_read_write_usd_million_tokens_28165c258e",
                                ),
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
                              ...translatedMessage(
                                "model_price_table_horizontally_scrollable_d198948e1d",
                              ),
                            )}
                          >
                            <table className="access-table">
                              <thead>
                                <tr>
                                  <th>
                                    {text(
                                      ...translatedMessage("model_9c2e5b5065"),
                                    )}
                                  </th>
                                  <th>
                                    {text(
                                      ...translatedMessage(
                                        "current_quote_and_assumptions_efe6c14fca",
                                      ),
                                    )}
                                  </th>
                                  <th>
                                    {text(
                                      ...translatedMessage(
                                        "cache_unit_rates_30b06e2728",
                                      ),
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
                                      {rate.rule.mode === "tokens" &&
                                      rate.quote ? (
                                        <>
                                          {text(
                                            ...translatedMessage(
                                              "read_write_9e708504a0",
                                            ),
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
                                              ...translatedMessage(
                                                "usd_million_tokens_6f08728512",
                                              ),
                                            )}
                                          </small>
                                        </>
                                      ) : (
                                        text(
                                          ...translatedMessage(
                                            "see_configured_rule_d57c6ab039",
                                          ),
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
                          ...translatedMessage(
                            "group_multiplier_is_already_included_an_uncon_1dd3ebbd6b",
                            { p0: model.pricing.group_ratio },
                          ),
                        )}
                      </p>
                    </>
                  )}
                  {tierPrices.length > 0 && (
                    <details className="access-details">
                      <summary>
                        {text(
                          ...translatedMessage(
                            "current_group_rates_a152f11009",
                          ),
                        )}
                      </summary>
                      <p className="access-muted">
                        {text(
                          ...translatedMessage(
                            "sla_values_are_operator_published_group_targe_76fe7fb035",
                          ),
                        )}
                      </p>
                      <div
                        className="access-table-wrap"
                        tabIndex={0}
                        role="region"
                        aria-label={text(
                          ...translatedMessage(
                            "model_price_table_horizontally_scrollable_d198948e1d",
                          ),
                        )}
                      >
                        <table className="access-table">
                          <thead>
                            <tr>
                              <th>
                                {text(...translatedMessage("group_528d77dde5"))}
                              </th>
                              <th>
                                {text(
                                  ...translatedMessage(
                                    "current_effective_price_945db1759f",
                                  ),
                                )}
                              </th>
                              <th>
                                {text(
                                  ...translatedMessage(
                                    "model_success_p95_7d_799e295f93",
                                  ),
                                )}
                              </th>
                              <th>
                                {text(
                                  ...translatedMessage(
                                    "sla_targets_8d6bc914ec",
                                  ),
                                )}
                              </th>
                              <th>
                                {text(
                                  ...translatedMessage("multiplier_f33953c9ca"),
                                )}
                              </th>
                            </tr>
                          </thead>
                          <tbody>
                            {tierPrices.map(
                              ({ tier: group, price: groupPrice }) => (
                                <tr
                                  key={group.id}
                                  className={
                                    !groupPrice?.available
                                      ? "detail-group-unavailable"
                                      : ""
                                  }
                                >
                                  <td>{group.id}</td>
                                  <td>
                                    {groupPrice?.available ? (
                                      <div
                                        style={{
                                          display: "flex",
                                          alignItems: "center",
                                          gap: 8,
                                        }}
                                      >
                                        <DirectoryPrice
                                          plazaPricing={model.pricing}
                                          lang={lang}
                                          compact
                                        />
                                        <Link
                                          to={`/console/keys?create=1&model=${encodeURIComponent(model.id)}&group=${encodeURIComponent(group.id)}`}
                                          style={{
                                            fontSize: "var(--text-xs)",
                                            color: "var(--brand)",
                                            textDecoration: "none",
                                            whiteSpace: "nowrap",
                                          }}
                                        >
                                          {text(
                                            ...translatedMessage(
                                              "use_group_6fd75f3ae5",
                                            ),
                                          )}
                                        </Link>
                                      </div>
                                    ) : (
                                      <span style={{ color: "var(--muted)" }}>
                                        {text(
                                          ...translatedMessage(
                                            "unavailable_2660727e4e",
                                          ),
                                        )}
                                      </span>
                                    )}
                                  </td>
                                  <td>
                                    {modelQuality &&
                                    modelQuality.total_requests > 0 ? (
                                      <span
                                        className="mono"
                                        style={{ fontSize: "var(--text-xs)" }}
                                        dir="ltr"
                                      >
                                        {(
                                          modelQuality.success_rate * 100
                                        ).toFixed(1)}
                                        % /{" "}
                                        {measuredP95 === null
                                          ? noLatencyData
                                          : `${measuredP95}ms`}
                                      </span>
                                    ) : (
                                      <Link
                                        to={`/console/service-quality?model=${encodeURIComponent(model.id)}`}
                                        style={{
                                          fontSize: "var(--text-xs)",
                                          color: "var(--brand)",
                                          textDecoration: "none",
                                        }}
                                      >
                                        {text(
                                          ...translatedMessage(
                                            "view_baad6e0094",
                                          ),
                                        )}
                                      </Link>
                                    )}
                                  </td>
                                  <td>
                                    {groupPrice?.available ? (
                                      <ServiceLevelValue
                                        value={plazaDetail.data?.group_sla}
                                        text={text}
                                      />
                                    ) : (
                                      "—"
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
                {model.pricing.mode === "per_token" && (
                  <section className="access-panel">
                    <h2>
                      {text(...translatedMessage("pricing_details_da10e4f8c2"))}
                    </h2>
                    <p className="access-muted">
                      {text(
                        ...translatedMessage(
                          "the_following_shows_the_model_s_pricing_struc_347c248c32",
                        ),
                      )}
                    </p>
                    <div
                      className="access-table-wrap"
                      tabIndex={0}
                      role="region"
                    >
                      <table className="access-table">
                        <thead>
                          <tr>
                            <th>
                              {text(
                                ...translatedMessage("billing_item_cfe9d9f378"),
                              )}
                            </th>
                            <th>
                              {text(
                                ...translatedMessage(
                                  "rate_usd_m_tokens_67f3b51774",
                                ),
                              )}
                            </th>
                            <th>
                              {text(
                                ...translatedMessage("description_c6bae5d4e7"),
                              )}
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr>
                            <td>
                              {text(
                                ...translatedMessage("input_tokens_eaf331aa0f"),
                              )}
                            </td>
                            <td className="mono">${amounts?.input ?? "—"}</td>
                            <td>
                              {text(
                                ...translatedMessage(
                                  "prompt_and_conversation_content_in_the_reques_4143987908",
                                ),
                              )}
                            </td>
                          </tr>
                          <tr>
                            <td>
                              {text(
                                ...translatedMessage(
                                  "output_tokens_e7f8d19e9f",
                                ),
                              )}
                            </td>
                            <td className="mono">${amounts?.output ?? "—"}</td>
                            <td>
                              {text(
                                ...translatedMessage(
                                  "model_generated_response_content_b5e6f31dc4",
                                ),
                              )}
                            </td>
                          </tr>
                          {amounts?.cacheRead !== undefined && (
                            <tr>
                              <td>
                                {text(
                                  ...translatedMessage("cache_read_98c9053b0f"),
                                )}
                              </td>
                              <td className="mono">${amounts.cacheRead}</td>
                              <td>
                                {text(
                                  ...translatedMessage(
                                    "cached_input_is_billed_at_a_discounted_rate_332ab580d2",
                                  ),
                                )}
                              </td>
                            </tr>
                          )}
                          {amounts?.cacheWrite !== undefined && (
                            <tr>
                              <td>
                                {text(
                                  ...translatedMessage(
                                    "cache_write_08397c9634",
                                  ),
                                )}
                              </td>
                              <td className="mono">${amounts.cacheWrite}</td>
                              <td>
                                {text(
                                  ...translatedMessage(
                                    "initial_write_of_input_to_cache_115c2005d6",
                                  ),
                                )}
                              </td>
                            </tr>
                          )}
                        </tbody>
                      </table>
                    </div>
                    <div
                      className="pricing-mobile-cards"
                      style={{ display: "none" }}
                    >
                      {[
                        {
                          label: text(
                            ...translatedMessage("input_tokens_eaf331aa0f"),
                          ),
                          value: amounts?.input,
                        },
                        {
                          label: text(
                            ...translatedMessage("output_tokens_e7f8d19e9f"),
                          ),
                          value: amounts?.output,
                        },
                        ...(amounts?.cacheRead !== undefined
                          ? [
                              {
                                label: text(
                                  ...translatedMessage("cache_read_98c9053b0f"),
                                ),
                                value: amounts.cacheRead,
                              },
                            ]
                          : []),
                        ...(amounts?.cacheWrite !== undefined
                          ? [
                              {
                                label: text(
                                  ...translatedMessage(
                                    "cache_write_08397c9634",
                                  ),
                                ),
                                value: amounts.cacheWrite,
                              },
                            ]
                          : []),
                      ].map((item) => (
                        <div key={item.label} className="pricing-mobile-card">
                          <span className="pricing-mobile-label">
                            {item.label}
                          </span>
                          <span className="mono pricing-mobile-value">
                            ${item.value ?? "—"}
                          </span>
                        </div>
                      ))}
                    </div>
                    <p className="access-muted">
                      {text(
                        ...translatedMessage(
                          "current_group_multiplier_included_above_histo_27a0cd3615",
                          { p0: model.pricing.group_ratio },
                        ),
                      )}
                    </p>
                  </section>
                )}
                {(model.pricing.mode === "tiered" ||
                  model.pricing.mode === "expression") &&
                  model.pricing.expression?.trim() && (
                    <section className="access-panel">
                      <h2>
                        {text(
                          ...translatedMessage("expression_pricing_d19d7f5185"),
                        )}
                      </h2>
                      <p className="access-muted">
                        {text(
                          ...translatedMessage(
                            "this_model_uses_expression_based_pricing_actu_e99413f585",
                          ),
                        )}
                      </p>
                      <div
                        className="docs-code"
                        style={{ marginBottom: "var(--space-4)" }}
                      >
                        <div>
                          <span>
                            {text(
                              ...translatedMessage(
                                "pricing_expression_b4a2e43450",
                              ),
                            )}
                          </span>
                        </div>
                        <pre dir="ltr" tabIndex={0}>
                          <code className="mono">
                            {model.pricing.expression}
                          </code>
                        </pre>
                      </div>
                    </section>
                  )}

                {model.pricing.mode === "per_request" &&
                  model.pricing.per_request !== null &&
                  model.pricing.per_request > 0 && (
                    <section className="access-panel">
                      <h2>
                        {text(
                          ...translatedMessage(
                            "per_request_pricing_c36d9156c1",
                          ),
                        )}
                      </h2>
                      <p className="access-muted">
                        {text(
                          ...translatedMessage(
                            "this_model_is_billed_per_request_at_a_fixed_r_2ea16ec328",
                          ),
                        )}
                      </p>
                      <div
                        className="access-table-wrap"
                        tabIndex={0}
                        role="region"
                      >
                        <table className="access-table">
                          <thead>
                            <tr>
                              <th>
                                {text(
                                  ...translatedMessage(
                                    "billing_item_cfe9d9f378",
                                  ),
                                )}
                              </th>
                              <th>
                                {text(
                                  ...translatedMessage(
                                    "rate_usd_request_dc80393580",
                                  ),
                                )}
                              </th>
                              <th>
                                {text(
                                  ...translatedMessage(
                                    "description_c6bae5d4e7",
                                  ),
                                )}
                              </th>
                            </tr>
                          </thead>
                          <tbody>
                            <tr>
                              <td>
                                {text(
                                  ...translatedMessage(
                                    "per_request_3e12d7736d",
                                  ),
                                )}
                              </td>
                              <td className="mono">
                                ${model.pricing.per_request.toFixed(6)}
                              </td>
                              <td>
                                {text(
                                  ...translatedMessage(
                                    "a_fixed_charge_per_api_request_regardless_of__ec7b06912b",
                                  ),
                                )}
                              </td>
                            </tr>
                          </tbody>
                        </table>
                      </div>
                    </section>
                  )}
                {supportsClient && (
                  <section className="access-panel">
                    <h2>
                      {text(
                        ...translatedMessage(
                          "select_the_model_in_your_tool_9ff2c1dcb0",
                        ),
                      )}
                    </h2>
                    <p>
                      {text(
                        ...translatedMessage(
                          "the_connection_center_generates_protocol_endp_0d55b9a403",
                        ),
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
                          ...translatedMessage(
                            "open_connection_center_dc211175ab",
                          ),
                        )}
                        <ArrowRight size={16} />
                      </Link>
                    </Button>
                  </section>
                )}
              </>
            )}
          </>
        )}
      </main>
    </div>
  );
}
