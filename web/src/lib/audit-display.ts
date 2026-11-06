import { message as translatedMessage } from "../i18n/messages";

import { staffRoleName } from "./api/staff";
import { formatFinanceAmount } from "./finance-format";

export type AuditText = (zh: string, en: string, ar: string) => string;
type Labels = Record<string, readonly [string, string, string]>;

const actions: Labels = {
  announcement_published: translatedMessage(
    "publish_incident_notice_c30bf65693",
  ),
  announcement_updated: translatedMessage("update_incident_notice_778e897e68"),
  announcement_resolved: translatedMessage(
    "resolve_incident_notice_8f1834adc0",
  ),
  resource_sync: translatedMessage(
    "force_full_resource_synchronization_ff6a39e942",
  ),
  staff_password_authenticated: translatedMessage(
    "verify_staff_password_fb1239f502",
  ),
  staff_password_authenticated_rejected: translatedMessage(
    "staff_password_verification_failed_f38797d7ea",
  ),
  staff_2fa_verified: translatedMessage("sign_in_to_back_office_643e4f59a1"),
  staff_2fa_verified_rejected: translatedMessage("staff_2fa_failed_93526b04d8"),
  staff_password_changed: translatedMessage("change_staff_password_6295d05e43"),
  staff_password_changed_rejected: translatedMessage(
    "staff_password_change_failed_379e6958ca",
  ),
  staff_created: translatedMessage("create_staff_account_3c1de3e79a"),
  staff_updated: translatedMessage("update_staff_account_0fbc5c21fd"),
  staff_root_initialized: translatedMessage(
    "initialize_super_administrator_cc3eefff58",
  ),
  staff_2fa_reset: translatedMessage("reset_staff_2fa_106922ce87"),
  customer_enabled: translatedMessage("enable_customer_account_ea0482e190"),
  customer_disabled: translatedMessage("disable_customer_account_47d75949cf"),
  customer_key_disabled: translatedMessage("disable_customer_key_a8be825070"),
  customer_concurrency_updated: translatedMessage(
    "change_concurrency_limit_c94428357d",
  ),
  customer_sessions_revoked: translatedMessage(
    "revoke_customer_sessions_3609a85d9e",
  ),
  balance_gift: translatedMessage("add_account_credit_c2f2ba6420"),
  balance_compensation: translatedMessage("add_compensation_3add6ad1f0"),
  balance_offline_payment: translatedMessage(
    "record_offline_payment_9a68b437bf",
  ),
  balance_deduction: translatedMessage("deduct_account_credit_8a26dd5fac"),
  pricing_changed: translatedMessage("change_pricing_configuration_defcdf1dbe"),
  model_pricing_changed: translatedMessage("change_model_price_a8cb664f5d"),
  model_reference_updated: translatedMessage(
    "update_official_reference_prices_e26c8a338b",
  ),
  service_group_multiplier_changed: translatedMessage(
    "change_service_group_multiplier_cd156addbb",
  ),
  standard_model_saved: translatedMessage("save_standard_model_505ec331d6"),
  standard_model_deleted: translatedMessage("delete_standard_model_72b7348ef8"),
  deployment_profile_updated: translatedMessage(
    "update_region_profile_2b52d00f54",
  ),
  legal_version_scheduled: translatedMessage(
    "schedule_legal_document_version_173ebd58cb",
  ),
  reconciliation_run: translatedMessage("run_reconciliation_0befbd0b23"),
  payment_statement_imported: translatedMessage(
    "import_payment_statement_fa3e80e60e",
  ),
};

const objects: Labels = {
  announcements: translatedMessage("incident_notice_0600652e70"),
  staff: translatedMessage("staff_account_206adf466b"),
  customers: translatedMessage("customer_account_92d663dd47"),
  keys: translatedMessage("access_key_9a2e8c436b"),
  models: translatedMessage("standard_model_18b603bd24"),
  pricing: translatedMessage("pricing_configuration_7a792e29a3"),
  "service-group": translatedMessage("service_group_1c6f8adf41"),
  "region-profile": translatedMessage("region_profile_15fb48e9d0"),
  legal: translatedMessage("legal_document_e6c062428e"),
  reconciliation: translatedMessage("reconciliation_report_617ed576b7"),
  payment_statement: translatedMessage("payment_statement_9258011802"),
};

