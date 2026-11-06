import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";

import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  Copy,
  Plus,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Drawer } from "../components/ui/overlay";
import { TierBadge } from "../components/tier-badge";
import { formatMoney } from "../lib/money";
import { useProductPreview, type PreviewRequest } from "../lib/product-preview";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useLedger, adaptLedgerEntry } from "../lib/api/billing";
import { copyText } from "./code-example";
import "../components/demo-scenarios.css";
import { PricingSnapshot } from "../components/pricing-snapshot";
import { SettledBilling } from "../components/settled-billing";
import { callTypeLabel } from "../lib/request-metadata";
import { useSystemAccess, isAdmin } from "../lib/system-access";

const errors: Record<string, [string, string, string]> = {
  insufficient_balance: translatedMessage(
    "the_balance_is_insufficient_this_request_was__e79ecf1371",
  ),
  key_quota_exceeded: translatedMessage(
    "this_request_s_maximum_estimated_cost_exceeds_2cedfd71a4",
  ),
  model_unauthorized: translatedMessage(
    "this_model_is_not_authorized_for_the_key_sele_18910965cb",
  ),
  protocol_incompatible: translatedMessage(
    "the_protocol_is_incompatible_with_this_model__8be2affeea",
  ),
  capability_incompatible: translatedMessage(
    "no_authorized_model_supports_the_requested_ca_27ef6f00e2",
  ),
  tool_chain_incompatible: translatedMessage(
    "this_tool_chain_is_pinned_to_a_compatible_mod_b73c756cb3",
  ),
  unknown_model: translatedMessage(
    "the_requested_model_id_does_not_exist_no_subs_bbbdb0fc06",
  ),
  model_unavailable: translatedMessage(
    "this_model_is_offline_or_unavailable_in_this__bc997ca21d",
  ),
  ip_unauthorized: translatedMessage(
    "the_simulated_source_address_is_outside_the_k_ded81bb424",
  ),
  key_disabled: translatedMessage(
    "this_key_is_disabled_enable_it_or_use_another_a86607b5be",
  ),
  key_expired: translatedMessage(
    "this_key_has_expired_update_its_validity_or_u_478bcfff1a",
  ),
  key_not_found: translatedMessage(
    "the_key_was_not_found_select_a_valid_key_b9a37fe20a",
  ),
  upstream_unavailable: translatedMessage(
    "the_simulated_upstream_failed_restore_the_dem_752832a535",
  ),
  account_unavailable: translatedMessage(
    "the_account_cannot_make_requests_in_its_curre_2a40e45795",
  ),
  invalid_request: translatedMessage(
    "request_parameters_are_invalid_preserve_your__8261e983de",
  ),
  pricing_invalid: translatedMessage(
    "the_pricing_rule_or_required_usage_is_invalid_30d867f534",
  ),
  stream_interrupted: translatedMessage(
    "the_simulated_stream_was_interrupted_check_re_155b5ccd92",
  ),
};

