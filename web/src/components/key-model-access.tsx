import { message as translatedMessage } from "../i18n/messages";
import { useId, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ArrowUpRight, Check, ChevronDown } from "lucide-react";
import type { ModelItem } from "../config/models";
import { useModelPricing } from "../lib/model-pricing";
import { useProductPreview } from "../lib/product-preview";
import type { TierId } from "../config/placeholders";
import { Field, Switch } from "./ui/primitives";
import { directoryPrices, getDirectoryModel } from "../config/model-directory";
import { DirectoryPrice } from "./directory-price";
import type { Language } from "../config/placeholders";

export const displayModelRate = (value: string) =>
  value.replace(/(\.\d*?)0+$/, "$1").replace(/\.$/, "");

export function KeyModelAccess({
  access,
  onAccessChange,
  selected,
  onSelectedChange,
  models,
  tier,
}: {
  access: "all" | "custom";
  onAccessChange: (value: "all" | "custom") => void;
  selected: string[];
  onSelectedChange: (value: string[]) => void;
  models: ModelItem[];
  tier: TierId;
}) {
  const { i18n } = useTranslation();
  const switchId = useId();
  const config = useModelPricing((s) => s.config);
  const accountId = useProductPreview((s) => s.user.id);
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const chosen = models.filter((model) => selected.includes(model.id));
  const unavailable = selected.filter(
    (id) => !models.some((model) => model.id === id),
  );
  return (
    <>
      <fieldset className="key-policy-section">
        <legend className="key-policy-legend">
          {c(...translatedMessage("model_allowlist_a482123a7e"))}
        </legend>
        <div className="key-restriction-heading">
          <label htmlFor={switchId}>
            <strong>
              {c(...translatedMessage("model_allowlist_a482123a7e"))}
            </strong>
            <small>
              {c(
                ...translatedMessage(
                  "allow_only_selected_models_including_smart_ro_c8a783c65b",
                ),
              )}
            </small>
          </label>
          <span className="key-restriction-control">
            <span>
              {access === "custom"
                ? c(...translatedMessage("enabled_d03bf0e513"))
                : c(...translatedMessage("disabled_a2a7e48b0e"))}
            </span>
            <Switch
              id={switchId}
              label={c(
                ...translatedMessage("enable_model_allowlist_b59f55a683"),
              )}
              checked={access === "custom"}
              onChange={(enabled) => onAccessChange(enabled ? "custom" : "all")}
            />
          </span>
        </div>
        {access === "all" ? (
          <p className="key-policy-hint">
            {c(
              ...translatedMessage(
                "all_group_models_are_allowed_route_entry_perm_d3f4b77fe4",
              ),
            )}
          </p>
        ) : (
          <>
            <div className="key-model-list">
              {models.map((model) => {
                const rate = directoryPrices(
                  getDirectoryModel(model.id)!,
                  tier,
                  config,
                  undefined,
                  accountId,
                ).rates[0];
                return (
                  <label
                    key={model.id}
                    className={selected.includes(model.id) ? "is-selected" : ""}
                  >
                    <input
                      type="checkbox"
                      checked={selected.includes(model.id)}
                      onChange={(event) =>
                        onSelectedChange(
                          event.target.checked
                            ? [...selected, model.id]
                            : selected.filter((id) => id !== model.id),
                        )
                      }
                    />
                    <span className="key-model-identity">
                      <strong>{model.name}</strong>
                      <small dir="ltr">{model.id}</small>
                    </span>
                    <span className="key-model-rate">
                      {rate && (
                        <DirectoryPrice
                          rate={rate}
                          lang={i18n.language as Language}
                          compact
                        />
                      )}
                    </span>
                  </label>
                );
              })}
            </div>
            <p
              className={`key-policy-hint ${chosen.length === 0 ? "is-error" : ""}`}
              role="status"
            >
              {chosen.length
                ? c(
                    ...translatedMessage(
                      "models_allowed_selecting_one_restricts_the_ke_291fab2c7f",
                      { p0: chosen.length },
                    ),
                  )
                : c(
                    ...translatedMessage(
                      "select_at_least_one_available_model_9ee5f5fb93",
                    ),
                  )}
            </p>
            {unavailable.length > 0 && (
              <p className="key-policy-hint is-warning" role="status">
                {c(
                  ...translatedMessage(
                    "previous_selections_are_unavailable_in_this_g_76ec21a3c5",
                    { p0: unavailable.length },
                  ),
                )}
              </p>
            )}
          </>
        )}
        <div className="key-policy-summary">
          <Check size={14} />
          <span>
            {c(
              ...translatedMessage(
                "smart_routing_follows_this_allowlist_explicit_b2983c9c8d",
              ),
            )}
          </span>
        </div>
        <Link to="/models" className="key-model-price-link">
          {c(
            ...translatedMessage("explore_models_and_group_prices_a654c223ea"),
          )}
          <ArrowUpRight size={13} />
        </Link>
      </fieldset>
    </>
  );
}

