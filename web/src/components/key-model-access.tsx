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
          {c("模型白名单", "Model allowlist", "قائمة السماح للنماذج")}
        </legend>
        <div className="key-restriction-heading">
          <label htmlFor={switchId}>
            <strong>
              {c("模型白名单", "Model allowlist", "قائمة السماح للنماذج")}
            </strong>
            <small>
              {c(
                "只允许选中的模型，智能路由遵守同一限制。",
                "Allow only selected models, including smart-routing candidates.",
                "السماح بالنماذج المحددة فقط، بما فيها مرشحو التوجيه.",
              )}
            </small>
          </label>
          <span className="key-restriction-control">
            <span>
              {access === "custom"
                ? c("已启用", "Enabled", "مفعلة")
                : c("已停用", "Disabled", "متوقفة")}
            </span>
            <Switch
              id={switchId}
              label={c(
                "启用模型白名单",
                "Enable model allowlist",
                "تفعيل قائمة النماذج",
              )}
              checked={access === "custom"}
              onChange={(enabled) => onAccessChange(enabled ? "custom" : "all")}
            />
          </span>
        </div>
        {access === "all" ? (
          <p className="key-policy-hint">
            {c(
              "当前允许分组内全部具体模型，智能路由入口权限保持不变。关闭仅暂停此限制，已选模型保留在表单草稿中。",
              "All group models are allowed; route-entry permissions stay unchanged. Turning this off preserves your selections in this form draft.",
              "كل نماذج المجموعة مسموحة وتبقى صلاحيات التوجيه دون تغيير. تحفظ الاختيارات في مسودة النموذج.",
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
                    `已授权 ${chosen.length} 个模型；仅选一个即为单模型限制。`,
                    `${chosen.length} models allowed. Selecting one restricts the key to that model.`,
                    `${chosen.length} نماذج مسموحة. اختيار نموذج واحد يقيد المفتاح به.`,
                  )
                : c(
                    "请至少选择一个当前分组可用的模型。",
                    "Select at least one available model.",
                    "اختر نموذجًا متاحًا واحدًا على الأقل.",
                  )}
            </p>
            {unavailable.length > 0 && (
              <p className="key-policy-hint is-warning" role="status">
                {c(
                  `有 ${unavailable.length} 个原选模型不在当前分组开放范围内，不会加入此密钥权限。`,
                  `${unavailable.length} previous selections are unavailable in this group and will not be authorized.`,
                  `${unavailable.length} نماذج محددة سابقًا غير متاحة ولن تُمنح صلاحية لها.`,
                )}
              </p>
            )}
          </>
        )}
        <div className="key-policy-summary">
          <Check size={14} />
          <span>
            {c(
              "智能路由同样受白名单约束。指定模型不会被静默替换，费用只按实际模型计算一次。",
              "Smart routing follows this allowlist. Explicit models are never substituted; the actual model is billed once.",
              "يلتزم التوجيه بقائمة السماح ولا تُستبدل النماذج المحددة. تحسب الرسوم مرة واحدة.",
            )}
          </span>
        </div>
        <Link to="/models" className="key-model-price-link">
          {c(
            "查看模型能力与分组价格",
            "Explore models and group prices",
            "استعرض النماذج وأسعار المجموعات",
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
            {c("高级设置", "Advanced settings", "إعدادات متقدمة")}
          </strong>
          <small>
            {c(
              "模型白名单与网络地址限制",
              "Model and IP allowlists",
              "قوائم النماذج وعناوين الشبكة",
            )}
          </small>
        </span>
        <span className="key-advanced-action">
          {open ? c("收起", "Collapse", "طي") : c("展开", "Expand", "توسيع")}
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
          <strong>
            {c("IP（网络地址）白名单", "IP allowlist", "قائمة عناوين الشبكة")}
          </strong>
          <small>
            {c(
              "只允许列表中的网络地址发起请求。",
              "Accept requests only from listed network addresses.",
              "قبول الطلبات من عناوين الشبكة المدرجة فقط.",
            )}
          </small>
        </label>
        <span className="key-restriction-control">
          <span>
            {enabled
              ? c("已启用", "Enabled", "مفعلة")
              : c("已停用", "Disabled", "متوقفة")}
          </span>
          <Switch
            id={`${id}-enabled`}
            label={c(
              "启用 IP（网络地址）白名单",
              "Enable IP allowlist",
              "تفعيل قائمة عناوين الشبكة",
            )}
            checked={enabled}
            onChange={onEnabledChange}
          />
        </span>
      </div>
      {enabled ? (
        <Field
          id={id}
          label={c("允许的网络地址", "Allowed addresses", "العناوين المسموحة")}
          hint={c(
            "每行一项，至少填写一项；支持单个地址或 CIDR（网段）。模拟请求来自 127.0.0.1（本机地址）。",
            "One per line, at least one required. Supports an IP address or CIDR range. Demo requests use 127.0.0.1.",
            "عنوان واحد على الأقل، كل عنوان في سطر. تدعم العناوين ونطاقات CIDR. مصدر المحاكاة 127.0.0.1.",
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
            "当前不限制来源地址。关闭仅暂停此限制，已填地址保留在表单草稿中。",
            "Source addresses are unrestricted. Turning this off preserves addresses in this form draft.",
            "عناوين المصدر غير مقيدة. تحفظ العناوين في مسودة النموذج.",
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
        {c(
          "通过 CC Switch（客户端配置工具）接入",
          "Connect through CC Switch",
          "الاتصال عبر CC Switch",
        )}
      </summary>
      <div>
        <p>
          {c(
            "将所需模型纳入授权范围。客户端发送的 model（模型标识）须与模型列表一致。",
            "Allow the models you need. Send the exact model ID listed in the catalog.",
            "اختر وضع العميل واسمح بالنماذج المطلوبة. أرسل معرّف النموذج المطابق للفهرس.",
          )}
        </p>
        <p>
          {c(
            "Claude Code（编程客户端）的 /model（切换模型命令）可切换模型；若使用 Sonnet / Opus / Haiku（模型档位别名），还需在客户端配置相应的实际模型映射。",
            "Claude Code can switch using /model. Sonnet, Opus and Haiku aliases also require mappings to actual model IDs in the client configuration.",
            "يتيح Claude Code التبديل عبر /model. تتطلب الأسماء Sonnet وOpus وHaiku تعيينًا لمعرّفات نماذج فعلية في إعدادات العميل.",
          )}
        </p>
        <p>
          {c(
            "授权多模型不代表协议通用：还需匹配客户端所用接口协议。本项目展示权限与计费逻辑，未接入真实网关。",
            "Multi-model permission does not provide protocol compatibility. Match the client's API protocol. This demo has no live gateway.",
            "السماح بعدة نماذج لا يعني توافق البروتوكولات. طابق بروتوكول العميل. لا يتصل هذا العرض ببوابة فعلية.",
          )}
        </p>
      </div>
    </details>
  );
}
