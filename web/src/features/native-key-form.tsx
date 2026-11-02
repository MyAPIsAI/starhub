import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { ChevronDown } from "lucide-react";
import { Drawer } from "../components/ui/overlay";
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
import {
  NativeKeyModelPolicy,
  type NativeModelScope,
  type NativePolicyError,
} from "./native-key-model-policy";
import "./native-key-form.css";
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
  const choicesFailed = query.isError && !query.isFetching;
  if (!initialRevision.current && choices && !query.isFetching)
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
  const [smartEnabled, setSmartEnabled] = useState(item?.smartAccess ?? true);
  const [period, setPeriod] = useState(item?.quotaPeriod ?? "never");
  const [expiry, setExpiry] = useState(() =>
    item?.validUntil
      ? new Date(
          new Date(item.validUntil).getTime() -
            new Date(item.validUntil).getTimezoneOffset() * 60000,
        )
          .toISOString()
          .slice(0, 16)
      : "",
  );
  const [smartDirty, setSmartDirty] = useState(false);
  const [expiryMode, setExpiryMode] = useState<"never" | "date">(
    item?.validUntil ? "date" : "never",
  );
  const [accessOpen, setAccessOpen] = useState(false);
  const [accessError, setAccessError] = useState<"expiry" | "ips" | null>(null);
  const expiryInput = useRef<HTMLInputElement>(null);
  const ipInput = useRef<HTMLTextAreaElement>(null);
  const ipEntries = ips
    .split(/\r?\n/)
    .map((value) => value.trim())
    .filter(Boolean);
  const [candidateModels, setCandidateModels] = useState<string[] | null>(
    item?.smartCandidates ?? null,
  );
  const pending = useRef(false);
  const [modelScope, setModelScope] = useState<NativeModelScope>(
    item ? "custom" : "snapshot",
  );
  const [snapshotModels, setSnapshotModels] = useState<string[] | null>(null);
  const [policyDirty, setPolicyDirty] = useState(false);
  const [policyError, setPolicyError] = useState<NativePolicyError>(null);
  const [validationAttempt, setValidationAttempt] = useState(0);
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
    : modelScope === "snapshot"
      ? (snapshotModels ?? available)
      : (models ?? (item?.modelAccess === "all" ? available : []));
  const policyChanged =
    !item ||
    effectiveGroup !== item.tier ||
    (!preservingAutoGroup &&
      policyDirty &&
      (modelScope === "snapshot" ||
        selected.join(",") !== (item.allowedModels ?? []).join(",")));
  const quotaChanged =
    !item ||
    quota !== keySpendingLimitValue(item) ||
    period !== (item.quotaPeriod ?? "never");
  const smartAvailable = selected.filter(
    (model) =>
      preservingAutoGroup ||
      choices?.choices.some(
        (choice) =>
          choice.group === effectiveGroup &&
          choice.model === model &&
          choice.smart_eligible !== false,
      ),
  );
  const smartSelected = (candidateModels ?? smartAvailable).filter((model) =>
    smartAvailable.includes(model),
  );
  const smartPolicyChanged = !item || smartDirty || policyChanged;
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
      query.isFetching ||
      query.isError
    )
      return;
    if (!name.trim()) {
      setError(c("请填写密钥名称。", "Enter a key name.", "أدخل اسم المفتاح."));
      return;
    }
    if (
      policyChanged &&
      (!selected.length ||
        selected.some(
          (m) =>
            !available.includes(m) &&
            !(item?.tier === effectiveGroup && item.allowedModels?.includes(m)),
        ))
    ) {
      setError("");
      setPolicyError("models");
      setValidationAttempt((value) => value + 1);
      return;
    }
    let remain: number | undefined;
    if (smartPolicyChanged && smartEnabled && smartSelected.length === 0) {
      setError("");
      setPolicyError("candidates");
      setValidationAttempt((value) => value + 1);
      return;
    }
    setPolicyError(null);
    if (
      expiryMode === "date" &&
      (!expiry || !Number.isFinite(new Date(expiry).getTime()))
    ) {
      setError("");
      setAccessError("expiry");
      setAccessOpen(true);
      requestAnimationFrame(() => expiryInput.current?.focus());
      return;
    }
    if (ipEntries.length > 50) {
      setError("");
      setAccessError("ips");
      setAccessOpen(true);
      requestAnimationFrame(() => ipInput.current?.focus());
      return;
    }
    setAccessError(null);
    if (quotaChanged && quota !== "") {
      try {
        remain = remainingQuotaForLifetimeLimit(quota);
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
        ...(smartPolicyChanged
          ? {
              smart_access: smartEnabled,
              smart_candidates: smartSelected.join(","),
            }
          : {}),
        allow_ips: ipEntries.join(","),
        ...(quotaChanged ? { unlimited_quota: quota === "" } : {}),
        ...(remain === undefined ? {} : { quota_limit: remain }),
        ...(quotaChanged ? { quota_period: period } : {}),
        expired_time:
          expiryMode === "date"
            ? Math.floor(new Date(expiry).getTime() / 1000)
            : -1,
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
      setError(
        e instanceof Error && e.message.startsWith("key limit reached:")
          ? c(
              "每个账号最多创建 200 把密钥，请先删除不再使用的密钥。",
              "This account has reached the 200-key limit. Delete an unused key before creating another.",
              "بلغ الحساب الحد الأقصى البالغ 200 مفتاح. احذف مفتاحًا غير مستخدم قبل إنشاء آخر.",
            )
          : e instanceof Error
            ? e.message
            : String(e),
      );
      if (!(
        e instanceof Object &&
        "status" in e &&
        [400, 404, 409].includes(Number(e.status))
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
    <Drawer
      open
      preventOutsideClose
      onOpenChange={(open) => {
        if (!open && !pending.current) onClose();
      }}
      title={
        created
          ? c("密钥已创建", "Key created", "أُنشئ المفتاح")
          : item
            ? c("编辑密钥", "Edit key", "تعديل المفتاح")
            : c("创建密钥", "Create key", "إنشاء مفتاح")
      }
      description={c(
        "设置服务分组、模型授权、额度和访问限制。查看完整密钥需要验证登录密码。",
        "Set the service group, model permissions, spending limit and access restrictions. Viewing the full key requires your login password.",
        "اضبط مجموعة الخدمة وأذونات النماذج وحد الإنفاق وقيود الوصول. عرض المفتاح الكامل يتطلب كلمة مرور تسجيل الدخول.",
      )}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {c("关闭", "Close", "إغلاق")}
          </Button>
          {!created && !receipt && (
            <Button
              variant="primary"
              disabled={
                busy ||
                !choices ||
                query.isError ||
                query.isFetching ||
                !initialRevision.current ||
                !!operation
              }
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
              "请妥善保存密钥。之后可在列表中验证登录密码后再次查看。",
              "Store the key securely. You can view it again from the list after verifying your login password.",
              "احفظ المفتاح بأمان. يمكنك عرضه مجددًا من القائمة بعد التحقق من كلمة مرور تسجيل الدخول.",
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
              disabled={busy || !!operation || query.isFetching}
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
                  onChange={(e) =>
                    setName(Array.from(e.target.value).slice(0, 50).join(""))
                  }
                />
              </label>
              <label>
                {c("服务分组", "Service group", "مجموعة الخدمة")}
                <select
                  id="native-key-group"
                  className="select-trigger"
                  value={effectiveGroup}
                  onChange={(e) => {
                    setGroup(e.target.value);
                    setModelScope(
                      e.target.value === item?.tier ? "custom" : "snapshot",
                    );
                    setSnapshotModels(
                      choices?.choices
                        .filter((choice) => choice.group === e.target.value)
                        .map((choice) => choice.model) ?? [],
                    );
                    setPolicyDirty(e.target.value !== item?.tier);
                    setPolicyError(null);
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
              <NativeKeyModelPolicy
                choices={choices?.choices ?? []}
                group={effectiveGroup}
                available={available}
                selected={selected}
                scope={modelScope}
                locked={preservingAutoGroup}
                existing={!!item}
                legacyUnrestricted={!!item && item.modelAccess === "all"}
                onScopeChange={(scope) => {
                  setModelScope(scope);
                  if (scope === "snapshot") setSnapshotModels([...available]);
                  if (scope === "snapshot" && choices && !query.isFetching)
                    initialRevision.current = choices.revision;
                  setPolicyDirty(true);
                  setPolicyError(null);
                  setError("");
                }}
                onModelsChange={(next) => {
                  setModels(next);
                  setPolicyDirty(true);
                  if (candidateModels !== null)
                    setCandidateModels(
                      candidateModels.filter((model) => next.includes(model)),
                    );
                  setPolicyError(null);
                  setError("");
                }}
                smartEnabled={smartEnabled}
                onSmartChange={(enabled) => {
                  setSmartEnabled(enabled);
                  setSmartDirty(true);
                  setPolicyError(null);
                  setError("");
                }}
                smartAvailable={smartAvailable}
                smartSelected={smartSelected}
                onCandidatesChange={(next) => {
                  setCandidateModels(next);
                  setSmartDirty(true);
                  setPolicyError(null);
                  setError("");
                }}
                error={policyError}
                validationAttempt={validationAttempt}
              />
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
              <label>
                {c("额度周期", "Quota period", "فترة الحصة")}
                <select
                  id="native-key-period"
                  className="select-trigger"
                  value={period}
                  onChange={(event) =>
                    setPeriod(event.target.value as typeof period)
                  }
                >
                  <option value="never">
                    {c("不重置", "Never reset", "دون إعادة تعيين")}
                  </option>
                  <option value="daily">
                    {c("每天重置", "Daily", "يوميًا")}
                  </option>
                  <option value="monthly">
                    {c("每月重置", "Monthly", "شهريًا")}
                  </option>
                </select>
                <small>
                  {c(
                    "按地区时区在午夜重置：",
                    "Resets at midnight in the region timezone: ",
                    "يُعاد عند منتصف الليل بتوقيت المنطقة: ",
                  )}
                  {item?.quotaTimezone ??
                    choices?.quota_timezone ??
                    "Asia/Dubai"}
                </small>
              </label>
              <section className="native-key-access">
                <button
                  id="native-key-access-toggle"
                  type="button"
                  className="native-key-access-toggle"
                  aria-expanded={accessOpen}
                  aria-controls="native-key-access-fields"
                  onClick={() => setAccessOpen((open) => !open)}
                >
                  <span className="native-key-access-heading">
                    <strong>
                      {c("访问控制", "Access control", "التحكم في الوصول")}
                    </strong>
                    <span>
                      {c(
                        "有效期 · 来源地址限制 · 可选",
                        "Expiration · Source restrictions · Optional",
                        "الصلاحية · قيود المصدر · اختياري",
                      )}
                    </span>
                  </span>
                  <ChevronDown size={15} aria-hidden="true" />
                </button>
                <div
                  id="native-key-access-fields"
                  className="native-key-access-body"
                  role="region"
                  aria-labelledby="native-key-access-toggle"
                  hidden={!accessOpen}
                >
                  <fieldset className="native-key-expiry">
                    <legend>{c("有效期", "Expiration", "الصلاحية")}</legend>
                    <div className="native-key-expiry-options">
                      {(["never", "date"] as const).map((mode) => (
                        <label key={mode} className="native-key-expiry-option">
                          <input
                            id={`native-key-expiry-${mode}`}
                            type="radio"
                            name="native-key-expiry-mode"
                            value={mode}
                            checked={expiryMode === mode}
                            onChange={() => {
                              setExpiryMode(mode);
                              setAccessError(null);
                            }}
                          />
                          <span>
                            {mode === "never"
                              ? c("永久有效", "No expiry", "دون انتهاء")
                              : c(
                                  "指定到期时间",
                                  "Set expiration",
                                  "تحديد الانتهاء",
                                )}
                          </span>
                        </label>
                      ))}
                    </div>
                    {expiryMode === "date" && (
                      <label className="native-key-expiry-date">
                        <span>
                          {c(
                            "到期日期与时间",
                            "Expiration date and time",
                            "تاريخ ووقت الانتهاء",
                          )}
                        </span>
                        <Input
                          ref={expiryInput}
                          id="native-key-expiry"
                          type="datetime-local"
                          value={expiry}
                          aria-invalid={accessError === "expiry"}
                          aria-describedby={
                            accessError === "expiry"
                              ? "native-key-expiry-error"
                              : undefined
                          }
                          onChange={(event) => {
                            setExpiry(event.target.value);
                            setAccessError(null);
                          }}
                        />
                      </label>
                    )}
                    {accessError === "expiry" && (
                      <p
                        id="native-key-expiry-error"
                        className="native-key-access-error"
                        role="alert"
                      >
                        {c(
                          "请选择到期日期与时间。",
                          "Choose an expiration date and time.",
                          "اختر تاريخ ووقت انتهاء الصلاحية.",
                        )}
                      </p>
                    )}
                  </fieldset>
                  <div className="native-key-ip-field">
                    <div className="native-key-ip-heading">
                      <label htmlFor="native-key-ips">
                        {c(
                          "IP（来源地址）白名单",
                          "IP (source address) allowlist",
                          "قائمة عناوين IP المسموحة",
                        )}
                      </label>
                      <span
                        id="native-key-ip-count"
                        dir="ltr"
                        className={
                          ipEntries.length > 50 ? "is-over-limit" : undefined
                        }
                        aria-live="polite"
                        aria-atomic="true"
                      >
                        {ipEntries.length} / 50
                      </span>
                    </div>
                    <textarea
                      ref={ipInput}
                      id="native-key-ips"
                      dir="ltr"
                      className="input native-key-ip-input"
                      rows={4}
                      placeholder={
                        "203.0.113.10\n198.51.100.0/24\n2001:db8::/32"
                      }
                      value={ips}
                      aria-invalid={accessError === "ips"}
                      aria-describedby={`native-key-ip-hint native-key-ip-count${accessError === "ips" ? " native-key-ip-error" : ""}`}
                      onChange={(event) => {
                        setIps(event.target.value);
                        setAccessError(null);
                      }}
                    />
                    <p
                      id="native-key-ip-hint"
                      className="native-key-access-hint"
                    >
                      {c(
                        "每行一条，支持 IPv4（四版地址）、IPv6（六版地址）与 CIDR（网段）；为空表示不限制。",
                        "One entry per line. Supports IPv4, IPv6 and CIDR networks. Leave empty to allow any source.",
                        "إدخال واحد لكل سطر. يدعم IPv4 وIPv6 وشبكات CIDR. اتركه فارغًا للسماح بأي مصدر.",
                      )}
                    </p>
                    {accessError === "ips" && (
                      <p
                        id="native-key-ip-error"
                        className="native-key-access-error"
                        role="alert"
                      >
                        {c(
                          "IP 白名单最多允许 50 条，请移除多余条目。",
                          "The IP allowlist supports up to 50 entries. Remove the extra entries.",
                          "تدعم قائمة IP حتى 50 إدخالًا. احذف الإدخالات الزائدة.",
                        )}
                      </p>
                    )}
                  </div>
                </div>
              </section>
            </fieldset>
          </form>
          {!choices && query.isFetching && (
            <p role="status">
              {c(
                "正在加载服务分组和模型…",
                "Loading service groups and models…",
                "جارٍ تحميل مجموعات الخدمة والنماذج…",
              )}
            </p>
          )}
          {choicesFailed && (
            <p role="alert">
              {c(
                "无法加载服务分组和模型，请重试。",
                "Unable to load service groups and models. Please retry.",
                "تعذر تحميل مجموعات الخدمة والنماذج. أعد المحاولة.",
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
          {choicesFailed && (
            <Button
              id="native-key-choices-retry"
              disabled={busy || query.isFetching || !!operation}
              onClick={() => void query.refetch()}
            >
              {c("重试", "Retry", "إعادة المحاولة")}
            </Button>
          )}
        </div>
      )}
    </Drawer>
  );
}
