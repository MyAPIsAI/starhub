import { ListPagination, useListPage } from "../components/list-pagination";
import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
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
    ? text(...translatedMessage("resets_daily_be717db526"))
    : period === "monthly"
      ? text(...translatedMessage("resets_monthly_9850fef677"))
      : text(...translatedMessage("no_reset_74958fd486"));
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
          ? text(...translatedMessage("enabled_d4fda6d86a"))
          : text(...translatedMessage("disabled_ab66c5caec"))}
      </span>
      {expired && (
        <small className="key-quota-exhausted">
          {text(...translatedMessage("expired_11167eed99"))}
        </small>
      )}
      {exhausted && (
        <small className="key-quota-exhausted">
          {exceeded
            ? text(...translatedMessage("current_limit_exceeded_cc3d80ebbd"))
            : text(...translatedMessage("limit_reached_04905e78e0"))}
        </small>
      )}
      {!exhausted && nearLimit && (
        <small className="key-quota-warning">
          {text(...translatedMessage("near_limit_6d339b9313"))}
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
          ...translatedMessage("more_actions_for_4965ae6bf1", {
            p0: item.name,
          }),
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
        <span>{text(...translatedMessage("more_cf83566eec"))}</span>
      </button>
      {open && (
        <div
          className="key-more-menu"
          id={menuId}
          role="menu"
          ref={menu}
          aria-label={text(...translatedMessage("key_actions_00ab564c69"))}
        >
          <button
            type="button"
            role="menuitem"
            data-key-action="details"
            onClick={() => choose(onDetails)}
          >
            <List size={15} />
            {text(...translatedMessage("key_details_85c8df0bff"))}
          </button>
          <button
            type="button"
            role="menuitem"
            data-key-action="toggle"
            onClick={() => choose(onToggle)}
          >
            {item.active ? <Pause size={15} /> : <Play size={15} />}
            {item.active
              ? text(...translatedMessage("disable_key_4dffc6b207"))
              : text(...translatedMessage("enable_key_bb19cb41d8"))}
          </button>
          <button
            type="button"
            role="menuitem"
            data-key-action="regenerate"
            onClick={() => choose(onRegenerate)}
          >
            <RotateCcw size={15} />
            {text(...translatedMessage("regenerate_key_230088f779"))}
          </button>
          <button
            type="button"
            role="menuitem"
            data-key-action="delete"
            className="is-danger"
            onClick={() => choose(onDelete)}
          >
            <Trash2 size={15} />
            {text(...translatedMessage("delete_key_90a1ff06c7"))}
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
  const pagination = useListPage(keys);
  const text: Text = (zh, en, ar) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  return (
    <section className="key-manager">
      <header className="key-manager-header">
        <div>
          <h1>{text(...translatedMessage("api_keys_7e3d5e4b7d"))}</h1>
          <p>
            {text(
              ...translatedMessage(
                "manage_application_credentials_status_and_spe_c1f9a26900",
              ),
            )}
          </p>
        </div>
        <Button variant="primary" onClick={onCreate}>
          <Plus size={15} />
          {text(...translatedMessage("create_key_06433c30b0"))}
        </Button>
      </header>
      {!keys.length ? (
        <div className="key-list-empty">
          <KeyRound size={28} />
          <h2>{text(...translatedMessage("no_keys_yet_0872a1eecd"))}</h2>
          <p>
            {text(
              ...translatedMessage(
                "create_a_key_then_select_your_model_in_your_c_d586e1860e",
              ),
            )}
          </p>
          <Button onClick={onCreate}>
            {text(...translatedMessage("create_key_06433c30b0"))}
          </Button>
        </div>
      ) : (
        <div
          className="key-list-table"
          role="table"
          aria-label={text(...translatedMessage("api_key_list_d9208d98a8"))}
        >
          <div className="key-list-head" role="row">
            {[
              text(...translatedMessage("key_64846eb2df")),
              text(...translatedMessage("service_group_1c6f8adf41")),
              text(...translatedMessage("status_dda39dcd63")),
              text(...translatedMessage("spending_limit_0562b63d71")),
              text(...translatedMessage("actions_d83f4ed14a")),
            ].map((label) => (
              <div key={label} role="columnheader">
                {label}
              </div>
            ))}
          </div>
          {pagination.rows.map((item) => {
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
                                ...translatedMessage(
                                  "hide_key_for_65de012616",
                                  { p0: item.name },
                                ),
                              )
                            : text(
                                ...translatedMessage(
                                  "reveal_key_for_3b07c078fc",
                                  { p0: item.name },
                                ),
                              )
                        }
                        title={
                          visible
                            ? text(...translatedMessage("hide_key_c69295abde"))
                            : text(
                                ...translatedMessage(
                                  "verify_to_reveal_6c27be333c",
                                ),
                              )
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
                          ...translatedMessage("copy_key_for_8ef91d257e", {
                            p0: item.name,
                          }),
                        )}
                        title={text(
                          ...translatedMessage("copy_key_eb45bf5c2d"),
                        )}
                        onClick={() => onSecret(item, "copy")}
                      >
                        <Copy size={14} />
                      </button>
                    </div>
                  </div>
                </div>
                <div className="key-list-group" role="cell">
                  <span className="key-mobile-label">
                    {text(...translatedMessage("service_group_1c6f8adf41"))}
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
                        text(...translatedMessage("unlimited_bb98361525"))
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
                        ...translatedMessage(
                          "spending_limit_used_for_f394c828f7",
                          { p0: item.name },
                        ),
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
                    {text(...translatedMessage("connect_81f76afa98"))}
                  </Button>
                  <button
                    className="key-edit-action"
                    type="button"
                    data-key-action="edit"
                    onClick={() => onEdit(item)}
                  >
                    {text(...translatedMessage("edit_6224671192"))}
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
      <ListPagination {...pagination} />
      <p className="key-list-footnote">
        {text(
          ...translatedMessage(
            "keys_spending_amounts_are_simulated_f9b8828c47",
            { p0: keys.length },
          ),
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
      ? new Intl.DateTimeFormat(regionalLocale(i18n.language), {
          dateStyle: "medium",
          timeStyle: "short",
          timeZone: item.quotaTimezone || "UTC",
        }).format(new Date(value)) + ` (${item.quotaTimezone || "UTC"})`
      : text(...translatedMessage("not_recorded_0407708992"));
  return (
    <div className="key-detail" data-key-detail={item.id}>
      <section>
        <h3>{text(...translatedMessage("basic_information_c28cfb4d8c"))}</h3>
        <dl>
          <div>
            <dt>{text(...translatedMessage("key_name_c60bdc543a"))}</dt>
            <dd>{item.name}</dd>
          </div>
          <div>
            <dt>{text(...translatedMessage("key_id_db5f419032"))}</dt>
            <dd>
              <code dir="ltr">{item.id}</code>
            </dd>
          </div>
          <div>
            <dt>{text(...translatedMessage("service_group_1c6f8adf41"))}</dt>
            <dd>
              <TierBadge id={item.tier} />
            </dd>
          </div>
          <div>
            <dt>{text(...translatedMessage("status_dda39dcd63"))}</dt>
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
            <dt>{text(...translatedMessage("last_used_42a34272e0"))}</dt>
            <dd>{date(item.lastUsedAt)}</dd>
          </div>
          <div>
            <dt>{text(...translatedMessage("expires_4c2b1bc79c"))}</dt>
            <dd>
              {item.validUntil
                ? date(item.validUntil)
                : text(...translatedMessage("no_expiry_5f6d3fb5e8"))}
            </dd>
          </div>
        </dl>
        <p className="key-detail-hint">
          {text(
            ...translatedMessage("times_are_shown_in_dubai_time_716dc35780"),
          )}
        </p>
      </section>
      <section>
        <h3>{text(...translatedMessage("spending_limit_0562b63d71"))}</h3>
        <dl>
          <div>
            <dt>{text(...translatedMessage("used_this_period_d49ac65202"))}</dt>
            <dd dir="ltr">{money(quota.usedNano)}</dd>
          </div>
          <div>
            <dt>{text(...translatedMessage("limit_8ca8e48322"))}</dt>
            <dd>
              {quota.limitNano === null ? (
                text(...translatedMessage("unlimited_3db9664741"))
              ) : (
                <bdi dir="ltr">{money(quota.limitNano)}</bdi>
              )}
            </dd>
          </div>
          {quota.remainingNano !== null && (
            <div>
              <dt>{text(...translatedMessage("remaining_735d237b85"))}</dt>
              <dd dir="ltr">{money(quota.remainingNano)}</dd>
            </div>
          )}
          <div>
            <dt>{text(...translatedMessage("quota_period_23b646d127"))}</dt>
            <dd>{periodName(quota.period, text)}</dd>
          </div>
          <div>
            <dt>
              {text(...translatedMessage("lifetime_spending_d3b58ade21"))}
            </dt>
            <dd dir="ltr">
              {money(item.totalUsed_nano ?? item.currentPeriodUsed_nano ?? "0")}
            </dd>
          </div>
        </dl>
        <p className="key-detail-hint">
          {text(
            ...translatedMessage(
              "totals_reflect_simulated_requests_periods_use_4b4d8c03c0",
            ),
          )}
        </p>
      </section>
      <section>
        <h3>{text(...translatedMessage("model_access_dedddbaa09"))}</h3>
        <dl>
          <div>
            <dt>{text(...translatedMessage("access_scope_1f11112dac"))}</dt>
            <dd>
              {item.modelAccess === "all"
                ? text(...translatedMessage("all_group_models_d6e225abd3"))
                : text(...translatedMessage("model_allowlist_3a57ae93e3"))}
            </dd>
          </div>
          <div>
            <dt>
              {text(...translatedMessage("direct_model_requests_dbb0db748c"))}
            </dt>
            <dd>
              {allowsSpecifiedModels(item)
                ? text(...translatedMessage("allowed_48c0aa47dc"))
                : text(...translatedMessage("not_allowed_a92c42fbae"))}
            </dd>
          </div>
          <div>
            <dt>{text(...translatedMessage("smart_routing_c4f3fd8e0e"))}</dt>
            <dd>
              {allowsSmartModel(item)
                ? text(
                    ...translatedMessage(
                      "allowed_within_this_scope_51483c14ba",
                    ),
                  )
                : text(...translatedMessage("not_allowed_a92c42fbae"))}
            </dd>
          </div>
        </dl>
        {item.smartAccess === undefined && (
          <p className="key-detail-legacy">
            {text(
              ...translatedMessage(
                "legacy_access_is_preserved_model_and_routing__1d8e9aa23b",
              ),
            )}
          </p>
        )}
        <h4 className="key-detail-model-heading">
          {allowsSpecifiedModels(item)
            ? text(...translatedMessage("authorized_models_c194b309e9"))
            : text(...translatedMessage("routing_candidates_5eef0ab025"))}
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
                    ...translatedMessage("unavailable_in_group_396ca621b8"),
                  )}
                </small>
              )}
            </li>
          ))}
        </ul>
        {!modelIds.length && (
          <p className="key-detail-hint">
            {text(
              ...translatedMessage(
                "no_authorized_models_are_currently_available_0f3b30d87b",
              ),
            )}
          </p>
        )}
      </section>
      <section>
        <h3>{text(...translatedMessage("ip_restrictions_4288eddb2a"))}</h3>
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
            {text(...translatedMessage("no_ip_restrictions_set_68f208188a"))}
          </p>
        )}
      </section>
      <Button onClick={() => onEdit(item)}>
        {text(...translatedMessage("edit_key_settings_7f830d4f88"))}
      </Button>
    </div>
  );
}