export function KeyAdvancedSettings({
  open,
  onOpenChange,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
}) {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  return (
    <details className="key-task-advanced" open={open}>
      <summary
        onClick={(event) => {
          event.preventDefault();
          onOpenChange(!open);
        }}
      >
        <span>
          <strong>
            {c(...translatedMessage("advanced_settings_f5b42651a8"))}
          </strong>
          <small>
            {c(...translatedMessage("model_and_ip_allowlists_e24f12d28a"))}
          </small>
        </span>
        <span className="key-advanced-action">
          {open
            ? c(...translatedMessage("collapse_e8d986c7b2"))
            : c(...translatedMessage("expand_c77fe8dab8"))}
          <ChevronDown size={16} aria-hidden="true" />
        </span>
      </summary>
      <div className="key-task-advanced-body">{children}</div>
    </details>
  );
}

export function KeyIpAllowlist({
  id,
  enabled,
  onEnabledChange,
  value,
  onChange,
}: {
  id: string;
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  value: string;
  onChange: (value: string) => void;
}) {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  return (
    <section className="key-ip-allowlist">
      <div className="key-restriction-heading">
        <label htmlFor={`${id}-enabled`}>
          <strong>{c(...translatedMessage("ip_allowlist_b4c0043c13"))}</strong>
          <small>
            {c(
              ...translatedMessage(
                "accept_requests_only_from_listed_network_addr_f1cf25c88b",
              ),
            )}
          </small>
        </label>
        <span className="key-restriction-control">
          <span>
            {enabled
              ? c(...translatedMessage("enabled_d03bf0e513"))
              : c(...translatedMessage("disabled_a2a7e48b0e"))}
          </span>
          <Switch
            id={`${id}-enabled`}
            label={c(...translatedMessage("enable_ip_allowlist_ac8ad60f7b"))}
            checked={enabled}
            onChange={onEnabledChange}
          />
        </span>
      </div>
      {enabled ? (
        <Field
          id={id}
          label={c(...translatedMessage("allowed_addresses_e713e28ca8"))}
          hint={c(
            ...translatedMessage(
              "one_per_line_at_least_one_required_supports_a_a44e4f6331",
            ),
          )}
        >
          <textarea
            id={id}
            className="input"
            value={value}
            onChange={(event) => onChange(event.target.value)}
            rows={3}
          />
        </Field>
      ) : (
        <p className="key-policy-hint">
          {c(
            ...translatedMessage(
              "source_addresses_are_unrestricted_turning_thi_e1033e066b",
            ),
          )}
        </p>
      )}
    </section>
  );
}

export function ClientModelGuide() {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  return (
    <details className="key-client-guide">
      <summary>
        {c(...translatedMessage("connect_through_cc_switch_6ace2817e3"))}
      </summary>
      <div>
        <p>
          {c(
            ...translatedMessage(
              "allow_the_models_you_need_send_the_exact_mode_f5268c207f",
            ),
          )}
        </p>
        <p>
          {c(
            ...translatedMessage(
              "claude_code_can_switch_using_model_sonnet_opu_9101a3524d",
            ),
          )}
        </p>
        <p>
          {c(
            ...translatedMessage(
              "multi_model_permission_does_not_provide_proto_3606be3750",
            ),
          )}
        </p>
      </div>
    </details>
  );
}
