import { message as translatedMessage } from "../i18n/messages";
import { useSettingsText } from "./settings-text";
import type { PricingRule, PricingAttributes } from "../config/models";
import { Button } from "../components/ui/button";
import { Plus, Trash2 } from "lucide-react";
export function PricingConditions({
  rule,
  onChange,
  disabled,
}: {
  rule: Extract<PricingRule, { mode: "unit" }>;
  onChange: (rule: PricingRule) => void;
  disabled: boolean;
}) {
  const { text } = useSettingsText();
  const rates = rule.conditionRates ?? [];
  return (
    <details className="cm-callout" open={rates.length > 0}>
      <summary>
        {text(
          ...translatedMessage(
            "ui_conditional_rates_by_specification_optional__08fe7ce3",
          ),
        )}
        {rates.length} {text(...translatedMessage("ui_rules_816eaf10"))}
      </summary>
      <p>
        {text(
          ...translatedMessage(
            "ui_for_example_price_video_per_second_by_resolut_734bff4d",
          ),
        )}
      </p>
      {rates.map((rate, index) => {
        const patch = (data: Partial<typeof rate>) =>
          onChange({
            ...rule,
            conditionRates: rates.map((r, i) =>
              i === index ? { ...r, ...data } : r,
            ),
          });
        const condition = (key: keyof PricingAttributes, value: string) => {
          const when = { ...rate.when };
          delete when[key];
          if (value)
            (when as Record<string, unknown>)[key] =
              key === "audio" ? value === "true" : value;
          patch({ when });
        };
        return (
          <div className="cm-form cm-callout" key={rate.id}>
            <label>
              {text(...translatedMessage("ui_condition_name_e90d1f1c"))}
              <input
                aria-label={text(
                  ...translatedMessage("ui_condition_name__7e95696d", {
                    p0: index + 1,
                  }),
                )}
                disabled={disabled}
                value={rate.label}
                onChange={(e) => patch({ label: e.target.value })}
              />
            </label>
            <label>
              {text(
                ...translatedMessage("ui_conditional_unit_price_usd__81b96c91"),
              )}
              <input
                aria-label={text(
                  ...translatedMessage("ui_conditional_unit_price__65e56357", {
                    p0: index + 1,
                  }),
                )}
                disabled={disabled}
                value={rate.priceUsd}
                onChange={(e) => patch({ priceUsd: e.target.value })}
              />
            </label>
            <label>
              {text(...translatedMessage("ui_resolution_8b8e33f5"))}
              <input
                aria-label={text(
                  ...translatedMessage("ui_condition_resolution__df232e4d", {
                    p0: index + 1,
                  }),
                )}
                placeholder={text(
                  ...translatedMessage(
                    "ui_leave_blank_for_any_for_example_1080p_0eaaae79",
                  ),
                )}
                disabled={disabled}
                value={rate.when.resolution ?? ""}
                onChange={(e) => condition("resolution", e.target.value)}
              />
            </label>
            <label>
              {text(...translatedMessage("ui_quality_c8666570"))}
              <input
                aria-label={text(
                  ...translatedMessage("ui_condition_quality__93dca4b5", {
                    p0: index + 1,
                  }),
                )}
                placeholder={text(
                  ...translatedMessage(
                    "ui_leave_blank_for_any_for_example_hd_6fe1ed16",
                  ),
                )}
                disabled={disabled}
                value={rate.when.quality ?? ""}
                onChange={(e) => condition("quality", e.target.value)}
              />
            </label>
            <label>
              {text(...translatedMessage("ui_audio_included_8cbb1ed1"))}
              <select
                aria-label={text(
                  ...translatedMessage("ui_condition_audio__efbae662", {
                    p0: index + 1,
                  }),
                )}
                disabled={disabled}
                value={
                  rate.when.audio === undefined ? "" : String(rate.when.audio)
                }
                onChange={(e) => condition("audio", e.target.value)}
              >
                <option value="">
                  {text(...translatedMessage("ui_any_4e32a896"))}
                </option>
                <option value="true">
                  {text(...translatedMessage("ui_with_audio_7e7b7006"))}
                </option>
                <option value="false">
                  {text(...translatedMessage("ui_without_audio_6d52b383"))}
                </option>
              </select>
            </label>
            <Button
              disabled={disabled}
              onClick={() =>
                onChange({
                  ...rule,
                  conditionRates: rates.filter((r) => r.id !== rate.id),
                })
              }
            >
              <Trash2 size={14} />
              {text(...translatedMessage("ui_remove_condition_36102593"))}
            </Button>
          </div>
        );
      })}
      <Button
        disabled={disabled || rates.length >= 24}
        onClick={() =>
          onChange({
            ...rule,
            conditionRates: [
              ...rates,
              {
                id: crypto.randomUUID(),
                label: text(
                  ...translatedMessage("ui_hd_specification_194a280a"),
                ),
                when: { resolution: "1080p" },
                priceUsd: rule.priceUsd,
              },
            ],
          })
        }
      >
        <Plus size={14} />
        {text(...translatedMessage("ui_add_conditional_rate_81f5bc4f"))}
      </Button>
    </details>
  );
}
export function PricingConditionInputs({
  rule,
  attributes,
  onChange,
}: {
  rule: PricingRule;
  attributes: PricingAttributes;
  onChange: (attributes: PricingAttributes) => void;
}) {
  const { text } = useSettingsText();
  if (rule.mode !== "unit" || !rule.conditionRates?.length) return null;
  return (
    <div className="cm-form">
      {(["resolution", "quality", "audio"] as const)
        .filter((key) =>
          rule.conditionRates!.some((r) => r.when[key] !== undefined),
        )
        .map((key) => (
          <label key={key}>
            {text(...translatedMessage("ui_sample__3a03c335"))}
            {
              {
                resolution: text(
                  ...translatedMessage("ui_resolution_8b8e33f5"),
                ),
                quality: text(...translatedMessage("ui_quality_c8666570")),
                audio: text(...translatedMessage("audio_42f61562b3")),
              }[key]
            }
            {key === "audio" ? (
              <select
                aria-label={text(
                  ...translatedMessage("ui_sample_audio_5ef4b0da"),
                )}
                value={
                  attributes.audio === undefined ? "" : String(attributes.audio)
                }
                onChange={(e) =>
                  onChange({
                    ...attributes,
                    audio:
                      e.target.value === ""
                        ? undefined
                        : e.target.value === "true",
                  })
                }
              >
                <option value="">
                  {text(...translatedMessage("ui_select_an_option_764d36cb"))}
                </option>
                <option value="true">
                  {text(...translatedMessage("ui_with_audio_7e7b7006"))}
                </option>
                <option value="false">
                  {text(...translatedMessage("ui_without_audio_6d52b383"))}
                </option>
              </select>
            ) : (
              <input
                aria-label={text(
                  ...translatedMessage("ui_sample__8e20d1ad", {
                    p0:
                      key === "resolution"
                        ? text(...translatedMessage("ui_resolution_8b8e33f5"))
                        : text(...translatedMessage("ui_quality_c8666570")),
                  }),
                )}
                value={attributes[key] ?? ""}
                placeholder={
                  rule.conditionRates!.find((r) => r.when[key])?.when[key]
                }
                onChange={(e) =>
                  onChange({
                    ...attributes,
                    [key]: e.target.value || undefined,
                  })
                }
              />
            )}
          </label>
        ))}
    </div>
  );
}