const fields: Labels = {
  provider: translatedMessage("payment_provider_28f57de213"),
  day: translatedMessage("statement_date_86676bc86f"),
  rows: translatedMessage("payment_count_2f6c9c68f6"),
  total_nano: translatedMessage("statement_amount_7404a5386d"),
  supersedes: translatedMessage("previous_version_70a428cebf"),
  digest: translatedMessage("file_checksum_2e66ed3a25"),
  username: translatedMessage("username_52bf46ce51"),
  platform_admin: translatedMessage("platform_administrator_489c68ad11"),
  role: translatedMessage("role_5b688c6fd8"),
  status: translatedMessage("status_dda39dcd63"),
  two_factor_bound: translatedMessage("two_factor_authentication_dbc5a3c12c"),
  must_change_password: translatedMessage(
    "password_change_required_b0c1f863ff",
  ),
  concurrency_limit: translatedMessage("concurrency_limit_a812217281"),
  revoked: translatedMessage("sessions_revoked_01bba8e864"),
  balance_nano: translatedMessage("balance_2afee946f4"),
  amount_nano: translatedMessage("adjustment_amount_22f393d3d6"),
  user_id: translatedMessage("account_id_267663f396"),
  reports: translatedMessage("report_count_b85b36b981"),
  component: translatedMessage("pricing_component_b6e0b92bb3"),
  value: translatedMessage("value_d0e4887528"),
  model: translatedMessage("model_9c2e5b5065"),
  name: translatedMessage("name_e639662a6f"),
  vendor: translatedMessage("vendor_5dc28b7ea6"),
  vendor_display: translatedMessage("vendor_display_name_9eeb355fb8"),
  kind: translatedMessage("type_b91a24e5ad"),
  protocols: translatedMessage("protocols_0fc0348ab3"),
  capabilities: translatedMessage("capabilities_cd2d018425"),
  tags: translatedMessage("tags_f8eeb96e24"),
  context_length: translatedMessage("context_length_7355a0b91b"),
  max_output: translatedMessage("maximum_output_tokens_0bf4ad2271"),
  description: translatedMessage("description_c6bae5d4e7"),
  listed: translatedMessage("listing_status_e12bdab353"),
  sort_weight: translatedMessage("sort_weight_8d8cdaea21"),
  revision: translatedMessage("revision_1a3b92b341"),
  version: translatedMessage("version_d07f8f123e"),
  created_at: translatedMessage("creation_timestamp_012f122ddc"),
  updated_at: translatedMessage("update_timestamp_82ee873bc0"),
  effective_at: translatedMessage("effective_timestamp_bff9cf2924"),
  reference: translatedMessage("official_reference_7896199b9d"),
  official_source_url: translatedMessage("official_price_source_3a5c46e468"),
  official_verified_date: translatedMessage("verification_date_e695c40f81"),
  official_conditions: translatedMessage(
    "reference_price_conditions_805f372bc5",
  ),
  official_historical: translatedMessage("historical_reference_879b0f1615"),
  official_input_per_m: translatedMessage(
    "official_input_usd_million_tokens_0f8af93274",
  ),
  official_output_per_m: translatedMessage(
    "official_output_usd_million_tokens_aab7040e71",
  ),
  official_cache_read_per_m: translatedMessage(
    "official_cache_read_usd_million_tokens_af8e8c4d3e",
  ),
  official_cache_write_per_m: translatedMessage(
    "official_cache_write_usd_million_tokens_1966ef3cf4",
  ),
  official_per_request: translatedMessage(
    "official_price_per_request_usd_6da84101c6",
  ),
  per_request_usd: translatedMessage("price_per_request_usd_9e00daa2cc"),
  score: translatedMessage("capability_score_f7a553d452"),
  compatible_groups: translatedMessage("compatible_groups_705a3e9423"),
  region_id: translatedMessage("region_id_9d764a8947"),
  brand: translatedMessage("brand_eff25bbb42"),
  domain: translatedMessage("domain_59cd72edd7"),
  logo_url: translatedMessage("logo_url_5c3cd83df9"),
  default_language: translatedMessage("default_language_af6457f09c"),
  languages: translatedMessage("languages_1fca20de33"),
  timezone: translatedMessage("timezone_589797f8d4"),
  currency: translatedMessage("currency_912b49ef57"),
  allowed_vendors: translatedMessage("allowed_vendors_ae00df057f"),
  login_methods: translatedMessage("login_methods_63cf073bb4"),
  payment_channels: translatedMessage("payment_channels_d3f84608b2"),
  payment_config_refs: translatedMessage(
    "payment_configuration_references_5a72e13b16",
  ),
  cookie_consent: translatedMessage("cookie_consent_250eb89009"),
  features: translatedMessage("features_b58eba93ee"),
  store_request_content: translatedMessage("store_request_content_dab33e2102"),
  playground: translatedMessage("playground_99cde0b0d9"),
  models: translatedMessage("model_catalog_ed043af818"),
  smart_entry: translatedMessage("smart_entry_95929b20ea"),
  video: translatedMessage("video_f038b3ffa9"),
  arabic: translatedMessage("arabic_f8904e8852"),
  model_ratio: translatedMessage("input_billing_ratio_9d9b5c9f9c"),
  completion_ratio: translatedMessage("output_billing_ratio_3606e5f1e4"),
  group_ratio: translatedMessage("sale_multiplier_0faa16e317"),
  group_group_ratio: translatedMessage(
    "account_group_sale_multiplier_c6753b1125",
  ),
  model_price: translatedMessage("per_request_price_usd_893a3c943c"),
  model_price_expr: translatedMessage("pricing_expression_97debc0743"),
  cache_ratio: translatedMessage("cache_billing_ratio_1cda243b8e"),
  audio_ratio: translatedMessage("audio_billing_ratio_fde0f3a5f0"),
  model_time_pricing: translatedMessage("time_pricing_factors_9000e40d77"),
  model_time_token_prices: translatedMessage("time_token_prices_706ad26ab5"),
};

