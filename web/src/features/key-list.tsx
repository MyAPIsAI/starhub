import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  Check,
  Copy,
  Eye,
  EyeOff,
  KeyRound,
  MoreHorizontal,
  Pause,
  Play,
  Plus,
  RotateCcw,
  Trash2,
  ChevronRight,
  List,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { TierBadge } from "../components/tier-badge";
import { formatMoney } from "../lib/money";
import { keyQuotaSummary } from "../lib/key-quota-summary";
import type { PreviewKey } from "../lib/product-preview";
import {
  allowsSmartModel,
  allowsSpecifiedModels,
  getKeyModels,
} from "../lib/key-model-policy";
import { modelCatalog } from "../lib/platform-catalog";
import { useModelPricing } from "../lib/model-pricing";
import "./key-list.css";

type Text = (zh: string, en: string, ar: string) => string;
export type KeySecretAction = "view" | "copy";
type KeyAction = (key: PreviewKey) => void;
const periodName = (period: PreviewKey["quotaPeriod"], text: Text) =>
  period === "daily"
    ? text("每日重置", "Resets daily", "إعادة يومية")
    : period === "monthly"
      ? text("每月重置", "Resets monthly", "إعادة شهرية")
      : text("不重置", "No reset", "دون إعادة");
const money = (nano: string) =>
  formatMoney(
    nano,
    BigInt(nano) > 0n && BigInt(nano) < 100_000n ? "call" : "total",
  ).replace(/(\.\d{2}.*?)0+$/, "$1");

export function useKeyDisplayTime(expiries: Record<string, number>) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const nextExpiry = Math.min(
      ...Object.values(expiries).filter((value) => value > Date.now()),
    );
    const delay = Math.min(30_000, Math.max(1, nextExpiry - Date.now() + 1));
    const timer = window.setTimeout(() => setNow(Date.now()), delay);
    return () => window.clearTimeout(timer);
  }, [now, expiries]);
  // Transactions can rerender between ticks; evaluate their new period at the
  // current instant rather than the last scheduled refresh.
  return Date.now();
}

function KeyStatus({
  item,
  exhausted,
  exceeded,
  nearLimit,
  now,
  text,
}: {
  item: PreviewKey;
  exhausted: boolean;
  exceeded: boolean;
  nearLimit: boolean;
  now: number;
  text: Text;
}) {
  const expired = Boolean(
    item.validUntil && Date.parse(item.validUntil) <= now,
  );
  return (
    <div className="key-list-status" data-key-action="status">
      <span
        className={`key-status-pill ${item.active ? "is-enabled" : "is-disabled"}`}
      >
        {item.active ? <Check size={12} /> : <Pause size={12} />}
        {item.active
          ? text("已启用", "Enabled", "مفعّل")
          : text("已停用", "Disabled", "معطّل")}
      </span>
      {expired && (
        <small className="key-quota-exhausted">
          {text("已过期", "Expired", "منتهي الصلاحية")}
        </small>
      )}
      {exhausted && (
        <small className="key-quota-exhausted">
          {exceeded
            ? text(
                "已超当前额度",
                "Current limit exceeded",
                "تجاوز الحد الحالي",
              )
            : text("额度已用尽", "Limit reached", "استُنفد الحد")}
        </small>
      )}
      {!exhausted && nearLimit && (
        <small className="key-quota-warning">
          {text("额度将尽", "Near limit", "قرب بلوغ الحد")}
        </small>
      )}
    </div>
  );
}

