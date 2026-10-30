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
    ? text("按词元", "Per token", "لكل رمز")
    : rule.mode === "unit"
      ? {
          request: text("按次", "Per request", "لكل طلب"),
          image: text("按张", "Per image", "لكل صورة"),
          second: text("按秒", "Per second", "لكل ثانية"),
        }[rule.unit]
      : text("表达式", "Expression", "تعبير");

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
          "官方参考报价需要填写来源地址和核验日期。",
          "An official reference needs a source URL and verification date.",
          "يتطلب المرجع الرسمي رابط المصدر وتاريخ التحقق.",
        )
      : [entry.verifiedAt, entry.effectiveAt].some(
            (date) => date && Date.parse(date) > Date.now(),
          )
        ? text(
            "核验日期与来源生效日期不能晚于今天。",
            "Verification and reference dates cannot be in the future.",
            "لا يمكن أن تكون تواريخ التحقق والمرجع في المستقبل.",
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
      "普通输入",
      "Uncached input",
      "الإدخال غير المخزن",
    ),
    output_per_million_usd: text("模型输出", "Output", "الإخراج"),
    cache_read_per_million_usd: text(
      "缓存读取（可选）",
      "Cache read (optional)",
      "قراءة الذاكرة (اختياري)",
    ),
    cache_write_per_million_usd: text(
      "缓存写入（可选）",
      "Cache write (optional)",
      "كتابة الذاكرة (اختياري)",
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
          "请填写有效时间后试算。",
          "Choose an eligible key and a valid time to quote.",
          "اختر مفتاحا متاحا ووقتا صالحا للحساب.",
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
                "无法试算，请检查规则和用量。",
                "Unable to quote. Check the rule and usage.",
                "تعذر الحساب. تحقق من القاعدة والاستخدام.",
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
            return text("无法报价", "Not quotable", "تعذر التسعير");
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
    input_tokens: text("普通输入词元", "Input tokens", "رموز الإدخال"),
    output_tokens: text("输出词元", "Output tokens", "رموز الإخراج"),
    cache_read_tokens: text(
      "缓存读取词元",
      "Cache read tokens",
      "رموز قراءة الذاكرة",
    ),
    cache_write_tokens: text(
      "缓存写入词元",
      "Cache write tokens",
      "رموز كتابة الذاكرة",
    ),
    images: text("图片张数", "Images", "الصور"),
    seconds: text("时长（秒）", "Duration (seconds)", "المدة بالثواني"),
    requests: text("请求次数", "Requests", "الطلبات"),
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
              `已提交模拟用量并扣除 ${formatMoney(record.cost_nano, "call")}。未生成真实模型内容。`,
              `Simulated usage submitted; ${formatMoney(record.cost_nano, "call")} charged. No real model output was generated.`,
              `تم إرسال الاستخدام المحاكى وخصم ${formatMoney(record.cost_nano, "call")}. لم ينتج محتوى حقيقي.`,
            )
          : `${record.error_message ?? text("模拟调用失败。", "Simulated call failed.", "فشل الطلب المحاكى.")} ${record.recovery_action ?? ""} ${text("未计费；当前输入已保留。", "Not charged; your inputs are retained.", "لم تحتسب رسوم واحتفظ بالمدخلات.")}`,
    });
  };
  return (
    <div className="pricing-admin settings-pricing">
      <div className="pricing-page-heading" hidden={embedded}>
        <div>
          <span className="pricing-eyebrow">
            <CircleDollarSign size={15} />
            {text("系统设置", "System settings", "إعدادات النظام")}
          </span>
          <h1>{text("模型定价", "Model pricing", "تسعير النماذج")}</h1>
          <p>
            {text(
              "维护此浏览器的演示定价与分时规则，确认前查看模拟影响。服务器完整定价管理仍待接入。",
              "Manage this browser’s Demo pricing and time rules, reviewing simulated impact before saving. Complete server pricing management is still pending.",
              "أدر التسعير التجريبي وقواعد الوقت في هذا المتصفح وراجع أثر المحاكاة قبل الحفظ. لا تزال إدارة تسعير الخادم الكاملة قيد التنفيذ.",
            )}
          </p>
        </div>
        <Button asChild variant="secondary">
          <Link to="/console/service-groups">
            {text(
              "管理服务分组",
              "Manage service groups",
              "إدارة مجموعات الخدمة",
            )}
            <ArrowUpRight size={15} />
          </Link>
        </Button>
      </div>
      <div className="pricing-policy">
        <Info size={18} />
        <div>
          <strong>
            {text(
              "参考报价有来源，历史费用有快照",
              "Reference sources and immutable historical rates",
              "مصادر مرجعية وأسعار تاريخية محفوظة",
            )}
          </strong>
          <p>
            {text(
              "此处规则保存在本浏览器，仅用于演示模拟，不发布到服务器计费。分组管理和下方服务器配置历史使用独立的真实接口；厂商官方参考价也单独维护。",
              "Rules here are stored in this browser for Demo simulations and are not published to server billing. Group management and the server configuration history below use separate real APIs; official provider references are managed separately.",
              "تُحفظ هذه القواعد في المتصفح للمحاكاة التجريبية ولا تُنشر لفوترة الخادم. تستخدم إدارة المجموعات وسجل إعدادات الخادم أدناه واجهات فعلية مستقلة، وتُدار المراجع الرسمية منفصلة.",
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
            "配置已有新版本。当前草稿仍保留，需放弃后重新修改，避免覆盖其他已发布设置。",
            "A newer configuration exists. Discard this retained draft before editing to avoid overwriting published settings.",
            "يوجد إصدار أحدث. تجاهل المسودة المحفوظة قبل التعديل لتجنب الكتابة فوق الإعدادات المنشورة.",
          )}
        </div>
      )}
      <section className="pricing-panel">
        <div
          hidden={embedded}
          className="pricing-model-tabs"
          aria-label={text(
            "选择计费模型",
            "Choose billing model",
            "اختر نموذج الفوترة",
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
            {text("演示参考价格", "Demo reference price", "سعر مرجعي تجريبي")} ·{" "}
            {modeName(rule, text)}
          </span>
        </div>
        <div className="settings-fields settings-source-fields">
          <label>
            <span>{text("基准来源", "Reference source", "مصدر المرجع")}</span>
            <select
              aria-label={text("基准来源", "Reference source", "مصدر المرجع")}
              value={entry.source}
              disabled={!canEdit}
              onChange={(event) =>
                setEntry({ source: event.target.value as typeof entry.source })
              }
            >
              <option value="sample">
                {text(
                  "演示样例 · 未核实",
                  "Demo sample · unverified",
                  "عينة تجريبية · غير موثقة",
                )}
              </option>
              <option value="manual-official">
                {text(
                  "参考官方报价的基准 · 人工录入",
                  "Base informed by official rates · manual",
                  "أساس يسترشد بالسعر الرسمي · يدوي",
                )}
              </option>
            </select>
          </label>
          <label>
            <span>
              {text(
                "官方来源地址",
                "Official source URL",
                "رابط المصدر الرسمي",
              )}
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
            <span>{text("核验日期", "Verification date", "تاريخ التحقق")}</span>
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
                "来源价格生效日期",
                "Reference effective date",
                "تاريخ سريان المرجع",
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
            "计价版本（发布自动生成）：",
            "Pricing version (generated on publish): ",
            "إصدار التسعير (ينشأ عند النشر): ",
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
            "适用日期记录供应商报价口径，发布后立即用于新演示调用；不代表定时发布。美元是本演示的统一结算币种。",
            "The effective date documents the reference; publishing applies immediately to new demo calls, not on a schedule. USD is this demo’s settlement currency.",
            "يوثق التاريخ سريان المرجع؛ يطبق النشر فورا على الطلبات الجديدة وليس في موعد مجدول. الدولار عملة التسوية التجريبية.",
          )}
        </p>
        <div className="settings-editor-grid">
          <div>
            <div className="settings-block-heading">
              <h3>{text("计费方式", "Billing method", "طريقة الفوترة")}</h3>
              <span>{text("基础规则", "Base rule", "القاعدة الأساسية")}</span>
            </div>
            <div
              className="settings-segment"
              aria-label={text("计费方式", "Billing method", "طريقة الفوترة")}
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
                    ? text("按词元", "Tokens", "الرموز")
                    : mode === "unit"
                      ? text("按单位", "Units", "الوحدات")
                      : text(
                          "表达式 · 高级",
                          "Expression · advanced",
                          "تعبير · متقدم",
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
                                ? text("未配置", "Not configured", "غير محدد")
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
                    "单位：美元 / 百万词元。四类用量独立计费；0 表示免费，缓存留空表示未配置。支持最多 6 位小数。",
                    "Unit: USD / million tokens. Buckets bill separately. Zero is free; empty cache rates are unconfigured. Up to 6 decimals.",
                    "الوحدة: دولار لكل مليون رمز. كل فئة مستقلة. الصفر مجاني وأسعار الذاكرة الفارغة غير محددة. حتى 6 منازل عشرية.",
                  )}
                </p>
              </>
            )}
            {rule.mode === "unit" && (
              <>
                <div className="settings-fields">
                  <label>
                    <span>
                      {text("计量单位", "Metering unit", "وحدة القياس")}
                    </span>
                    <select
                      aria-label={text(
                        "计量单位",
                        "Metering unit",
                        "وحدة القياس",
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
                        {text("次（请求）", "Request", "طلب")}
                      </option>
                      <option value="image">
                        {text("张（图片）", "Image", "صورة")}
                      </option>
                      <option value="second">
                        {text("秒（时长）", "Second", "ثانية")}
                      </option>
                    </select>
                  </label>
                  {(["priceUsd", "minimum", "ceilTo"] as const).map((field) => (
                    <label key={field}>
                      <span>
                        {field === "priceUsd"
                          ? text(
                              "每单位价格（美元）",
                              "Price per unit (USD)",
                              "سعر الوحدة بالدولار",
                            )
                          : field === "minimum"
                            ? text(
                                "最低计费数量",
                                "Minimum billed quantity",
                                "الحد الأدنى للكمية",
                              )
                            : text(
                                "向上取整步长",
                                "Rounding increment",
                                "خطوة التقريب للأعلى",
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
                    "按所选单位计价，不换算为百万词元。实际用量先与最低数量取较大值，再按步长向上取整。",
                    "Bills the selected unit, not tokens. Apply the minimum quantity, then round up to the increment.",
                    "تُحسب الوحدة المحددة لا الرموز. يطبق الحد الأدنى أولا ثم تقرب الكمية للأعلى بالخطوة.",
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
                      "计费表达式（结果单位：美元）",
                      "Billing expression (result: USD)",
                      "تعبير الفوترة (الناتج بالدولار)",
                    )}
                  </span>
                  <textarea
                    aria-label={text(
                      "计费表达式（结果单位：美元）",
                      "Billing expression (result: USD)",
                      "تعبير الفوترة (الناتج بالدولار)",
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
                    "仅支持四则运算、括号、min（最小值）、max（最大值）、ceil（向上取整）。不执行脚本；除零、负费用、缺少变量均拒绝计费。",
                    "Supports arithmetic, parentheses, min, max and ceil only. No scripts; division by zero, negative cost and missing variables are rejected.",
                    "يدعم العمليات والأقواس وmin وmax وceil فقط. لا ينفذ نصوصا؛ ترفض القسمة على صفر والرسوم السالبة والمتغيرات الناقصة.",
                  )}
                </p>
                <details className="settings-details">
                  <summary>
                    {text(
                      "查看变量与单位",
                      "Variables and units",
                      "المتغيرات والوحدات",
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
                      "表达式先产生基准费用，再由系统应用分时与所属服务分组规则；勿在表达式内重复叠加。",
                      "The expression returns the base charge; time and service group rules are then applied by the system. Do not duplicate them in the formula.",
                      "يعيد التعبير الرسوم الأساسية ثم يطبق النظام قواعد الوقت والمجموعة. لا تكررها داخل الصيغة.",
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
                {text("报价试算", "Quote simulator", "محاكي التسعير")}
              </h3>
              <span>
                {dirty
                  ? text("使用草稿", "Using draft", "باستخدام المسودة")
                  : text("已发布规则", "Published rules", "القواعد المنشورة")}
              </span>
            </div>
            <label>
              <span>
                {text("用于试算的密钥", "Key for quotation", "مفتاح الحساب")}
              </span>
              <select
                aria-label={text(
                  "用于试算的密钥",
                  "Key for quotation",
                  "مفتاح الحساب",
                )}
                value={key?.id ?? ""}
                onChange={(event) => {
                  setSelectedKey(event.target.value);
                  setResult(null);
                }}
              >
                <option value="" disabled>
                  {text(
                    "请选择可用密钥",
                    "Choose an eligible key",
                    "اختر مفتاحا متاحا",
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
                  "当前只试算模型基准费用，不含分组倍率、不能入账。需要结算验证时，请先映射投放资源并创建兼容密钥。",
                  "No enabled key is authorized for this model. Map and place a resource in Channels or adjust key permissions.",
                  "لا يوجد مفتاح نشط مخول لهذا النموذج. أتحه في المجموعات أو عدل صلاحية المفتاح.",
                )}{" "}
                <Link to="/console/keys">
                  {text("管理密钥", "Manage keys", "إدارة المفاتيح")}
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
                  "计价模拟时间（本地时区）",
                  "Pricing simulation time (local timezone)",
                  "وقت محاكاة التسعير (التوقيت المحلي)",
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
                  key ? "预估演示费用" : "模型基准费用（未含分组倍率）",
                  "Estimated demo charge",
                  "التكلفة التجريبية المقدرة",
                )}
              </span>
              <strong>{quote.amount}</strong>
            </div>
            {quote.quoted && (
              <small>
                {quote.quoted.snapshot.matchedWindow
                  ? text(
                      `命中时段：${quote.quoted.snapshot.matchedWindow.label}（${quote.quoted.snapshot.timeMultiplier}×）`,
                      `Time window: ${quote.quoted.snapshot.matchedWindow.label} (${quote.quoted.snapshot.timeMultiplier}×)`,
                      `الفترة: ${quote.quoted.snapshot.matchedWindow.label} (${quote.quoted.snapshot.timeMultiplier}×)`,
                    )
                  : text(
                      "未命中分时时段，使用基础规则。",
                      "No time window matched; base rules apply.",
                      "لا توجد فترة مطابقة؛ تطبق القواعد الأساسية.",
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
                `当前演示余额 ${formatMoney(balance, "balance")}。试算不会扣款；提交后的记录时间和额度周期仍按当前时间。`,
                `Demo balance ${formatMoney(balance, "balance")}. Quoting does not charge. Submitted records and quota periods use the current real time.`,
                `الرصيد التجريبي ${formatMoney(balance, "balance")}. الحساب لا يخصم الرسوم. وقت السجلات ودورات الحصة هو الوقت الحالي.`,
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
                "提交模拟用量并计费",
                "Submit simulated usage & bill",
                "إرسال استخدام محاكى واحتساب الرسوم",
              )}
            </Button>
            {dirty && (
              <small>
                {text(
                  "先发布规则，再按已发布价格提交用量。",
                  "Publish first to submit usage at the published price.",
                  "انشر أولا لإرسال الاستخدام بالسعر المنشور.",
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
                  {text(
                    "查看关联调用明细",
                    "View linked request",
                    "عرض تفاصيل الطلب المرتبط",
                  )}
                </Link>
              </div>
            )}
            <small>
              {text(
                "此操作只提交人工输入的模拟用量并联动演示余额，不调用图片、视频或其他真实服务。",
                "This bills manually entered simulated usage against the demo balance. No image, video or other real service is called.",
                "يحتسب الاستخدام المدخل يدويا على الرصيد التجريبي. لا يستدعي خدمات صور أو فيديو أو خدمات حقيقية.",
              )}
            </small>
          </aside>
        </div>
        <div className="settings-time-section">
          <div className="settings-block-heading">
            <h3>
              <Clock3 size={16} />
              {text("分时计价", "Time-based pricing", "تسعير حسب الوقت")}
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
                        "低峰时段",
                        "Off-peak window",
                        "وقت خارج الذروة",
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
              {text("添加时段", "Add time window", "إضافة فترة")}
            </Button>
          </div>
          <p className="pricing-help">
            {text(
              "分时规则可以叠加到任一种计费方式。未命中时使用原价；时间窗必须不重叠。跨午夜时段归属于开始日，结束时刻不包含在内。",
              "Time rules apply to any billing mode. Outside windows the base rate applies. Windows must not overlap; overnight windows belong to their start day and exclude the end time.",
              "تطبق قواعد الوقت على كل الطرق. خارج الفترات يطبق السعر الأساسي. لا تتداخل الفترات؛ تنسب العابرة لمنتصف الليل ليوم البدء ولا تشمل النهاية.",
            )}
          </p>
          {windows.length === 0 && (
            <div className="settings-empty">
              {text(
                "未设置分时规则 · 全天使用同一基准",
                "No time rules · same base price all day",
                "لا توجد قواعد وقت · سعر أساسي موحد طوال اليوم",
              )}
            </div>
          )}
          {windows.map((window, index) => (
            <div key={window.id} className="settings-window">
              <div className="settings-fields settings-window-fields">
                <label>
                  <span>{text("时段名称", "Window name", "اسم الفترة")}</span>
                  <input
                    aria-label={`${text("时段名称", "Window name", "اسم الفترة")} ${index + 1}`}
                    value={window.label}
                    disabled={!canEdit}
                    onChange={(event) =>
                      updateWindow(window.id, { label: event.target.value })
                    }
                  />
                </label>
                <label>
                  <span>{text("时区", "Timezone", "المنطقة الزمنية")}</span>
                  <select
                    aria-label={text("时区", "Timezone", "المنطقة الزمنية")}
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
                  <span>{text("开始时刻", "Start time", "وقت البدء")}</span>
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
                  <span>{text("结束时刻", "End time", "وقت الانتهاء")}</span>
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
                    {text("时段系数", "Time coefficient", "معامل الفترة")}
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
                    为此时段单独设置词元价格
                  </label>
                  <p className="pricing-help">
                    适用于输入、输出或缓存折扣比例不同的高低峰价格。未覆盖的价格沿用常规基准；使用独立价格时不叠加时段系数。
                  </p>
                  {window.tokenPrices && (
                    <div className="settings-fields">
                      {(Object.keys(labels) as (keyof ModelPricing)[]).map(
                        (field) => (
                          <label key={field}>
                            {labels[field]}（美元 / 百万词元）
                            <input
                              aria-label={`${window.label} ${labels[field]}`}
                              inputMode="decimal"
                              disabled={!canEdit}
                              value={window.tokenPrices![field] ?? ""}
                              placeholder="留空沿用常规价"
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
                    "适用星期",
                    "Applicable days",
                    "الأيام المعنية",
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
                      {lang === "zh"
                        ? ["日", "一", "二", "三", "四", "五", "六"][day]
                        : lang === "ar"
                          ? [
                              "أحد",
                              "اثنين",
                              "ثلاثاء",
                              "أربعاء",
                              "خميس",
                              "جمعة",
                              "سبت",
                            ][day]
                          : ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"][
                              day
                            ]}
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
                  {text("删除时段", "Remove window", "حذف الفترة")}
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
                  `${changedModels.length} 个模型有待发布修改`,
                  `${changedModels.length} models have unpublished changes`,
                  `${changedModels.length} نماذج بتغييرات غير منشورة`,
                )
              : text(
                  "浏览器演示定价已保存",
                  "Browser Demo pricing saved",
                  "حُفظ التسعير التجريبي في المتصفح",
                )}
          </strong>
          <small>
            {text(
              "切换设置模块可恢复草稿；发布前检查影响。",
              "Drafts survive switching settings modules; review impact before publishing.",
              "تبقى المسودات عند تبديل وحدات الإعداد؛ راجع الأثر قبل النشر.",
            )}
          </small>
          {!valid && (
            <small className="settings-error">
              {text(
                "有无效规则或来源信息，请修正后发布。",
                "Correct invalid rules or reference metadata before publishing.",
                "صحح القواعد أو بيانات المصدر غير الصالحة قبل النشر.",
              )}
            </small>
          )}
        </div>
        <div>
          <Button disabled={!dirty} onClick={discard}>
            {text("放弃修改", "Discard changes", "تجاهل التغييرات")}
          </Button>
          <Button
            variant="primary"
            disabled={!canEdit || !dirty || !valid || conflict}
            onClick={() => setReviewOpen(true)}
          >
            {text("检查并发布", "Review & publish", "مراجعة ونشر")}
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
                  "服务器配置版本（独立于演示）",
                  "Server config version (separate from Demo)",
                  "إصدار إعدادات الخادم (مستقل عن العرض)",
                )}
              </strong>
              {optionStatusQuery.data && (
                <span
                  style={{ marginInlineStart: 8, fontSize: "var(--text-sm)" }}
                >
                  v{optionStatusQuery.data.database_version}
                  {optionStatusQuery.data.in_sync
                    ? text(
                        " · 响应进程已加载",
                        " · loaded by response process",
                        " · محمّل في العملية المستجيبة",
                      )
                    : text(
                        " · 响应进程版本滞后",
                        " · response process behind",
                        " · العملية المستجيبة متأخرة",
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
              {text(
                "服务器历史与回滚",
                "Server history & rollback",
                "سجل الخادم والتراجع",
              )}
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
          "保存浏览器演示定价",
          "Save browser Demo pricing",
          "حفظ التسعير التجريبي في المتصفح",
        )}
        description={text(
          "仅影响此浏览器的后续模拟。不会更新服务器模型倍率、分组或真实密钥权限。",
          "Affects future simulations in this browser. Server model ratios, groups and actual key permissions are unchanged.",
          "يؤثر في المحاكاة القادمة بهذا المتصفح. لا تتغير معاملات نماذج الخادم أو المجموعات أو صلاحيات المفاتيح الفعلية.",
        )}
        size="wide"
        preventOutsideClose
        footer={
          <>
            <Button onClick={() => setReviewOpen(false)}>
              {text("继续编辑", "Continue editing", "متابعة التعديل")}
            </Button>
            <Button
              variant="primary"
              disabled={!canEdit || !valid || conflict}
              onClick={publish}
            >
              <Check size={16} />
              {text("确认发布", "Publish now", "تأكيد النشر")}
            </Button>
          </>
        }
      >
        <div className="settings-review-summary">
          <strong>
            {text(
              `${changedModels.length} 个模型 · ${affectedKeys.length} 把启用密钥受影响`,
              `${changedModels.length} models · ${affectedKeys.length} active keys affected`,
              `${changedModels.length} نماذج · ${affectedKeys.length} مفاتيح نشطة متأثرة`,
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
              `${billingChangedModels.length} 个模型的计费规则变化。官方参考价只影响广场展示，不参与费用计算。`,
              `${billingChangedModels.length} models have billing changes. Official references affect catalog display only, never charges.`,
              `${billingChangedModels.length} نماذج بتغييرات فوترة. المراجع الرسمية للعرض فقط ولا تحتسب في الرسوم.`,
            )}
          </p>
        </div>
        <p className="pricing-help">
          {text(
            "统一示例用量：1,000 输入词元、500 输出词元、无缓存、1 张图、5 秒、1 次请求。仅取规则实际使用的变量，计费时间沿用试算面板。",
            "Example usage: 1,000 input, 500 output tokens, no cache, 1 image, 5 seconds, 1 request. Only relevant variables are used; time follows the simulator.",
            "الاستخدام المثال: 1000 إدخال و500 إخراج بلا ذاكرة وصورة واحدة و5 ثوان وطلب واحد. تستخدم المتغيرات المعنية فقط ووقت المحاكي.",
          )}
        </p>
        <div className="pricing-table-scroll">
          <table>
            <thead>
              <tr>
                <th>{text("模型", "Model", "النموذج")}</th>
                <th>{text("受影响密钥", "Affected key", "المفتاح المتأثر")}</th>
                <th>
                  {text(
                    "示例费用：当前 → 发布后",
                    "Example charge: current → published",
                    "التكلفة: الحالية ← المنشورة",
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
                        示例规格：{JSON.stringify(row.attributes)}
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
                      "暂无启用密钥受影响，后续获授权的请求仍使用新规则。",
                      "No active keys affected; future authorized requests will use the new rules.",
                      "لا تتأثر مفاتيح نشطة؛ تستخدم الطلبات المخولة القادمة القواعد الجديدة.",
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
          "服务器配置变更历史",
          "Server config change history",
          "سجل تغييرات إعدادات الخادم",
        )}
        description={text(
          "此处操作真实服务器配置，与浏览器演示定价独立。回滚恢复所选变更前的配置值，并校验当前服务器版本。",
          "These actions change real server configuration independently of browser Demo pricing. Rollback restores the selected pre-change value and checks the current server version.",
          "تغيّر هذه الإجراءات إعدادات الخادم الفعلية مستقلة عن التسعير التجريبي. يستعيد التراجع القيمة السابقة المحددة ويتحقق من إصدار الخادم الحالي.",
        )}
        size="wide"
      >
        {optionHistoryQuery.isLoading ? (
          <p style={{ color: "var(--muted)" }}>
            {text("加载中…", "Loading…", "جارٍ التحميل…")}
          </p>
        ) : (
          <table className="pricing-table-scroll">
            <thead>
              <tr>
                <th>{text("版本", "Version", "الإصدار")}</th>
                <th>{text("键", "Key", "المفتاح")}</th>
                <th>{text("旧值", "Old value", "القيمة القديمة")}</th>
                <th>{text("新值", "New value", "القيمة الجديدة")}</th>
                <th>{text("时间", "Time", "الوقت")}</th>
                <th>{text("操作", "Action", "الإجراء")}</th>
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
                                  "已回滚到此版本。",
                                  "Rolled back.",
                                  "تم التراجع.",
                                ),
                              ),
                            onError: (err) =>
                              setRollbackNotice(
                                err instanceof ApiError
                                  ? err.message
                                  : text(
                                      "回滚失败",
                                      "Rollback failed",
                                      "فشل التراجع",
                                    ),
                              ),
                          },
                        );
                      }}
                    >
                      {text("回滚到此版本", "Rollback", "تراجع")}
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
                      {text("暂无变更记录", "No changes", "لا تغييرات")}
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
