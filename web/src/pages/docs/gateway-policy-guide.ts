import { message as translatedMessage } from "../../i18n/messages";
type Text = (zh: string, en: string, ar: string) => string;

export function gatewayPolicyGuide(text: Text) {
  return {
    quota: text(
      ...translatedMessage(
        "account_api_quota_and_remaining_key_quota_are_c91ba2bf9e",
      ),
    ),
    wallet: text(
      ...translatedMessage(
        "the_payment_wallet_and_api_quota_use_separate_cfdaf03ca0",
      ),
    ),
    defaults: text(
      ...translatedMessage(
        "these_are_current_code_defaults_not_live_sett_ce8ea70171",
      ),
    ),
    rateHelp: text(
      ...translatedMessage(
        "rate_limits_return_429_rate_limit_exceeded_re_52a6ae5be3",
      ),
    ),
    tokenFields: [
      [
        "group",
        text(
          ...translatedMessage(
            "service_group_selects_eligible_channels_and_b_4c2336370f",
          ),
        ),
      ],
      [
        "auto_groups",
        text(
          ...translatedMessage(
            "comma_separated_candidate_group_ids_for_auto__49a973e712",
          ),
        ),
      ],
      [
        "model_limits",
        text(
          ...translatedMessage(
            "comma_separated_authorized_model_ids_empty_ad_e55f8f0922",
          ),
        ),
      ],
      [
        "allow_ips",
        text(
          ...translatedMessage(
            "comma_separated_source_ip_or_cidr_allowlist_e_3dbb764787",
          ),
        ),
      ],
      [
        "remain_quota",
        text(
          ...translatedMessage(
            "remaining_key_quota_with_unlimited_quota_fals_6bdf8fa2a2",
          ),
        ),
      ],
      [
        "unlimited_quota",
        text(
          ...translatedMessage(
            "true_skips_the_key_quota_check_not_account_ap_436c583e4a",
          ),
        ),
      ],
    ],
    // Defaults mirror config/runtime.go and the middleware rate-limit constants.
    rateRows: [
      [
        text(...translatedMessage("ip_rate_limit_59dda8e6fc")),
        "240 req/min",
        text(...translatedMessage("api_requests_per_source_ip_ad4a9f8e3a")),
      ],
      [
        text(...translatedMessage("user_rate_limit_de52125835")),
        "120 req/min",
        text(
          ...translatedMessage("relay_requests_for_the_same_user_062eda2886"),
        ),
      ],
      [
        text(...translatedMessage("key_rate_limit_f948446671")),
        "60 req/min",
        text(
          ...translatedMessage(
            "relay_requests_for_the_same_api_key_ea4c8992cd",
          ),
        ),
      ],
      [
        text(...translatedMessage("model_rate_limit_bcb1b141f8")),
        "200 req/min",
        text(
          ...translatedMessage(
            "shared_model_rate_bucket_individual_overrides_7fbad631ff",
          ),
        ),
      ],
    ],
  };
}
