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
import { useSettingsText } from "./pricing-settings-shared";
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
      text("官方普通输入", "Official input", "إدخال رسمي"),
    ],
    [
      "output_per_million_usd",
      text("官方模型输出", "Official output", "إخراج رسمي"),
    ],
    [
      "cache_read_per_million_usd",
      text("官方缓存读取", "Official cache read", "قراءة ذاكرة رسمية"),
    ],
    [
      "cache_write_per_million_usd",
      text("官方缓存写入", "Official cache write", "كتابة ذاكرة رسمية"),
    ],
  ];
  const error = reference ? officialReferenceError(reference) : null;
  const errors = {
    shape: text(
      "请检查币种、报价状态和计价方式。",
      "Check currency, reference status and billing unit.",
      "تحقق من العملة وحالة المرجع ووحدة التسعير.",
    ),
    source: text(
      "请填写有效的 https（安全链接）官方来源地址。",
      "Enter a valid HTTPS official source URL.",
      "أدخل رابط HTTPS صالحا للمصدر الرسمي.",
    ),
    date: text(
      "请填写有效核验日期，不能晚于今天。",
      "Enter a valid verification date no later than today.",
      "أدخل تاريخ تحقق صالحا لا يتجاوز اليوم.",
    ),
    scope: text(
      "请填写适用条件，最多 1,200 字。",
      "Enter the scope, up to 1,200 characters.",
      "أدخل نطاق التطبيق حتى 1,200 حرف.",
    ),
    price: text(
      "请填写非负报价，最多 6 位小数；未单独报价的缓存项留空。",
      "Enter non-negative prices with up to 6 decimals; leave unquoted cache items blank.",
      "أدخل أسعارا غير سالبة حتى 6 منازل عشرية واترك بنود الذاكرة غير المسعرة فارغة.",
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
          {text(
            "厂商官方参考价",
            "Official provider reference",
            "مرجع المورد الرسمي",
          )}
          <small>
            {reference
              ? reference.status === "legacy"
                ? text("历史参考", "Historical reference", "مرجع تاريخي")
                : text("已配置", "Configured", "محدد")
              : text(
                  "未核实 / 未展示",
                  "Unverified / hidden",
                  "غير موثق / مخفي",
                )}
          </small>
        </span>
        <ChevronDown size={16} />
      </summary>
      <div className="official-reference-body">
        <div className="official-reference-toggle">
          <div>
            <strong>
              {text(
                "在模型广场展示官方参考价",
                "Show the official reference in the catalog",
                "اعرض المرجع الرسمي في الكتالوج",
              )}
            </strong>
            <p>
              {text(
                "与上方平台计费基准分开维护；不乘分组倍率，不参与扣费。",
                "Maintained separately from platform billing; no group multiplier and no effect on charges.",
                "يدار منفصلا عن فوترة المنصة دون معامل مجموعة أو أثر على الرسوم.",
              )}
            </p>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={Boolean(reference)}
            aria-label={text(
              "展示官方参考价",
              "Show official reference",
              "عرض المرجع الرسمي",
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
                "官方参考价配置",
                "Official reference settings",
                "إعدادات المرجع الرسمي",
              )}
            </legend>
            <div className="settings-fields">
              <label>
                <span>
                  {text("报价状态", "Reference status", "حالة المرجع")}
                </span>
                <select
                  aria-label={text(
                    "报价状态",
                    "Reference status",
                    "حالة المرجع",
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
                      "核验时在售参考",
                      "Available at verification",
                      "متاح عند التحقق",
                    )}
                  </option>
                  <option value="legacy">
                    {text(
                      "历史参考 / 已退役",
                      "Historical / retired",
                      "تاريخي / متقاعد",
                    )}
                  </option>
                </select>
              </label>
              <label>
                <span>
                  {text(
                    "官方计价单位",
                    "Official pricing unit",
                    "وحدة السعر الرسمي",
                  )}
                </span>
                <select
                  aria-label={text(
                    "官方计价单位",
                    "Official pricing unit",
                    "وحدة السعر الرسمي",
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
                      "美元 / 百万词元",
                      "USD / million tokens",
                      "دولار / مليون رمز",
                    )}
                  </option>
                  <option value="image">
                    {text("美元 / 张", "USD / image", "دولار / صورة")}
                  </option>
                  <option value="second">
                    {text("美元 / 秒", "USD / second", "دولار / ثانية")}
                  </option>
                  <option value="request">
                    {text("美元 / 次", "USD / request", "دولار / طلب")}
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
                                "不适用 / 未单独报价",
                                "Not applicable / unquoted",
                                "غير منطبق / غير مسعر",
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
                      "官方每单位价格（美元）",
                      "Official unit price (USD)",
                      "السعر الرسمي للوحدة بالدولار",
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
                    "官方参考来源地址",
                    "Official reference source URL",
                    "رابط المصدر المرجعي الرسمي",
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
                    "官方参考核验日期",
                    "Official reference verification date",
                    "تاريخ تحقق المرجع الرسمي",
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
                  "适用条件（修改内容按原文展示）",
                  "Scope (edits are displayed as entered)",
                  "النطاق (تعرض التعديلات كما أدخلت)",
                )}
              </span>
              <textarea
                rows={3}
                maxLength={1200}
                value={reference.scope[lang]}
                placeholder={text(
                  "例如：标准付费、输入≤200,000词元、5分钟缓存、指定图片分辨率；写清排除项。",
                  "For example: standard paid tier, prompts ≤200,000 tokens, 5-minute cache or image resolution. State exclusions.",
                  "مثال: فئة مدفوعة قياسية ومدخلات حتى 200,000 رمز وذاكرة 5 دقائق أو دقة الصورة. اذكر الاستثناءات.",
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
                "只展示已核验的一种明确口径；不同上下文、地区、缓存时长和媒体规格不能混比。复杂阶梯价请写明当前范围并附官方来源，不使用表达式推算官价。",
                "Show one verified scope. Do not mix context lengths, regions, cache durations or media specifications. For complex tiers, state the displayed scope and link the source; do not derive official rates from expressions.",
                "اعرض نطاقا موثقا واحدا ولا تخلط أطوال السياق أو المناطق أو مدد الذاكرة أو مواصفات الوسائط. وضح نطاق الشرائح المعقدة واربط المصدر ولا تستنتج الأسعار من تعبيرات.",
              )}
            </p>
            {!error && (
              <a
                className="official-reference-source"
                href={reference.sourceUrl}
                target="_blank"
                rel="noreferrer"
              >
                {text(
                  "核对官方来源",
                  "Check official source",
                  "تحقق من المصدر الرسمي",
                )}
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
              "未配置时显示“未核实”，平台基准价不会被当成官价。关闭展示会随草稿发布保存，不会在刷新后自动恢复。",
              "When unconfigured, the catalog says unverified; platform base prices are never treated as official. A published removal survives refresh.",
              "عند غياب الإعداد يعرض غير موثق ولا تعتبر أسعار المنصة رسمية. يبقى الإخفاء المنشور بعد التحديث.",
            )}
          </p>
        )}
      </div>
    </details>
  );
}