function KeyActionsMenu({
  item,
  text,
  onDetails,
  onToggle,
  onRegenerate,
  onDelete,
}: {
  item: PreviewKey;
  text: Text;
  onDetails: KeyAction;
  onToggle: KeyAction;
  onRegenerate: KeyAction;
  onDelete: KeyAction;
}) {
  const [open, setOpen] = useState(false);
  const wrapper = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const menuId = `key-actions-${item.id}`;
  useEffect(() => {
    if (!open) return;
    menu.current?.querySelector<HTMLButtonElement>("button")?.focus();
    const outside = (event: PointerEvent) => {
      if (
        event.target instanceof Node &&
        !wrapper.current?.contains(event.target)
      )
        setOpen(false);
    };
    document.addEventListener("pointerdown", outside, true);
    return () => document.removeEventListener("pointerdown", outside, true);
  }, [open]);
  const choose = (action: KeyAction) => {
    trigger.current?.focus();
    setOpen(false);
    action(item);
  };
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!open) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      trigger.current?.focus();
    } else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.preventDefault();
      const items = Array.from(
        menu.current?.querySelectorAll<HTMLButtonElement>("button") ?? [],
      );
      const current = items.indexOf(
        document.activeElement as HTMLButtonElement,
      );
      const next =
        event.key === "Home"
          ? 0
          : event.key === "End"
            ? items.length - 1
            : (current + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
              items.length;
      items[next]?.focus();
    }
  };
  return (
    <div
      className="key-more"
      ref={wrapper}
      onKeyDown={onKeyDown}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null))
          setOpen(false);
      }}
    >
      <button
        type="button"
        ref={trigger}
        className="key-more-trigger"
        data-key-more
        data-key-action="more"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={text(
          `${item.name} 的更多操作`,
          `More actions for ${item.name}`,
          `المزيد من إجراءات ${item.name}`,
        )}
        onClick={() => setOpen(!open)}
        onKeyDown={(event) => {
          if (!open && (event.key === "ArrowDown" || event.key === "ArrowUp")) {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        <MoreHorizontal size={17} />
        <span>{text("更多", "More", "المزيد")}</span>
      </button>
      {open && (
        <div
          className="key-more-menu"
          id={menuId}
          role="menu"
          ref={menu}
          aria-label={text("密钥操作", "Key actions", "إجراءات المفتاح")}
        >
          <button
            type="button"
            role="menuitem"
            data-key-action="details"
            onClick={() => choose(onDetails)}
          >
            <List size={15} />
            {text("密钥详情", "Key details", "تفاصيل المفتاح")}
          </button>
          <button
            type="button"
            role="menuitem"
            data-key-action="toggle"
            onClick={() => choose(onToggle)}
          >
            {item.active ? <Pause size={15} /> : <Play size={15} />}
            {item.active
              ? text("停用密钥", "Disable key", "تعطيل المفتاح")
              : text("启用密钥", "Enable key", "تفعيل المفتاح")}
          </button>
          <button
            type="button"
            role="menuitem"
            data-key-action="regenerate"
            onClick={() => choose(onRegenerate)}
          >
            <RotateCcw size={15} />
            {text("重置密钥", "Regenerate key", "تجديد المفتاح")}
          </button>
          <button
            type="button"
            role="menuitem"
            data-key-action="delete"
            className="is-danger"
            onClick={() => choose(onDelete)}
          >
            <Trash2 size={15} />
            {text("删除密钥", "Delete key", "حذف المفتاح")}
          </button>
        </div>
      )}
    </div>
  );
}

export default function KeyList({
  keys,
  revealedKeyIds,
  visibleKeyIds,
  now,
  onCreate,
  onDetails,
  onEdit,
  onConnect,
  onToggle,
  onRegenerate,
  onDelete,
  onSecret,
  onHide,
}: {
  keys: PreviewKey[];
  revealedKeyIds: Record<string, number>;
  visibleKeyIds: Record<string, boolean>;
  now: number;
  onCreate: () => void;
  onDetails: KeyAction;
  onEdit: KeyAction;
  onConnect: KeyAction;
  onToggle: KeyAction;
  onRegenerate: KeyAction;
  onDelete: KeyAction;
  onSecret: (key: PreviewKey, intent: KeySecretAction) => void;
  onHide: (id: string) => void;
}) {
  const { i18n } = useTranslation();
  const text: Text = (zh, en, ar) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  return (
    <section className="key-manager">
      <header className="key-manager-header">
        <div>
          <h1>{text("访问密钥", "API keys", "مفاتيح الوصول")}</h1>
          <p>
            {text(
              "管理应用凭证、启用状态与消费额度。",
              "Manage application credentials, status and spending limits.",
              "أدر بيانات اعتماد التطبيقات وحالتها وحدود الإنفاق.",
            )}
          </p>
        </div>
        <Button variant="primary" onClick={onCreate}>
          <Plus size={15} />
          {text("创建密钥", "Create key", "إنشاء مفتاح")}
        </Button>
      </header>
      {!keys.length ? (
        <div className="key-list-empty">
          <KeyRound size={28} />
          <h2>{text("还没有访问密钥", "No keys yet", "لا توجد مفاتيح بعد")}</h2>
          <p>
            {text(
              "创建一把密钥，再在客户端选择模型使用。",
              "Create a key, then select your model in your client.",
              "أنشئ مفتاحًا ثم اختر النموذج في تطبيقك.",
            )}
          </p>
          <Button onClick={onCreate}>
            {text("创建密钥", "Create key", "إنشاء مفتاح")}
          </Button>
        </div>
      ) : (
        <div
          className="key-list-table"
          role="table"
          aria-label={text(
            "访问密钥列表",
            "API key list",
            "قائمة مفاتيح الوصول",
          )}
        >
          <div className="key-list-head" role="row">
            {[
              text("密钥", "Key", "المفتاح"),
              text("服务分组", "Service group", "مجموعة الخدمة"),
              text("状态", "Status", "الحالة"),
              text("消费额度", "Spending limit", "حد الإنفاق"),
              text("操作", "Actions", "الإجراءات"),
            ].map((label) => (
              <div key={label} role="columnheader">
                {label}
              </div>
            ))}
          </div>
          {keys.map((item) => {
            const quota = keyQuotaSummary(item, new Date(now).toISOString());
            const visible = Boolean(
              visibleKeyIds[item.id] && revealedKeyIds[item.id] > now,
            );
            return (
              <div
                className="key-list-row"
                data-key-id={item.id}
                key={item.id}
                role="row"
              >
                <div className="key-list-identity" role="cell">
                  <button
                    type="button"
                    className="key-name-button"
                    data-key-action="details"
                    onClick={() => onDetails(item)}
                  >
                    <span>{item.name}</span>
                    <ChevronRight size={14} className="directional" />
                  </button>
                  <div className="key-list-credential">
                    <code dir="ltr">
                      {visible ? item.keySecret : item.keyMasked}
                    </code>
                    <div className="key-secret-actions">
                      <button
                        type="button"
                        data-key-action="reveal"
                        aria-label={
                          visible
                            ? text(
                                `隐藏 ${item.name} 的密钥`,
                                `Hide key for ${item.name}`,
                                `إخفاء مفتاح ${item.name}`,
                              )
                            : text(
                                `查看 ${item.name} 的密钥`,
                                `Reveal key for ${item.name}`,
                                `عرض مفتاح ${item.name}`,
                              )
                        }
                        title={
                          visible
                            ? text("隐藏密钥", "Hide key", "إخفاء المفتاح")
                            : text("验证查看", "Verify to reveal", "تحقق للعرض")
                        }
                        onClick={() =>
                          visible ? onHide(item.id) : onSecret(item, "view")
                        }
                      >
                        {visible ? <EyeOff size={14} /> : <Eye size={14} />}
                      </button>
                      <button
                        type="button"
                        data-key-action="copy"
                        aria-label={text(
                          `复制 ${item.name} 的密钥`,
                          `Copy key for ${item.name}`,
                          `نسخ مفتاح ${item.name}`,
                        )}
                        title={text("复制密钥", "Copy key", "نسخ المفتاح")}
                        onClick={() => onSecret(item, "copy")}
                      >
                        <Copy size={14} />
                      </button>
                    </div>
                  </div>
                </div>
                <div className="key-list-group" role="cell">
                  <span className="key-mobile-label">
                    {text("服务分组", "Service group", "مجموعة الخدمة")}
                  </span>
                  <TierBadge id={item.tier} />
                </div>
                <div className="key-list-state" role="cell">
                  <KeyStatus
                    item={item}
                    exhausted={quota.exhausted}
                    exceeded={quota.exceeded}
                    nearLimit={quota.nearLimit}
                    now={now}
                    text={text}
                  />
                </div>
                <div
                  className="key-list-quota"
                  role="cell"
                  data-key-action="quota"
                >
                  <div className="key-quota-values" dir="ltr">
                    <strong dir="ltr">{money(quota.usedNano)}</strong>
                    <span>
                      {quota.limitNano === null ? (
                        text(" / 不限额", " / Unlimited", " / بلا حد")
                      ) : (
                        <bdi dir="ltr"> / {money(quota.limitNano)}</bdi>
                      )}
                    </span>
                  </div>
                  {quota.percentage !== null && (
                    <div
                      className={`key-quota-track${quota.exhausted ? " is-exhausted" : quota.nearLimit ? " is-near" : ""}`}
                      role="progressbar"
                      aria-label={text(
                        `${item.name} 的消费额度使用率`,
                        `Spending limit used for ${item.name}`,
                        `استخدام حد الإنفاق لـ ${item.name}`,
                      )}
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-valuenow={quota.percentage}
                      aria-valuetext={`${quota.percentage}%`}
                    >
                      <span style={{ width: `${quota.percentage}%` }} />
                    </div>
                  )}
                  <div className="key-quota-caption">
                    <span>{periodName(quota.period, text)}</span>
                    {quota.percentage !== null && (
                      <span dir="ltr">{quota.percentage}%</span>
                    )}
                  </div>
                </div>
                <div className="key-list-actions" role="cell">
                  <Button
                    size="small"
                    variant="secondary"
                    data-key-action="connect"
                    disabled={!item.active}
                    onClick={() => onConnect(item)}
                  >
                    {text("接入", "Connect", "ربط")}
                  </Button>
                  <button
                    className="key-edit-action"
                    type="button"
                    data-key-action="edit"
                    onClick={() => onEdit(item)}
                  >
                    {text("编辑", "Edit", "تعديل")}
                  </button>
                  <KeyActionsMenu
                    item={item}
                    text={text}
                    onDetails={onDetails}
                    onToggle={onToggle}
                    onRegenerate={onRegenerate}
                    onDelete={onDelete}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}
      <p className="key-list-footnote">
        {text(
          `${keys.length} 把密钥 · 消费金额为模拟数据`,
          `${keys.length} keys · Spending amounts are simulated`,
          `${keys.length} مفاتيح · مبالغ الإنفاق تجريبية`,
        )}
      </p>
    </section>
  );
}

export function KeyDetails({
  item,
  now,
  onEdit,
}: {
  item: PreviewKey;
  now: number;
  onEdit: KeyAction;
}) {
  const { i18n } = useTranslation();
  const text: Text = (zh, en, ar) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const pricing = useModelPricing((state) => state.config);
  const quota = keyQuotaSummary(item, new Date(now).toISOString());
  const available = getKeyModels(item, pricing);
  const catalog = modelCatalog(pricing);
  const modelIds =
    item.modelAccess === "all"
      ? available.map((model) => model.id)
      : (item.allowedModels ?? [item.model]);
  const date = (value?: string | null) =>
    value
      ? new Intl.DateTimeFormat(i18n.language, {
          dateStyle: "medium",
          timeStyle: "short",
          timeZone: "Asia/Dubai",
        }).format(new Date(value))
      : text("尚无记录", "Not recorded", "غير مسجّل");
  return (
    <div className="key-detail" data-key-detail={item.id}>
      <section>
        <h3>{text("基本信息", "Basic information", "المعلومات الأساسية")}</h3>
        <dl>
          <div>
            <dt>{text("密钥名称", "Key name", "اسم المفتاح")}</dt>
            <dd>{item.name}</dd>
          </div>
          <div>
            <dt>{text("密钥标识", "Key ID", "معرّف المفتاح")}</dt>
            <dd>
              <code dir="ltr">{item.id}</code>
            </dd>
          </div>
          <div>
            <dt>{text("服务分组", "Service group", "مجموعة الخدمة")}</dt>
            <dd>
              <TierBadge id={item.tier} />
            </dd>
          </div>
          <div>
            <dt>{text("状态", "Status", "الحالة")}</dt>
            <dd>
              <KeyStatus
                item={item}
                exhausted={quota.exhausted}
                exceeded={quota.exceeded}
                nearLimit={quota.nearLimit}
                now={now}
                text={text}
              />
            </dd>
          </div>
          <div>
            <dt>{text("最后使用", "Last used", "آخر استخدام")}</dt>
            <dd>{date(item.lastUsedAt)}</dd>
          </div>
          <div>
            <dt>{text("有效期", "Expires", "انتهاء الصلاحية")}</dt>
            <dd>
              {item.validUntil
                ? date(item.validUntil)
                : text("长期有效", "No expiry", "دون انتهاء")}
            </dd>
          </div>
        </dl>
        <p className="key-detail-hint">
          {text(
            "时间按迪拜时区显示。",
            "Times are shown in Dubai time.",
            "تُعرض الأوقات بتوقيت دبي.",
          )}
        </p>
      </section>
      <section>
        <h3>{text("消费额度", "Spending limit", "حد الإنفاق")}</h3>
        <dl>
          <div>
            <dt>
              {text("当前周期已用", "Used this period", "المستخدم خلال الفترة")}
            </dt>
            <dd dir="ltr">{money(quota.usedNano)}</dd>
          </div>
          <div>
            <dt>{text("额度上限", "Limit", "الحد")}</dt>
            <dd>
              {quota.limitNano === null ? (
                text("不限额", "Unlimited", "بلا حد")
              ) : (
                <bdi dir="ltr">{money(quota.limitNano)}</bdi>
              )}
            </dd>
          </div>
          {quota.remainingNano !== null && (
            <div>
              <dt>{text("剩余额度", "Remaining", "المتبقي")}</dt>
              <dd dir="ltr">{money(quota.remainingNano)}</dd>
            </div>
          )}
          <div>
            <dt>{text("额度周期", "Quota period", "فترة الحصة")}</dt>
            <dd>{periodName(quota.period, text)}</dd>
          </div>
          <div>
            <dt>{text("累计消费", "Lifetime spending", "إجمالي الإنفاق")}</dt>
            <dd dir="ltr">
              {money(item.totalUsed_nano ?? item.currentPeriodUsed_nano ?? "0")}
            </dd>
          </div>
        </dl>
        <p className="key-detail-hint">
          {text(
            "按模拟请求实际累计；周期以迪拜时间为准。",
            "Totals reflect simulated requests; periods use Dubai time.",
            "تعكس الإجماليات الطلبات التجريبية؛ الفترات بتوقيت دبي.",
          )}
        </p>
      </section>
      <section>
        <h3>{text("模型授权", "Model access", "صلاحيات النماذج")}</h3>
        <dl>
          <div>
            <dt>{text("授权范围", "Access scope", "نطاق الصلاحيات")}</dt>
            <dd>
              {item.modelAccess === "all"
                ? text(
                    "分组全部模型",
                    "All group models",
                    "جميع نماذج المجموعة",
                  )
                : text(
                    "模型白名单",
                    "Model allowlist",
                    "قائمة النماذج المسموحة",
                  )}
            </dd>
          </div>
          <div>
            <dt>
              {text(
                "指定模型调用",
                "Direct model requests",
                "طلبات النماذج المباشرة",
              )}
            </dt>
            <dd>
              {allowsSpecifiedModels(item)
                ? text("允许", "Allowed", "مسموح")
                : text("不允许", "Not allowed", "غير مسموح")}
            </dd>
          </div>
          <div>
            <dt>{text("智能路由入口", "Smart routing", "التوجيه الذكي")}</dt>
            <dd>
              {allowsSmartModel(item)
                ? text(
                    "允许 · 受上述授权约束",
                    "Allowed · within this scope",
                    "مسموح ضمن هذا النطاق",
                  )
                : text("不允许", "Not allowed", "غير مسموح")}
            </dd>
          </div>
        </dl>
        {item.smartAccess === undefined && (
          <p className="key-detail-legacy">
            {text(
              "此密钥保留旧版授权，改版没有自动扩大模型或智能路由权限。",
              "Legacy access is preserved. Model and routing permissions were not automatically expanded.",
              "تُحفظ الصلاحيات القديمة دون توسيع تلقائي للنماذج أو التوجيه.",
            )}
          </p>
        )}
        <h4 className="key-detail-model-heading">
          {allowsSpecifiedModels(item)
            ? text("授权模型", "Authorized models", "النماذج المصرّح بها")
            : text("路由候选", "Routing candidates", "مرشحو التوجيه")}
        </h4>
        <ul className="key-detail-models">
          {modelIds.map((id) => (
            <li key={id}>
              <div>
                <strong>
                  {catalog.find((model) => model.id === id)?.name ?? id}
                </strong>
                <code dir="ltr">{id}</code>
              </div>
              {!available.some((model) => model.id === id) && (
                <small>
                  {text(
                    "当前分组不可用",
                    "Unavailable in group",
                    "غير متاح في المجموعة",
                  )}
                </small>
              )}
            </li>
          ))}
        </ul>
        {!modelIds.length && (
          <p className="key-detail-hint">
            {text(
              "当前没有可用的授权模型。",
              "No authorized models are currently available.",
              "لا تتوفر نماذج مصرّح بها حاليًا.",
            )}
          </p>
        )}
      </section>
      <section>
        <h3>{text("来源地址限制", "IP restrictions", "قيود عناوين المصدر")}</h3>
        {item.ipWhitelist?.length ? (
          <ul className="key-detail-ip">
            {item.ipWhitelist.map((ip) => (
              <li key={ip}>
                <code dir="ltr">{ip}</code>
              </li>
            ))}
          </ul>
        ) : (
          <p className="key-detail-hint">
            {text(
              "未设置地址限制。",
              "No IP restrictions set.",
              "لم تُحدّد قيود للعناوين.",
            )}
          </p>
        )}
      </section>
      <Button onClick={() => onEdit(item)}>
        {text("编辑密钥设置", "Edit key settings", "تعديل إعدادات المفتاح")}
      </Button>
    </div>
  );
}
