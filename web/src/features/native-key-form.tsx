import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { Modal } from "../components/ui/overlay";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { useTokenChoices } from "../lib/api/token-choices";
import {
  useCreateToken,
  useUpdateToken,
  readOwnedKeyReceipt,
  type Token,
} from "../lib/api/tokens";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useSessionQueryIdentity } from "../lib/system-access";
import type { PreviewKey } from "../lib/product-preview";
import { isSmartModelId } from "../lib/smart-model";
import {
  keySpendingLimitValue,
  remainingQuotaForLifetimeLimit,
} from "../lib/key-spending-limit";
import { copyText } from "./code-example";
export function NativeKeyForm({
  item,
  onClose,
  onCreated,
  initialModel,
  initialGroup,
}: {
  item?: PreviewKey;
  onClose: () => void;
  onCreated?: (token: Token) => void;
  initialModel?: string;
  initialGroup?: string;
}) {
  const { i18n } = useTranslation(),
    navigate = useNavigate(),
    scope = useSessionQueryIdentity(),
    query = useTokenChoices(),
    create = useCreateToken(),
    update = useUpdateToken(),
    { keys } = useBackendKeys();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const choices = query.isError ? undefined : query.data,
    initialRevision = useRef<string>();
  if (!initialRevision.current && choices)
    initialRevision.current = choices.revision;
  const [name, setName] = useState(item?.name ?? ""),
    [group, setGroup] = useState(item?.tier ?? initialGroup ?? ""),
    [models, setModels] = useState<string[] | null>(
      item?.allowedModels ?? null,
    ),
    [quota, setQuota] = useState(() => keySpendingLimitValue(item)),
    [ips, setIps] = useState(item?.ipWhitelist?.join("\n") ?? ""),
    [error, setError] = useState(""),
    [operation, setOperation] = useState(""),
    [receipt, setReceipt] = useState(""),
    [created, setCreated] = useState<Token | null>(null),
    [busy, setBusy] = useState(false);
  const pending = useRef(false);
  const groups = [...new Set(choices?.choices.map((c) => c.group) ?? [])];
  const effectiveGroup = groups.includes(group)
    ? group
    : !item
      ? (groups.find((g) =>
          choices?.choices.some(
            (c) => c.group === g && c.model === initialModel,
          ),
        ) ??
        groups[0] ??
        "")
      : group;
  const available = [
    ...new Set(
      choices?.choices
        .filter((c) => c.group === effectiveGroup)
        .map((c) => c.model) ?? [],
    ),
  ];
  const preservingAutoGroup =
    item?.tier === "auto" && effectiveGroup === "auto";
  const selected = preservingAutoGroup
    ? (item.allowedModels ?? [])
    : (models ?? available);
  const policyChanged =
    !item ||
    effectiveGroup !== item.tier ||
    (!preservingAutoGroup &&
      models !== null &&
      models.join(",") !== (item.allowedModels ?? []).join(","));
  const quotaChanged = !item || quota !== keySpendingLimitValue(item);
  const reusable = initialModel
    ? keys.find(
        (k) =>
          k.active &&
          choices?.choices.some(
            (c) =>
              c.group === k.tier &&
              (isSmartModelId(initialModel)
                ? k.smartAccess === true
                : c.model === initialModel &&
                  (k.modelAccess === "all" ||
                    k.allowedModels?.includes(initialModel))),
          ),
      )
    : undefined;
  async function submit() {
    if (
      pending.current ||
      operation ||
      !choices ||
      !initialRevision.current ||
      query.isError
    )
      return;
    if (
      !name.trim() ||
      (policyChanged &&
        (!selected.length || selected.some((m) => !available.includes(m))))
    ) {
      setError(
        c(
          "请填写名称并选择可用模型。",
          "Enter a name and select available models.",
          "أدخل اسمًا واختر نماذج متاحة.",
        ),
      );
      return;
    }
    let remain: number | undefined;
    if (quotaChanged && quota !== "") {
      try {
        remain = remainingQuotaForLifetimeLimit(quota, item);
      } catch (e) {
        const reason = e instanceof Error ? e.message : "invalid_amount";
        setError(
          reason === "below_used"
            ? c(
                "累计额度不能低于已消费金额。",
                "The lifetime limit cannot be below the amount already spent.",
                "لا يمكن أن يكون الحد التراكمي أقل من المبلغ المنفق.",
              )
            : reason === "too_large"
              ? c(
                  "额度过大。",
                  "Spending limit is too large.",
                  "حد الإنفاق كبير جدًا.",
                )
              : c(
                  "请输入非负美元金额，最多六位小数。",
                  "Enter a nonnegative USD amount with at most six decimals.",
                  "أدخل مبلغًا غير سالب بست منازل عشرية كحد أقصى.",
                ),
        );
        return;
      }
    }
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const payload = {
        name: name.trim(),
        ...(policyChanged
          ? { group: effectiveGroup, model_limits: selected.join(",") }
          : {}),
        allow_ips: ips
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean)
          .join(","),
        ...(quotaChanged ? { unlimited_quota: quota === "" } : {}),
        ...(remain === undefined ? {} : { remain_quota: remain }),
        choices_revision: initialRevision.current,
      };
      if (item) {
        await update.mutateAsync({
          ...payload,
          id: Number(item.id),
          expected_revision: item.managementRevision,
          expected_quota: item.quota,
        });
        onClose();
      } else {
        const token = await create.mutateAsync(payload);
        setCreated(token);
        onCreated?.(token);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      if (!(
        e instanceof Object &&
        "status" in e &&
        [400, 409].includes(Number(e.status))
      ))
        setOperation((e as { operationId?: string })?.operationId ?? "");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function checkReceipt() {
    if (!operation || pending.current) return;
    pending.current = true;
    setBusy(true);
    try {
      const result = await readOwnedKeyReceipt(operation, scope);
      setReceipt(
        c(
          `操作已确认，密钥 ID：${result.token_id}。请重新读取列表；明文不能恢复。`,
          `Operation confirmed for key ${result.token_id}. Reload keys; plaintext cannot be recovered.`,
          `تأكدت العملية للمفتاح ${result.token_id}. أعد تحميل المفاتيح؛ لا يمكن استعادة النص السري.`,
        ),
      );
    } catch {
      setReceipt(
        c(
          "未查到可确认的收据，不能据此判断写入失败。",
          "No confirmed receipt was found; this does not prove the write failed.",
          "لم يُعثر على إيصال مؤكد؛ لا يثبت ذلك فشل الكتابة.",
        ),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Modal
      open
      onOpenChange={(open) => {
        if (!open && !pending.current) onClose();
      }}
      preventOutsideClose
      size="wide"
      title={
        created
          ? c("密钥已创建", "Key created", "أُنشئ المفتاح")
          : item
            ? c("编辑密钥", "Edit key", "تعديل المفتاح")
            : c("创建密钥", "Create key", "إنشاء مفتاح")
      }
      description={c(
        "使用服务器分组和明确模型白名单。密钥只在创建或重置成功时展示一次明文。",
        "Use server groups and an explicit model allowlist. Plaintext is shown once after creation or regeneration.",
        "استخدم مجموعات الخادم وقائمة نماذج صريحة. يظهر النص السري مرة واحدة بعد الإنشاء أو التجديد.",
      )}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {c("关闭", "Close", "إغلاق")}
          </Button>
          {!created && !receipt && (
            <Button
              variant="primary"
              disabled={busy || !choices || query.isError || !!operation}
              onClick={() => void submit()}
            >
              {item
                ? c("保存", "Save", "حفظ")
                : c("创建密钥", "Create key", "إنشاء مفتاح")}
            </Button>
          )}
        </>
      }
    >
      {created ? (
        <div className="key-success">
          <div className="created-secret">
            <code dir="ltr">{created.key}</code>
            <Button
              onClick={() =>
                void copyText(
                  created.key,
                  c("已复制", "Copied", "نُسخ"),
                  c("复制失败", "Copy failed", "فشل النسخ"),
                )
              }
            >
              {c("复制", "Copy", "نسخ")}
            </Button>
          </div>
          <p>
            {c(
              "关闭后无法再次获取此明文，请妥善保存。",
              "Save this secret now. It cannot be recovered after closing.",
              "احفظ السر الآن؛ لا يمكن استعادته بعد الإغلاق.",
            )}
          </p>
        </div>
      ) : (
        <div>
          {initialModel && (
            <p>
              {c("接入示例模型：", "Example model: ", "نموذج المثال: ")}
              <code>{initialModel}</code>
            </p>
          )}
          {reusable && (
            <div className="key-reuse">
              <p>{reusable.name}</p>
              <Button
                onClick={() => {
                  onClose();
                  navigate(
                    `/console/connect?key=${reusable.id}&model=${encodeURIComponent(initialModel!)}`,
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
          <form
            id={item ? "edit-key-form" : "new-key-form"}
            onSubmit={(e) => {
              e.preventDefault();
              void submit();
            }}
          >
            <fieldset
              disabled={busy || !!operation}
              style={{
                border: 0,
                padding: 0,
                minWidth: 0,
                display: "grid",
                gap: "1rem",
              }}
            >
              <label>
                {c("密钥名称", "Key name", "اسم المفتاح")}
                <Input
                  id={item ? "edit-key-name" : "new-key-name"}
                  value={name}
                  maxLength={64}
                  onChange={(e) => setName(e.target.value)}
                />
              </label>
              <label>
                {c("服务器分组", "Server group", "مجموعة الخادم")}
                <select
                  id="native-key-group"
                  className="select-trigger"
                  value={effectiveGroup}
                  onChange={(e) => {
                    setGroup(e.target.value);
                    setModels(
                      e.target.value === item?.tier
                        ? (item.allowedModels ?? null)
                        : null,
                    );
                  }}
                >
                  {!groups.includes(effectiveGroup) && (
                    <option value={effectiveGroup}>
                      {effectiveGroup || "—"}
                    </option>
                  )}
                  {groups.map((g) => (
                    <option key={g} value={g}>
                      {g}
                    </option>
                  ))}
                </select>
              </label>
              <div>
                <p>
                  {c(
                    "明确授权的模型",
                    "Explicitly allowed models",
                    "النماذج المسموحة صراحةً",
                  )}
                </p>
                {preservingAutoGroup && (
                  <p role="status">
                    {c(
                      "保留现有自动分组及模型授权。可直接修改名称、额度和 IP 限制；选择具体分组才会更改路由授权。",
                      "Existing automatic groups and model permissions are preserved. Edit the name, limit or IP restrictions directly; selecting a specific group changes routing permissions.",
                      "تُحفظ المجموعات التلقائية وأذونات النماذج الحالية. يمكنك تعديل الاسم أو الحد أو قيود IP؛ اختيار مجموعة محددة يغيّر أذونات التوجيه.",
                    )}
                  </p>
                )}
                {(preservingAutoGroup
                  ? (item.allowedModels ?? [])
                  : available
                ).map((m) => (
                  <label
                    key={m}
                    style={{
                      display: "flex",
                      gap: ".5rem",
                      overflowWrap: "anywhere",
                    }}
                  >
                    <input
                      type="checkbox"
                      checked={selected.includes(m)}
                      disabled={preservingAutoGroup}
                      onChange={(e) =>
                        setModels(
                          e.target.checked
                            ? [...selected, m]
                            : selected.filter((x) => x !== m),
                        )
                      }
                    />
                    <span>{m}</span>
                  </label>
                ))}
              </div>
              <label>
                {c(
                  "消费额度（美元，留空不限额）",
                  "Spending limit (USD, empty for unlimited)",
                  "حد الإنفاق بالدولار، فارغ بلا حد",
                )}
                <Input
                  id="new-key-quota"
                  inputMode="decimal"
                  value={quota}
                  onChange={(e) => setQuota(e.target.value)}
                />
              </label>
              <p>
                {c(
                  "额度为累计上限，不会按日或按月重置。",
                  "The spending limit is lifetime; it does not reset daily or monthly.",
                  "حد الإنفاق تراكمي ولا يُعاد يوميًا أو شهريًا.",
                )}
              </p>
              <label>
                {c(
                  "允许的 IP 地址（每行一个，留空不限）",
                  "Allowed IP addresses (one per line, empty for unrestricted)",
                  "عناوين IP المسموحة، عنوان لكل سطر، فارغ دون قيد",
                )}
                <textarea
                  className="input"
                  value={ips}
                  onChange={(e) => setIps(e.target.value)}
                />
              </label>
            </fieldset>
          </form>
          {!choices && (
            <p role="alert">
              {c(
                "尚未确认服务器密钥选项，请重新读取。",
                "Server key choices are unconfirmed. Reload to retry.",
                "خيارات مفاتيح الخادم غير مؤكدة. أعد التحميل.",
              )}
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {operation && (
            <>
              <p>
                {c(
                  "结果未确认，请核对收据；不要重复提交。",
                  "The result is unconfirmed. Check the receipt before another action.",
                  "النتيجة غير مؤكدة. تحقق من الإيصال قبل إجراء آخر.",
                )}
              </p>
              <Button disabled={busy} onClick={() => void checkReceipt()}>
                {c(
                  "查询操作收据",
                  "Check operation receipt",
                  "التحقق من إيصال العملية",
                )}
              </Button>
            </>
          )}
          {receipt && <p role="status">{receipt}</p>}
          <Button
            disabled={busy || query.isFetching || !!operation}
            onClick={() => void query.refetch()}
          >
            {c(
              "重新读取密钥选项",
              "Reload key choices",
              "إعادة تحميل خيارات المفاتيح",
            )}
          </Button>
        </div>
      )}
    </Modal>
  );
}
