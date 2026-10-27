import { visibleGroups } from "../lib/platform-catalog";
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { CircleCheck, Copy, ArrowRight } from "lucide-react";
import { Modal } from "../components/ui/overlay";
import { Button } from "../components/ui/button";
import { Field, Input } from "../components/ui/primitives";
import {
  TierModelPicker,
  getTierModels,
} from "../components/tier-model-picker";
import {
  KeyModelAccess,
  KeyAdvancedSettings,
  KeyIpAllowlist,
} from "../components/key-model-access";
import { useModelPricing } from "../lib/model-pricing";
import { useProductPreview, type PreviewKey } from "../lib/product-preview";
import { getKeyModels, getKeyModelOptions } from "../lib/key-model-policy";
import { type TierId } from "../config/placeholders";
import { copyText } from "./code-example";
import { useCreateToken } from "../lib/api/tokens";
import type { TokenCreateInput } from "../lib/api/tokens";
import { ApiError } from "../lib/api/http";
import {
  tierToGroup,
  usdToQuota,
  joinList,
  tokenToPreviewKey,
  useBackendKeys,
} from "../lib/api/key-adapter";

const draftKey = "veyra-key-draft-v2";
function readDraft() {
  try {
    return JSON.parse(sessionStorage.getItem(draftKey) || "{}");
  } catch {
    return {};
  }
}
export default function CreateKey({
  open,
  setOpen,
  initialModel,
  initialGroup,
  onCreated,
}: {
  open: boolean;
  setOpen: (v: boolean) => void;
  initialModel?: string;
  initialGroup?: TierId;
  onCreated?: (keyId: string, plaintext: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const navigate = useNavigate();
  const nameInput = useRef<HTMLInputElement>(null);
  const [draft] = useState(readDraft);
  const [name, setName] = useState<string>(draft.name || "");
  const [tier, setTier] = useState<TierId>(draft.tier || "pro");
  const [access, setAccess] = useState<"all" | "custom">(draft.access || "all");
  const [models, setModels] = useState<string[]>(draft.models || []);
  const [quota, setQuota] = useState<string>(draft.quota || "");
  const [period, setPeriod] = useState<"never" | "daily" | "monthly">(
    draft.period || "never",
  );
  const [ips, setIps] = useState<string>(draft.ips || "");
  const [ipsEnabled, setIpsEnabled] = useState<boolean>(
    draft.ipsEnabled ?? Boolean(draft.ips?.trim()),
  );
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<PreviewKey | null>(null);
  const config = useModelPricing((s) => s.config);
  const accountId = useProductPreview((s) => s.user.id);
  const keys = useBackendKeys().keys;
  const createToken = useCreateToken();
  const available = getTierModels(tier, config);
  const allowed =
    access === "all"
      ? available
      : available.filter((m) => models.includes(m.id));
  const reusable = initialModel
    ? keys.find(
        (k) =>
          k.active &&
          getKeyModelOptions(k, undefined, config).some(
            (m) => m.id === initialModel,
          ),
      )
    : undefined;
  useEffect(() => {
    if (!initialModel || initialModel === "veyra-auto") return;
    const match = visibleGroups(config, accountId).find((t) =>
      getTierModels(t.id, config).some((m) => m.id === initialModel),
    );
    if (
      match &&
      !getTierModels(tier, config).some((m) => m.id === initialModel)
    )
      setTier(match.id);
  }, [initialModel]);
  useEffect(() => {
    if (
      initialGroup &&
      visibleGroups(config, accountId).some((group) => group.id === initialGroup)
    )
      setTier(initialGroup);
  }, [initialGroup]);
  useEffect(() => {
    try {
      sessionStorage.setItem(
        draftKey,
        JSON.stringify({
          name,
          tier,
          access,
          models,
          quota,
          period,
          ips,
          ipsEnabled,
        }),
      );
    } catch {}
  }, [name, tier, access, models, quota, period, ips, ipsEnabled]);
  const close = (next: boolean) => {
    setOpen(next);
    if (!next && created) {
      setCreated(null);
      setName("");
      setAccess("all");
      setModels([]);
      setQuota("");
      setIps("");
      setIpsEnabled(false);
      setPeriod("never");
      setAdvancedOpen(false);
      sessionStorage.removeItem(draftKey);
    }
  };
  const proceed = (path: string) => {
    const key = created;
    close(false);
    if (key)
      navigate(
        `${path}?key=${encodeURIComponent(key.id)}&model=${encodeURIComponent(initialModel || key.model)}`,
      );
  };
  const submit = () => {
    if (!visibleGroups(config, accountId).some((group) => group.id === tier)) {
      setError(
        c(
          "当前账号不可用此服务分组，请重新选择。",
          "This group is unavailable for this account. Select another.",
          "هذه المجموعة غير متاحة للحساب. اختر مجموعة أخرى.",
        ),
      );
      return;
    }
    if (!name.trim() || name.trim().length > 50) {
      setError(
        c(
          "请输入 1～50 字名称。",
          "Enter a name of 1–50 characters.",
          "أدخل اسمًا من 1 إلى 50 حرفًا.",
        ),
      );
      return;
    }
    if (!allowed.length) {
      setAdvancedOpen(true);
      setError(
        c(
          "至少授权一个当前可用模型。",
          "Allow at least one available model.",
          "اسمح بنموذج متاح واحد.",
        ),
      );
      return;
    }
    if (
      quota !== "" &&
      (!Number.isFinite(Number(quota)) || Number(quota) < 0)
    ) {
      setError(
        c(
          "消费额度必须大于或等于零。",
          "Spending limit must be zero or positive.",
          "يجب أن يكون حد الإنفاق غير سالب.",
        ),
      );
      return;
    }
    if (ipsEnabled && !ips.trim()) {
      setAdvancedOpen(true);
      setError(
        c(
          "已启用网络地址白名单，请至少填写一个地址，或停用该限制。",
          "Enter at least one address or disable the IP allowlist.",
          "أدخل عنوانًا واحدًا على الأقل أو عطّل قائمة العناوين.",
        ),
      );
      return;
    }
    try {
      const input: TokenCreateInput = {
        name: name.trim(),
        group: tierToGroup(tier),
        remain_quota: quota === "" ? undefined : usdToQuota(Number(quota)),
        unlimited_quota: quota === "" ? true : undefined,
        model_limits:
          access === "custom" ? joinList(allowed.map((m) => m.id)) : "",
        allow_ips: ipsEnabled
          ? joinList(
              ips
                .split("\n")
                .map((v) => v.trim())
                .filter(Boolean),
            )
          : "",
      };
      createToken.mutate(input, {
        onSuccess: (token) => {
          const previewKey = tokenToPreviewKey(token, token.key);
          setCreated(previewKey);
          setError("");
          onCreated?.(String(token.id), token.key);
        },
        onError: (err) => {
          setError(
            err instanceof ApiError
              ? err.message
              : String(err instanceof Error ? err.message : err),
          );
        },
      });
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    }
  };
  return (
    <Modal
      open={open}
      onOpenChange={close}
      preventOutsideClose
      size="wide"
      initialFocus={nameInput}
      title={
        created
          ? c("密钥已创建", "Key created", "تم إنشاء المفتاح")
          : c("创建密钥", "Create key", "إنشاء مفتاح")
      }
      description={c(
        "Key（密钥）控制权限和消费；具体模型在客户端选择。所有调用、价格和支付均为本地模拟。",
        "A key controls permissions and spending. Choose models in your client. Calls, prices and payments are simulated.",
        "يتحكم المفتاح بالصلاحيات والإنفاق؛ اختر النموذج في العميل. كل الطلبات والأسعار والمدفوعات محاكاة.",
      )}
      footer={
        created ? (
          <>
            <Button onClick={() => proceed("/console/playground")}>
              {c(
                "验证配置（可选）",
                "Verify configuration (optional)",
                "تحقق من الإعداد (اختياري)",
              )}
            </Button>
            <Button
              variant="primary"
              onClick={() => proceed("/console/connect")}
            >
              {c("接入工具", "Connect a tool", "ربط أداة")}
              <ArrowRight size={15} />
            </Button>
          </>
        ) : (
          <>
            <Button onClick={() => close(false)}>
              {c(
                "取消，保留草稿",
                "Cancel, keep draft",
                "إلغاء مع حفظ المسودة",
              )}
            </Button>
            <Button
              form="new-key-form"
              type="submit"
              variant="primary"
              disabled={!allowed.length || createToken.isPending}
            >
              {c("创建密钥", "Create key", "إنشاء مفتاح")}
            </Button>
          </>
        )
      }
    >
      {created ? (
        <div className="key-success">
          <CircleCheck size={36} />
          <h3>{created.name}</h3>
          <p>
            {c(
              `已授权 ${getKeyModels(created).length} 个具体模型与智能路由；同一把密钥可交替调用。`,
              `${getKeyModels(created).length} models and smart routing allowed with this key.`,
              `يمكن استخدام ${getKeyModels(created).length} نماذج والتوجيه الذكي بهذا المفتاح.`,
            )}
          </p>
          <div className="created-secret">
            <code dir="ltr">{created.keySecret}</code>
            <Button
              onClick={() =>
                copyText(created.keySecret, t("copied"), t("copyFailed"))
              }
            >
              <Copy size={14} />
              {t("copy")}
            </Button>
          </div>
          <p className="field-hint">
            {c(
              "演示凭证，不可用于真实服务。以后可通过演示验证再次查看。",
              "Demo credential, unusable for live services. You can reveal it again after demo verification.",
              "بيانات اعتماد تجريبية لا تعمل مع خدمات حقيقية.",
            )}
          </p>
        </div>
      ) : (
        <form
          id="new-key-form"
          className="create-key-form key-task-form"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          {initialModel && (
            <div className="key-policy-summary">
              {c("接入示例模型：", "Example model: ", "نموذج المثال: ")}
              <code>{initialModel}</code>
              {c(
                "；不会自动限制授权范围。",
                "; this does not restrict permissions.",
                "؛ لا يقيّد الصلاحيات.",
              )}
            </div>
          )}
          {reusable && (
            <div className="key-reuse">
              <p>
                {c(
                  `已有兼容密钥「${reusable.name}」，可以直接复用。`,
                  `Compatible key “${reusable.name}” is available.`,
                  `المفتاح المتوافق «${reusable.name}» متاح.`,
                )}
              </p>
              <Button
                onClick={() => {
                  close(false);
                  navigate(
                    `/console/connect?key=${reusable.id}&model=${initialModel}`,
                  );
                }}
              >
                {c(
                  "复用已有密钥",
                  "Reuse existing key",
                  "إعادة استخدام المفتاح",
                )}
              </Button>
            </div>
          )}
          <Field id="new-key-name" label={t("pKeyName")}>
            <Input
              id="new-key-name"
              ref={nameInput}
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              maxLength={50}
              placeholder={c(
                "例如：研发工具、客服应用",
                "e.g. Development tools",
                "مثل أدوات التطوير",
              )}
            />
          </Field>
          <TierModelPicker value={tier} onChange={setTier} />
          <p className="key-policy-hint">
            {c(
              `默认开放本分组 ${available.length} 个具体模型及智能路由入口。模型由每次请求的 model（模型标识）决定。`,
              `All ${available.length} group models and smart routes are allowed by default. Choose the model per request.`,
              `يسمح افتراضيًا بكل نماذج المجموعة (${available.length}) والتوجيه الذكي.`,
            )}
          </p>
          <div className="key-task-fields">
            <Field
              id="new-key-quota"
              label={c(
                "消费额度（美元）",
                "Spending limit (USD)",
                "حد الإنفاق (دولار)",
              )}
              hint={c(
                "留空不限额；0 表示禁止产生费用。",
                "Empty means unlimited; 0 prevents spending.",
                "فارغ يعني بلا حد؛ صفر يمنع الإنفاق.",
              )}
            >
              <Input
                id="new-key-quota"
                type="number"
                min="0"
                step="0.01"
                value={quota}
                onChange={(e) => setQuota(e.target.value)}
                placeholder={c("不限额", "Unlimited", "بلا حد")}
              />
            </Field>
            <Field
              id="new-key-period"
              label={c("额度周期", "Limit period", "فترة الحد")}
            >
              <select
                id="new-key-period"
                className="select-trigger"
                value={period}
                onChange={(e) => setPeriod(e.target.value as typeof period)}
              >
                <option value="never">{t("neverReset")}</option>
                <option value="daily">{t("dailyReset")}</option>
                <option value="monthly">{t("monthlyReset")}</option>
              </select>
            </Field>
          </div>
          <KeyAdvancedSettings
            open={advancedOpen}
            onOpenChange={setAdvancedOpen}
          >
            <KeyModelAccess
              access={access}
              onAccessChange={setAccess}
              selected={models}
              onSelectedChange={setModels}
              models={available}
              tier={tier}
            />
            <KeyIpAllowlist
              id="new-key-ips"
              enabled={ipsEnabled}
              onEnabledChange={setIpsEnabled}
              value={ips}
              onChange={setIps}
            />
          </KeyAdvancedSettings>
          {error && (
            <p role="alert" className="field-error">
              {error}
            </p>
          )}
        </form>
      )}
    </Modal>
  );
}
