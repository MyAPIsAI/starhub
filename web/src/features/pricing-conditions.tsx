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
  const rates = rule.conditionRates ?? [];
  return (
    <details className="cm-callout" open={rates.length > 0}>
      <summary>按规格设置条件价（可选） · {rates.length} 条</summary>
      <p>
        例如视频按分辨率及是否含音频设置每秒价格，图片按质量设置每张价格。命中条件价后替换默认单位价，不叠加；未命中使用上方默认价。条件不得重叠。
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
              条件名称
              <input
                aria-label={`条件名称 ${index + 1}`}
                disabled={disabled}
                value={rate.label}
                onChange={(e) => patch({ label: e.target.value })}
              />
            </label>
            <label>
              条件单位价（美元）
              <input
                aria-label={`条件单位价 ${index + 1}`}
                disabled={disabled}
                value={rate.priceUsd}
                onChange={(e) => patch({ priceUsd: e.target.value })}
              />
            </label>
            <label>
              分辨率
              <input
                aria-label={`条件分辨率 ${index + 1}`}
                placeholder="留空为任意；例如 1080p"
                disabled={disabled}
                value={rate.when.resolution ?? ""}
                onChange={(e) => condition("resolution", e.target.value)}
              />
            </label>
            <label>
              质量
              <input
                aria-label={`条件质量 ${index + 1}`}
                placeholder="留空为任意；例如 hd"
                disabled={disabled}
                value={rate.when.quality ?? ""}
                onChange={(e) => condition("quality", e.target.value)}
              />
            </label>
            <label>
              是否含音频
              <select
                aria-label={`条件音频 ${index + 1}`}
                disabled={disabled}
                value={
                  rate.when.audio === undefined ? "" : String(rate.when.audio)
                }
                onChange={(e) => condition("audio", e.target.value)}
              >
                <option value="">任意</option>
                <option value="true">含音频</option>
                <option value="false">不含音频</option>
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
              删除条件
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
                label: "高清规格",
                when: { resolution: "1080p" },
                priceUsd: rule.priceUsd,
              },
            ],
          })
        }
      >
        <Plus size={14} />
        添加条件价
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
  if (rule.mode !== "unit" || !rule.conditionRates?.length) return null;
  return (
    <div className="cm-form">
      {(["resolution", "quality", "audio"] as const)
        .filter((key) =>
          rule.conditionRates!.some((r) => r.when[key] !== undefined),
        )
        .map((key) => (
          <label key={key}>
            模拟{{ resolution: "分辨率", quality: "质量", audio: "音频" }[key]}
            {key === "audio" ? (
              <select
                aria-label="模拟音频"
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
                <option value="">请选择</option>
                <option value="true">含音频</option>
                <option value="false">不含音频</option>
              </select>
            ) : (
              <input
                aria-label={`模拟${key === "resolution" ? "分辨率" : "质量"}`}
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