function label(labels: Labels, key: string, text: AuditText): string {
  const value = labels[key];
  return Array.isArray(value) ? text(value[0], value[1], value[2]) : key;
}

export function auditActionLabel(action: string, text: AuditText): string {
  const translated = label(actions, action, text);
  return translated !== action
    ? translated
    : `${text(...translatedMessage("unrecognized_action_c21bec19e5"))} (${action})`;
}

export function auditSearchActions(query: string, text: AuditText): string[] {
  const search = query.trim().toLocaleLowerCase();
  return search
    ? Object.keys(actions).filter((key) =>
        auditActionLabel(key, text).toLocaleLowerCase().includes(search),
      )
    : [];
}

export function auditObjectLabel(kind: string, text: AuditText): string {
  return label(objects, kind, text);
}

export function auditRoleLabel(role: number, text: AuditText): string {
  if (role === 0)
    return text(...translatedMessage("role_not_recorded_b3c7d2f94f"));
  if (role === 1) return text(...translatedMessage("customer_37e417bb9d"));
  return [5, 10, 20, 100].includes(role)
    ? staffRoleName(role, text)
    : `${staffRoleName(role, text)} (${role})`;
}

function describe(value: unknown, text: AuditText, key = ""): string {
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value))
    return value.length
      ? value.map((item) => describe(item, text, key)).join(" / ")
      : "—";
  if (typeof value === "object")
    return (
      Object.entries(value)
        .map(
          ([field, item]) =>
            `${label(fields, field, text)}: ${describe(item, text, field)}`,
        )
        .join("; ") || "—"
    );
  if (key === "role" && typeof value === "number")
    return auditRoleLabel(value, text);
  if (key === "status" && value === 1)
    return text(...translatedMessage("enabled_d4fda6d86a"));
  if (key === "status" && value === 2)
    return text(...translatedMessage("disabled_11bee48d5f"));
  if (typeof value === "boolean") {
    if (key === "two_factor_bound")
      return value
        ? text(...translatedMessage("bound_261ddf4c15"))
        : text(...translatedMessage("not_bound_6a172e1958"));
    if (key === "listed")
      return value
        ? text(...translatedMessage("listed_435899385f"))
        : text(...translatedMessage("not_listed_bd0a1a8b51"));
    return value
      ? text(...translatedMessage("yes_3445b9ddec"))
      : text(...translatedMessage("no_b02901acfa"));
  }
  if (
    (key === "balance_nano" || key === "amount_nano") &&
    /^-?\d+$/.test(String(value))
  )
    return isolateValue(formatFinanceAmount(String(value)), text);
  if (key === "component") return label(fields, String(value), text);
  return String(value);
}