export default function RequestInspector({
  record,
  timezone = "Asia/Dubai",
  onClose,
  onRetry,
}: {
  record: PreviewRequest | null;
  timezone?: string;
  onClose: () => void;
  onRetry: () => void;
}) {
  const { i18n, t } = useTranslation();
  const lang = i18n.language.startsWith("zh")
    ? "zh"
    : i18n.language.startsWith("ar")
      ? "ar"
      : "en";
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const { keys } = useBackendKeys();
  const ledgerQuery = useLedger(record?.trace_id);
  const ledger = (ledgerQuery.data ?? []).map(adaptLedgerEntry) as any[];
  const setOutage = useProductPreview((s) => s.demoSetUpstreamOutage);
  const role = useSystemAccess((s) => s.role);
  const failed = record?.status !== "success";
  const error = record?.error_code ? errors[record.error_code] : undefined;
  const reason = error
    ? error[lang === "zh" ? 0 : lang === "ar" ? 2 : 1]
    : text(
        record?.error_message ?? "请检查请求配置后重试。",
        "Review the request configuration, then retry.",
        "راجع إعداد الطلب ثم أعد المحاولة.",
      );
  const key = keys.find((item) => item.id === record?.key_id);
  const entry = ledger.find((item) => item.trace_id === record?.trace_id);
  const requestFix = [
    "unknown_model",
    "model_unavailable",
    "capability_incompatible",
    "tool_chain_incompatible",
    "invalid_request",
    "pricing_invalid",
  ].includes(record?.error_code ?? "");
  const actionPath =
    record?.error_code === "insufficient_balance"
      ? "/console/billing/recharge"
      : record?.error_code === "protocol_incompatible"
        ? "/console/connect"
        : "/console/keys";
  const actionLabel =
    record?.error_code === "insufficient_balance"
      ? text(...translatedMessage("simulated_top_up_891a80067c"))
      : record?.error_code === "protocol_incompatible"
        ? text(...translatedMessage("check_configuration_e06cde4c52"))
        : text(...translatedMessage("manage_key_access_limits_7986c270a9"));
  return (
    <Drawer
      open={!!record}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={text(...translatedMessage("request_details_1c3ba2496f"))}
      description={
        record?.source === "api"
          ? text(
              ...translatedMessage(
                "gateway_record_recorded_charge_and_usage_sour_3d915fd5d6",
              ),
            )
          : text(
              ...translatedMessage(
                "simulated_call_one_business_record_one_charge_aada7035f8",
              ),
            )
      }
      footer={
        <>
          <Button
            onClick={() =>
              record &&
              copyText(
                JSON.stringify(
                  {
                    trace_id: record.trace_id,
                    requested_model: record.requested_model ?? record.model,
                    actual_model:
                      record.actual_model ??
                      (record.status === "success" ? record.model : null),
                    key_id: record.key_id,
                    status: record.status,
                    error_code: record.error_code,
                    billing_status: record.billing_status,
                    cost_nano: record.cost_nano,
                    usage_source: record.usage_source,
                    pricing_revision: record.pricing_revision,
                    input_rate_usd: record.input_rate_usd,
                    output_rate_usd: record.output_rate_usd,
                    pricing_snapshot: record.pricing_snapshot,
                    metered_usage: record.metered_usage,
                    ledger_id: entry?.id,
                  },
                  null,
                  2,
                ),
                t("copied"),
                t("copyFailed"),
              )
            }
          >
            <Copy size={14} />
            {text(...translatedMessage("copy_diagnostics_807fda1c4f"))}
          </Button>
          <Button
            variant="primary"
            onClick={onRetry}
            disabled={record?.metered_demo && !isAdmin(role)}
          >
            {text(
              failed ? "保留输入并重试" : "复用请求验证",
              failed ? "Retry with input" : "Reuse request",
              failed ? "إعادة المحاولة بالمدخلات" : "إعادة استخدام الطلب",
            )}
            <ArrowRight size={14} />
          </Button>
        </>
      }
    >
      {record && (
        <>
          <div className={`request-outcome ${failed ? "is-failed" : ""}`}>
            {failed ? <CircleAlert size={20} /> : <CheckCircle2 size={20} />}
            <h3>
              {record.source === "api"
                ? record.outcome === "cancelled"
                  ? text(...translatedMessage("request_cancelled_76abde8a23"))
                  : failed
                    ? text(
                        ...translatedMessage("api_request_failed_45751177ef"),
                      )
                    : text(
                        ...translatedMessage(
                          "api_request_succeeded_59c37cd2d7",
                        ),
                      )
                : record.outcome === "cancelled"
                  ? text(...translatedMessage("request_cancelled_76abde8a23"))
                  : failed
                    ? text(
                        ...translatedMessage("request_incomplete_d09066c8de"),
                      )
                    : text(
                        ...translatedMessage(
                          "simulated_request_completed_78801548e1",
                        ),
                      )}
            </h3>
            <p>
              {record.source === "api"
                ? text(
                    ...translatedMessage(
                      "this_record_comes_from_gateway_logs_and_shows_c1fe7c83f7",
                    ),
                  )
                : failed
                  ? reason
                  : text(
                      ...translatedMessage(
                        "this_local_demo_result_verifies_configuration_3437c28bce",
                      ),
                    )}
            </p>
            {failed && record.source !== "api" && (
              <div className="request-inspector-actions">
                {record.error_code === "upstream_unavailable" ? (
                  <Button
                    onClick={() => {
                      setOutage(false);
                      onRetry();
                    }}
                  >
                    {text(
                      ...translatedMessage("restore_upstream_retry_8fc5a87c56"),
                    )}
                  </Button>
                ) : requestFix ? (
                  <Button onClick={onRetry}>
                    {text(
                      ...translatedMessage(
                        "edit_request_with_input_ba92f3ac20",
                      ),
                    )}
                  </Button>
                ) : (
                  <Button asChild>
                    <Link to={actionPath} onClick={onClose}>
                      {actionLabel}
                    </Link>
                  </Button>
                )}
              </div>
            )}
          </div>
          <div className="request-charge">
            <span>
              {BigInt(record.cost_nano) > 0n
                ? text(...translatedMessage("charged_c6cfa19bff"))
                : record.status === "success"
                  ? text(...translatedMessage("settled_zero_cost_08cab6a891"))
                  : text(
                      ...translatedMessage(
                        "not_charged_balance_and_quota_unchanged_bc26199b73",
                      ),
                    )}
            </span>
            <strong className="money">
              {formatMoney(record.cost_nano, "detail")}
            </strong>
          </div>
          <dl className="product-detail-list">
            <div>
              <dt>
                {text(...translatedMessage("business_record_3ef74e7812"))}
              </dt>
              <dd className="mono" dir="ltr">
                {record.trace_id}
              </dd>
            </div>
            <div>
              <dt>
                {text(...translatedMessage("requested_model_59bacdc290"))}
              </dt>
              <dd className="mono" dir="ltr">
                {record.requested_model ?? record.model}
              </dd>
            </div>
            <div>
              <dt>{text(...translatedMessage("actual_model_87fe7e61b2"))}</dt>
              <dd className="mono" dir="ltr">
                {record.actual_model ??
                  (record.status === "success" ? record.model : "—")}
              </dd>
            </div>
            <div>
              <dt>{text(...translatedMessage("key_1108ef42bc"))}</dt>
              <dd>
                {record.key_deleted
                  ? text(...translatedMessage("deleted_key_571647d876"))
                  : (record.key_name ??
                    key?.name ??
                    record.key_id ??
                    text(...translatedMessage("historical_sample_95c37e035f")))}
              </dd>
            </div>
            <div>
              <dt>{text(...translatedMessage("service_group_1c6f8adf41"))}</dt>
              <dd>
                {record.service_group ||
                  (record.tier ? <TierBadge id={record.tier} /> : "—")}
              </dd>
            </div>
            <div>
              <dt>{text(...translatedMessage("vendor_5dc28b7ea6"))}</dt>
              <dd>{record.vendor || "—"}</dd>
            </div>
            <div>
              <dt>{text(...translatedMessage("call_type_481db78ba3"))}</dt>
              <dd>{callTypeLabel(record.call_type, i18n.language)}</dd>
            </div>
            <div>
              <dt>
                {text(...translatedMessage("automatic_switch_e1171667b9"))}
              </dt>
              <dd>
                {record.fallback
                  ? text(
                      ...translatedMessage("automatically_switched_cd94edcbb0"),
                    )
                  : text(
                      ...translatedMessage("no_automatic_switch_72bec5356c"),
                    )}
              </dd>
            </div>
            <div>
              <dt>
                {text(
                  ...translatedMessage("cache_read_write_tokens_824bb2f440"),
                )}
              </dt>
              <dd>
                {record.cache_read_tokens ?? 0} /{" "}
                {record.cache_write_tokens ?? 0}
              </dd>
            </div>
            {record.billing_detail && (
              <SettledBilling
                detail={record.billing_detail}
                timezone={timezone}
              />
            )}
            {record.usage_source === "admission_estimate" && (
              <div>
                <dt>{text(...translatedMessage("usage_source_94c3143825"))}</dt>
                <dd>
                  {text(
                    ...translatedMessage(
                      "estimated_billing_usage_complete_provider_tok_2d18cc5438",
                    ),
                  )}
                </dd>
              </div>
            )}
            {record.billing_detail ? (
              <div>
                <dt>
                  {text(...translatedMessage("input_output_tokens_52437a9a3b"))}
                </dt>
                <dd>
                  {record.input_tokens} / {record.output_tokens}
                </dd>
              </div>
            ) : record.pricing_snapshot ? (
              <PricingSnapshot snapshot={record.pricing_snapshot} />
            ) : record.metered_demo ? (
              <div>
                <dt>
                  {text(
                    ...translatedMessage(
                      "simulated_usage_unsettled_7636142d95",
                    ),
                  )}
                </dt>
                <dd>
                  <code style={{ overflowWrap: "anywhere" }}>
                    {JSON.stringify(record.metered_usage)}
                  </code>
                  <small className="request-rate-note">
                    {record.error_message}
                  </small>
                </dd>
              </div>
            ) : (
              <>
                <div>
                  <dt>
                    {text(
                      ...translatedMessage("input_output_tokens_57071059f4"),
                    )}
                  </dt>
                  <dd>
                    {record.usage_source === "admission_estimate"
                      ? text(
                          ...translatedMessage(
                            "the_provider_did_not_report_usage_billing_use_1da92aa07e",
                          ),
                        )
                      : `${record.input_tokens.toLocaleString()} / ${record.output_tokens.toLocaleString()}`}
                  </dd>
                </div>
                <div>
                  <dt>
                    {text(
                      ...translatedMessage(
                        "input_output_rate_at_request_d42a64db8d",
                      ),
                    )}
                  </dt>
                  <dd>
                    {record.input_rate_usd !== undefined &&
                    record.output_rate_usd !== undefined
                      ? `$${record.input_rate_usd} / $${record.output_rate_usd}`
                      : record.input_price_nano !== undefined &&
                          record.output_price_nano !== undefined
                        ? `${formatMoney(record.input_price_nano, "detail")} / ${formatMoney(record.output_price_nano, "detail")}`
                        : text(...translatedMessage("not_recorded_4bbcb12676"))}
                    <small className="request-rate-note">
                      {text(
                        ...translatedMessage(
                          "usd_1m_tokens_group_multiplier_included_21a5b91b49",
                        ),
                      )}
                    </small>
                  </dd>
                </div>
              </>
            )}
            <div>
              <dt>{text(...translatedMessage("rate_snapshot_b2e43d5884"))}</dt>
              <dd className="mono" dir="ltr">
                {record.billing_detail?.priced_at ??
                  record.pricing_revision ??
                  text(
                    ...translatedMessage(
                      "historical_tariff_not_recorded_3a33290c1b",
                    ),
                  )}
              </dd>
            </div>
            <div>
              <dt>{text(...translatedMessage("ledger_entry_c7dfa3438d"))}</dt>
              <dd>
                {entry ? (
                  <Link
                    to={`/console/billing?trace=${encodeURIComponent(record.trace_id)}`}
                    onClick={onClose}
                  >
                    {entry.id}
                  </Link>
                ) : (
                  text(...translatedMessage("no_debit_entry_aab01374b3"))
                )}
              </dd>
            </div>
            {record.request_source === "playground" && (
              <div>
                <dt>{text(...translatedMessage("source_7986b56c96"))}</dt>
                <dd>{text(...translatedMessage("playground_8e85ca0e84"))}</dd>
              </div>
            )}
            <div>
              <dt>{text(...translatedMessage("time_d70a9315f6"))}</dt>
              <dd>
                {new Date(record.time).toLocaleString(regionalLocale(lang), {
                  timeZone: timezone,
                  timeZoneName: "short",
                })}
              </dd>
            </div>
          </dl>
          <p className="request-inspector-note">
            {text(
              ...translatedMessage(
                "charges_use_the_saved_settlement_missing_hist_6910db4dcf",
              ),
            )}
          </p>
          <details className="technical-details">
            <summary>
              {text(...translatedMessage("advanced_diagnostics_d7a77fc80e"))}
              <Plus size={15} />
            </summary>
            <dl className="product-detail-list">
              <div>
                <dt>{text(...translatedMessage("error_code_2ca4ba8a4b"))}</dt>
                <dd className="mono">{record.error_code ?? "—"}</dd>
              </div>
              <div>
                <dt>
                  {text(
                    ...translatedMessage("duration_first_token_81bea9d0fe"),
                  )}
                </dt>
                <dd>
                  {record.duration_ms} / {record.ttft_ms}{" "}
                  {text(...translatedMessage("ms_7184894997"))}
                </dd>
              </div>
              <div>
                <dt>{text(...translatedMessage("exact_cost_b82f4aeaa1"))}</dt>
                <dd>{formatMoney(record.cost_nano, "detail")}</dd>
              </div>
              <div>
                <dt>{text(...translatedMessage("source_7986b56c96"))}</dt>
                <dd>
                  {record.metered_demo
                    ? text(
                        ...translatedMessage(
                          "admin_billing_simulation_no_media_generated_7471087256",
                        ),
                      )
                    : record.request_source === "playground"
                      ? text(...translatedMessage("playground_8e85ca0e84"))
                      : record.source === "api"
                        ? text(...translatedMessage("api_request_7e17b3af15"))
                        : record.source === "playground"
                          ? text(
                              ...translatedMessage(
                                "playground_simulation_322edd136e",
                              ),
                            )
                          : text(
                              ...translatedMessage(
                                "simulated_client_request_f6bb40ad28",
                              ),
                            )}
                </dd>
              </div>
            </dl>
            {record.request && (
              <pre className="request-inspector-code" dir="ltr">
                {JSON.stringify(record.request, null, 2)}
              </pre>
            )}
          </details>
        </>
      )}
    </Drawer>
  );
}
