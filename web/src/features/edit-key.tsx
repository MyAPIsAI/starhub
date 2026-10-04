import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "../components/ui/overlay";
import { Button } from "../components/ui/button";
import { Field, Input } from "../components/ui/primitives";
import {
  KeyModelAccess,
  KeyAdvancedSettings,
  KeyIpAllowlist,
} from "../components/key-model-access";
import { getTierModels } from "../components/tier-model-picker";
import { useModelPricing } from "../lib/model-pricing";
import { useProductPreview, type PreviewKey } from "../lib/product-preview";
import { formatMoney } from "../lib/money";
export default function EditKey({
  item,
  onClose,
}: {
  item: PreviewKey;
  onClose: () => void;
}) {
  const { i18n, t } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [name, setName] = useState(item.name);
  const [quota, setQuota] = useState(
    item.quotaLimit_usd == null ? "" : String(item.quotaLimit_usd),
  );
  const [period, setPeriod] = useState(item.quotaPeriod || "never");
  const [access, setAccess] = useState<"all" | "custom">(
    item.modelAccess || "custom",
  );
  const [models, setModels] = useState(item.allowedModels || [item.model]);
  const [error, setError] = useState("");
  const [ips, setIps] = useState((item.ipWhitelist || []).join("\n"));
  const [ipsEnabled, setIpsEnabled] = useState(
    Boolean(item.ipWhitelist?.length),
  );
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const config = useModelPricing((s) => s.config);
  const update = useProductPreview((s) => s.updateKey);
  const available = getTierModels(item.tier, config);
  const save = () => {
    if (
      !name.trim() ||
      name.trim().length > 50 ||
      (quota !== "" &&
        (!Number.isFinite(Number(quota)) || Number(quota) < 0)) ||
      (access === "custom" && !available.some((m) => models.includes(m.id)))
    ) {
      setError(
        c(
          "请检查名称、非负额度与模型授权。",
          "Check name, nonnegative limit and allowed models.",
          "تحقق من الاسم والحد والنماذج.",
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
      update(item.id, {
        name: name.trim(),
        quotaLimit_usd: quota === "" ? null : Number(quota),
        quotaPeriod: period,
        modelAccess: access,
        allowedModels: access === "custom" ? models : undefined,
        ipWhitelist: ipsEnabled
          ? ips
              .split("\n")
              .map((v) => v.trim())
              .filter(Boolean)
          : [],
      });
      onClose();
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    }
  };
  return (
    <Modal
      open
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
      size="wide"
      preventOutsideClose
      title={c(
        "编辑密钥权限与额度",
        "Edit key permissions and limits",
        "تعديل صلاحيات المفتاح وحدوده",
      )}
      description={c(
        "修改会影响后续请求，历史消费与费率保留。取消不会保存。",
        "Changes affect subsequent requests; historical usage and rates are retained. Cancel discards changes.",
        "تؤثر التغييرات في الطلبات التالية وتبقى الأسعار والاستخدام السابق.",
      )}
      footer={
        <>
          <Button onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" form="edit-key-form" variant="primary">
            {c("保存修改", "Save changes", "حفظ التغييرات")}
          </Button>
        </>
      }
    >
      <form
        id="edit-key-form"
        className="key-task-form"
        onSubmit={(e) => {
          e.preventDefault();
          save();
        }}
      >
        <Field id="edit-key-name" label={t("pKeyName")}>
          <Input
            id="edit-key-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            maxLength={50}
          />
        </Field>
        <div className="key-task-fields">
          <Field
            id="edit-key-quota"
            label={c("消费额度（美元）", "Spending limit (USD)", "حد الإنفاق")}
            hint={c(
              `本周期已用 ${formatMoney(item.currentPeriodUsed_nano || "0", "total")}，改限额不会清空用量。`,
              `Used ${formatMoney(item.currentPeriodUsed_nano || "0", "total")}; changing the limit does not erase usage.`,
              `الاستخدام الحالي ${formatMoney(item.currentPeriodUsed_nano || "0", "total")}`,
            )}
          >
            <Input
              id="edit-key-quota"
              type="number"
              min="0"
              step="0.01"
              value={quota}
              onChange={(e) => setQuota(e.target.value)}
            />
          </Field>
          <Field
            id="edit-key-period"
            label={c("额度周期", "Limit period", "فترة الحد")}
          >
            <select
              id="edit-key-period"
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
        <KeyAdvancedSettings open={advancedOpen} onOpenChange={setAdvancedOpen}>
          <KeyModelAccess
            access={access}
            onAccessChange={setAccess}
            selected={models}
            onSelectedChange={setModels}
            models={available}
            tier={item.tier}
          />
          <KeyIpAllowlist
            id="edit-key-ips"
            enabled={ipsEnabled}
            onEnabledChange={setIpsEnabled}
            value={ips}
            onChange={setIps}
          />
        </KeyAdvancedSettings>
        <p className="field-hint">
          {c(
            "保留此密钥原有的具体模型与智能路由入口权限。白名单同时限制智能路由候选。",
            "Existing concrete-model and route-entry permissions are preserved. The allowlist also constrains routing candidates.",
            "تُحفظ صلاحيات النماذج والتوجيه الحالية وتقيّد القائمة مرشحي التوجيه.",
          )}
        </p>
        {error && (
          <p role="alert" className="field-error">
            {error}
          </p>
        )}
      </form>
    </Modal>
  );
}