function isolateValue(value: string, text: AuditText): string {
  return text(...translatedMessage("ltr_f1338d980b")) === "rtl"
    ? `\u2066${value}\u2069`
    : value;
}

function parse(value: string): unknown {
  try {
    return value ? JSON.parse(value) : undefined;
  } catch {
    return value;
  }
}

export function auditSummary(value: string, text: AuditText): string {
  return describe(parse(value), text);
}

type Entry = { path: string[]; value: unknown };
function flatten(value: unknown, path: string[] = []): Entry[] {
  if (
    value &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.keys(value).length
  ) {
    return Object.entries(value).flatMap(([key, item]) =>
      flatten(item, [...path, key]),
    );
  }
  return [{ path, value }];
}

export function auditChanges(
  before: string,
  after: string,
  text: AuditText,
): string[] {
  if (!before && !after) return [];
  const old = new Map(
    flatten(parse(before)).map((entry) => [JSON.stringify(entry.path), entry]),
  );
  const next = new Map(
    flatten(parse(after)).map((entry) => [JSON.stringify(entry.path), entry]),
  );
  const result: string[] = [];
  for (const path of new Set([...old.keys(), ...next.keys()])) {
    const previous = old.get(path),
      current = next.get(path);
    if (JSON.stringify(previous?.value) === JSON.stringify(current?.value))
      continue;
    const keys = (current ?? previous)!.path;
    const key = keys.at(-1) ?? "";
    let name = keys.map((part) => label(fields, part, text)).join(" · ");
    if (key === "value" && keys.length === 1) {
      const component =
        next.get('["component"]')?.value ?? old.get('["component"]')?.value;
      if (typeof component === "string") name = label(fields, component, text);
    }
    const value = !before
      ? describe(current?.value, text, key)
      : isolateValue(
          `${describe(previous?.value, text, key)} → ${describe(current?.value, text, key)}`,
          text,
        );
    result.push(name ? `${name}: ${value}` : value);
  }
  return result;
}

export function auditAuthResult(action: string, text: AuditText): string {
  switch (action) {
    case "staff_2fa_verified":
      return text(...translatedMessage("2fa_verification_passed_446853dd95"));
    case "staff_password_authenticated":
      return text(
        ...translatedMessage("password_verified_awaiting_2fa_1e2b8633df"),
      );
    case "staff_password_changed":
      return text(...translatedMessage("password_changed_809c9e5ff3"));
    case "staff_password_authenticated_rejected":
    case "staff_password_changed_rejected":
    case "staff_2fa_verified_rejected":
      return text(...translatedMessage("verification_failed_770b92f762"));
    default:
      return "";
  }
}

export function auditHTTPAction(method: string, text: AuditText): string {
  const methods: Labels = {
    POST: translatedMessage("submit_operation_572574d337"),
    PUT: translatedMessage("update_configuration_5a4a9836cb"),
    PATCH: translatedMessage("update_configuration_5a4a9836cb"),
    DELETE: translatedMessage("delete_operation_ebf3789e9b"),
    CHANGE: translatedMessage("configuration_change_2266df19ec"),
  };
  return label(methods, method, text);
}
