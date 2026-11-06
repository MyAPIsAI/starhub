import { message as translatedMessage } from "../i18n/messages";

import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  ArrowRight,
  ArrowLeft,
  Check,
  CheckCircle2,
  ChevronDown,
  Clock3,
  CreditCard,
  Info,
  RotateCcw,
  ShieldCheck,
  TriangleAlert,
  Wallet,
  XCircle,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { usePreferences } from "../lib/preferences";
import {
  sessionUsername,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { fundingSourceLabel } from "../lib/funding-source";
import { validateRechargeAmount } from "../lib/billing";
import { formatMoney } from "../lib/money";
import { placeholders } from "../config/placeholders";
import {
  useWallet,
  useRechargeOrders,
  useRechargeOrder,
  useCreateRechargeOrder,
  useBeginRechargePayment,
  useResolveRechargeOrder,
  adaptRechargeOrder,
} from "../lib/api/billing";
import type { RechargeOrder } from "../lib/billing";
import "../components/recharge-flow.css";

const messages = {
  fundingSource: translatedMessage("funding_source_355341b128"),
  quotaConversion: translatedMessage("api_quota_conversion_78f6b675e2"),
  conversionUnavailable: translatedMessage("currently_unavailable_fff1a9e4ff"),
  title: translatedMessage("add_account_credit_153675939d"),
  intro: translatedMessage(
    "add_funds_to_this_account_s_prepaid_balance_b2d5bfa20b",
  ),
  orderTitle: translatedMessage("your_recharge_order_8ffcb4b96e"),
  orderIntro: translatedMessage(
    "your_order_is_saved_credit_is_added_only_afte_4f68eaff2f",
  ),
  cashierTitle: translatedMessage("demo_checkout_b416dec071"),
  cashierIntro: translatedMessage(
    "choose_a_payment_result_to_explore_the_rechar_47d2ba9f6b",
  ),
  successTitle: translatedMessage("credit_added_ae67aa3a6b"),
  successIntro: translatedMessage(
    "the_recharge_amount_is_now_in_this_account_s__4e91c024ee",
  ),
  incompleteTitle: translatedMessage("recharge_not_completed_01dee5f0bd"),
  incompleteIntro: translatedMessage(
    "this_order_has_not_added_credit_you_can_creat_798f5f58fe",
  ),
  amountStep: translatedMessage("amount_643d19b6a7"),
  paymentStep: translatedMessage("payment_75233d5697"),
  creditedStep: translatedMessage("credited_f63b00798e"),
  currentBalance: translatedMessage("available_balance_ff3b5421e1"),
  amountLabel: translatedMessage("recharge_amount_7ae3994595"),
  customLabel: translatedMessage("or_enter_another_amount_2be11f8616"),
  amountHint: translatedMessage(
    "10_10_000_per_order_with_up_to_2_decimal_plac_8fb4bca206",
  ),
  required: translatedMessage("enter_an_amount_a8a78dc94e"),
  format: translatedMessage(
    "enter_a_valid_amount_with_up_to_2_decimal_pla_e2f1177c70",
  ),
  min: translatedMessage("the_minimum_recharge_is_10_1c8fc2f256"),
  max: translatedMessage("the_maximum_recharge_is_10_000_6993d15e25"),
  channel: translatedMessage("payment_method_3283479ba3"),
  card: translatedMessage("bank_card_31663ab079"),
  demoChannel: translatedMessage("demo_channel_8385cffea9"),
  cardHint: translatedMessage(
    "complete_payment_in_the_local_demo_checkout_786ffb553d",
  ),
  creditedAmount: translatedMessage("amount_credited_db2452b30c"),
  expectedCredit: translatedMessage("expected_credit_a4b944f3ce"),
  fee: translatedMessage("additional_fees_0c8dc29ba3"),
  noFee: translatedMessage("none_fbfe0e70fb"),
  payable: translatedMessage("payment_amount_fa6b120cbf"),
  equalAmount: translatedMessage(
    "the_full_payment_amount_is_added_to_your_prep_d03a15cb44",
  ),
  demoNote: translatedMessage(
    "frontend_demo_no_real_card_details_or_charges_9714a40312",
  ),
  createOrder: translatedMessage("confirm_and_create_order_aa644e900b"),
  createHint: translatedMessage(
    "orders_are_valid_for_30_minutes_after_confirm_e708b944e8",
  ),
  cancel: translatedMessage("cancel_0828e4ce08"),
  close: translatedMessage("done_981dc3f35b"),
  later: translatedMessage("pay_later_52e3325a6d"),
  continueLater: translatedMessage("continue_later_696f80a96d"),
  goCheckout: translatedMessage("open_demo_checkout_63f5982683"),
  resumeCheckout: translatedMessage("resume_demo_payment_3fe17ba600"),
  backOrder: translatedMessage("back_to_order_e58d7f0d90"),
  pending: translatedMessage("awaiting_payment_4166815c6e"),
  processing: translatedMessage("awaiting_confirmation_48c279a77f"),
  paid: translatedMessage("credited_c144ccc983"),
  failed: translatedMessage("payment_failed_54d80406b3"),
  cancelled: translatedMessage("payment_cancelled_a6229977a7"),
  expired: translatedMessage("order_expired_e92ccb18be"),
  disputed: translatedMessage("order_under_review_7da2006df6"),
  orderNumber: translatedMessage("recharge_order_a56ed9c63f"),
  account: translatedMessage("credit_destination_8cfb51f5c2"),
  prepaid: translatedMessage("account_prepaid_balance_6f04be23bb"),
  validUntil: translatedMessage("valid_until_6f24240fab"),
  remaining: translatedMessage("time_left_to_pay_303434e702"),
  keepOrder: translatedMessage(
    "closing_keeps_your_order_resume_it_from_recha_e0473b8cff",
  ),
  resultTitle: translatedMessage("choose_a_demo_payment_result_22c7c9054f"),
  resultHint: translatedMessage(
    "this_simulates_card_payment_no_card_details_a_c639c76794",
  ),
  simulateSuccess: translatedMessage("simulate_success_8af25344dd"),
  simulateFailure: translatedMessage("simulate_failure_1877882c62"),
  simulateCancel: translatedMessage("simulate_cancellation_eceb26bace"),
  actualCredited: translatedMessage("credit_added_by_this_order_2fe84c2811"),
  latestBalance: translatedMessage("current_account_balance_78fdf2d6d5"),
  viewRecords: translatedMessage("view_recharge_records_dc17345445"),
  newOrder: translatedMessage("start_a_new_recharge_4b7995ef91"),
  failedReason: translatedMessage(
    "the_demo_payment_failed_and_no_credit_was_add_ec76461b3e",
  ),
  cancelledReason: translatedMessage(
    "you_cancelled_this_demo_payment_no_credit_was_db52813af3",
  ),
  expiredReason: translatedMessage(
    "the_30_minute_payment_window_has_ended_create_e07aa48207",
  ),
  disputedReason: translatedMessage(
    "this_order_needs_review_check_its_status_in_r_f98478a086",
  ),
  missing: translatedMessage("recharge_order_not_found_32f6f3ac8e"),
  missingHint: translatedMessage(
    "demo_data_may_have_been_reset_view_records_or_d3933e6c62",
  ),
  genericError: translatedMessage(
    "this_action_could_not_be_completed_check_the__fb65d28c54",
  ),
  notPayable: translatedMessage(
    "this_order_can_no_longer_accept_payment_check_71c55d7551",
  ),
  latePayment: translatedMessage(
    "payment_was_confirmed_after_expiry_the_full_a_e162ba4d41",
  ),
  scenarios: translatedMessage("demo_scenarios_740afd2ade"),
  scenariosHint: translatedMessage(
    "for_checking_the_local_demo_only_no_real_paym_5da8eb024e",
  ),
  simulateExpiry: translatedMessage("simulate_expiry_3711227cb7"),
  repeatSuccess: translatedMessage("repeat_success_notification_250f4d51c5"),
  lateSuccess: translatedMessage("simulate_late_payment_78b479bbc6"),
  repeated: translatedMessage(
    "this_order_was_already_credited_the_repeated__5da4629495",
  ),
  expiredDemo: translatedMessage("order_expiry_simulated_e143ad30a8"),
  expiryHint: translatedMessage(
    "simulating_expiry_advances_demo_time_other_or_c38e075abf",
  ),
} as const;

function RechargePageLayout({
  title,
  description,
  children,
  footer,
  onExit,
}: {
  title: string;
  description: string;
  children: ReactNode;
  footer: ReactNode;
  onExit: () => void;
}) {
  const language = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  return (
    <section className="rf-page">
      <button className="rf-page-back" onClick={onExit}>
        <ArrowLeft size={16} className="directional" />
        {text(...translatedMessage("back_146b4da468"))}
      </button>
      <header className="rf-page-heading">
        <div>
          <span className="rf-page-eyebrow">
            <Wallet size={14} />
            {text(...translatedMessage("account_billing_2dc8658792"))}
          </span>
          <h1>{title}</h1>
          <p>{description}</p>
        </div>
        <span className="rf-page-currency">
          USD
          <span>
            {text(...translatedMessage("usd_prepaid_account_ed1e591a0a"))}
          </span>
        </span>
      </header>
      <div className="rf-page-card">
        <div className="rf-page-body">{children}</div>
        <div className="rf-page-footer">{footer}</div>
      </div>
      <p className="rf-page-footnote">
        <ShieldCheck size={14} />
        {text(
          ...translatedMessage(
            "interactive_demo_no_real_charges_saved_orders_a1ecace918",
          ),
        )}
      </p>
    </section>
  );
}

type RechargeFlowProps = {
  onExit: () => void;
  onOrderChange?: (id?: string) => void;
  orderId?: string;
  onViewRecords?: (orderId?: string) => void;
};

export default function RechargeFlow(props: RechargeFlowProps) {
  const identity = useSessionQueryIdentity();
  return <RechargeFlowView key={JSON.stringify(identity)} {...props} />;
}

function RechargeFlowView({
  onExit,
  onOrderChange,
  orderId,
  onViewRecords,
}: RechargeFlowProps) {
  const language = usePreferences((state) => state.language);
  const t = (key: keyof typeof messages) =>
    messages[key][language === "zh" ? 0 : language === "ar" ? 2 : 1];
  // Wallet amounts come from the owner-specific endpoint, never a ledger row.
  const walletQuery = useWallet();
  const ordersQuery = useRechargeOrders();
  const createOrderMut = useCreateRechargeOrder();
  const beginPaymentMut = useBeginRechargePayment();
  const resolveOrderMut = useResolveRechargeOrder();
  const accountName =
    useSystemAccess((state) => sessionUsername(state.session)) ?? "—";
  const backendOrders = (ordersQuery.data ?? []).map(adaptRechargeOrder);
  const balance = walletQuery.error
    ? undefined
    : walletQuery.data?.balance_nano;
  const orders: RechargeOrder[] = backendOrders as unknown as RechargeOrder[];
  const [activeId, setActiveId] = useState<string | null>(orderId ?? null);
  // Precise single-order polling: when an order is active and waiting,
  // poll GET /api/recharge/{id} every 5s for status updates instead of
  // refetching the entire order list.
  const activeNumericId = activeId ? Number(activeId) : undefined;
  const singleOrderQuery = useRechargeOrder(activeNumericId);
  const polledOrder = singleOrderQuery.data
    ? adaptRechargeOrder(singleOrderQuery.data)
    : undefined;
  const [amount, setAmount] = useState("100");
  const [attempted, setAttempted] = useState(false);
  const [cashier, setCashier] = useState(false);
  const [actionError, setActionError] = useState<keyof typeof messages | null>(
    null,
  );
  const [notice, setNotice] = useState<"repeated" | "expiredDemo" | null>(null);
  const [now, setNow] = useState(Date.now);
  const amountInput = useRef<HTMLInputElement>(null);
  const stagePanel = useRef<HTMLDivElement>(null);
  const previousStage = useRef("");
  const submitted = useRef(false);
  const formId = useId();
  const order = polledOrder ?? orders.find((item) => item.id === activeId);
  const validAmount = validateRechargeAmount(amount);
  const isWaiting =
    order?.status === "pending" || order?.status === "processing";
  const isPaid = order?.status === "paid";
  const isSetup = activeId === null;
  const expires = order ? Date.parse(order.expires_at) : 0;
  const remainingSeconds = Math.max(0, Math.ceil((expires - now) / 1000));
  const countdown = `${Math.floor(remainingSeconds / 60)
    .toString()
    .padStart(2, "0")}:${(remainingSeconds % 60).toString().padStart(2, "0")}`;

  useEffect(() => {
    setActiveId(orderId ?? null);
    setCashier(false);
    setActionError(null);
    setNotice(null);
    setAttempted(false);
    setNow(Date.now());
    submitted.current = false;
  }, [orderId]);

  useEffect(() => {
    if (!isWaiting) return;
    const tick = () => {
      setNow(Date.now());
    };
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [isWaiting, activeId]);

  useLayoutEffect(() => {
    const next = !activeId
      ? "amount"
      : `${activeId}:${order?.status}:${cashier}`;
    if (previousStage.current !== next) {
      if (previousStage.current) {
        if (!activeId) amountInput.current?.focus({ preventScroll: true });
        else stagePanel.current?.focus({ preventScroll: true });
      }
      window.scrollTo({ top: 0, behavior: "instant" });
    }
    previousStage.current = next;
  }, [activeId, order?.status, cashier]);

  const displayError = (error: string) => {
    setActionError(
      error === "required" ||
        error === "format" ||
        error === "min" ||
        error === "max"
        ? error
        : error.includes("expired")
          ? "expiredReason"
          : error.includes("not_found")
            ? "missingHint"
            : error === "order_not_payable" || error === "new_order_required"
              ? "notPayable"
              : "genericError",
    );
  };
  const confirmAmount = async () => {
    setAttempted(true);
    setActionError(null);
    if (!validAmount.ok) {
      amountInput.current?.focus();
      return;
    }
    if (submitted.current) return;
    submitted.current = true;
    createOrderMut.mutate(
      { amount_usd: amount, channel: "card" },
      {
        onSuccess: (order) => {
          const adapted = adaptRechargeOrder(order);
          setNow(Date.now());
          setActiveId(adapted.id);
          onOrderChange?.(adapted.id);
          setCashier(false);
          setNotice(null);
        },
        onError: (err) => {
          submitted.current = false;
          displayError(err instanceof Error ? err.message : String(err));
        },
      },
    );
  };
  const checkout = async () => {
    if (!order) return;
    setActionError(null);
    setNotice(null);
    beginPaymentMut.mutate(Number(order.id), {
      onSuccess: () => {
        setCashier(true);
      },
      onError: (err) => {
        displayError(err instanceof Error ? err.message : String(err));
      },
    });
  };
  const resolve = (
    outcome: "success" | "failed" | "cancelled",
    _late = false,
  ) => {
    if (!order) return;
    setActionError(null);
    setNotice(null);
    resolveOrderMut.mutate(
      { id: Number(order.id), outcome },
      {
        onSuccess: (order) => {
          setCashier(false);
          if (order.status === "paid") {
            // Check if it was already paid before this resolve.
            const previous = backendOrders.find(
              (o) => o.id === String(order.id),
            );
            if (previous?.status === "paid") setNotice("repeated");
          }
        },
        onError: (err) => {
          displayError(err instanceof Error ? err.message : String(err));
        },
      },
    );
  };
  const startAgain = () => {
    if (order) setAmount(order.amount_usd);
    setActiveId(null);
    onOrderChange?.(undefined);
    setCashier(false);
    setActionError(null);
    setNotice(null);
    setAttempted(false);
    submitted.current = false;
  };
  const viewRecords = () => {
    if (onViewRecords) onViewRecords(activeId ?? undefined);
    else onExit();
  };
  const formatDate = (value: string) =>
    new Intl.DateTimeFormat(
      language === "zh"
        ? "zh-CN"
        : language === "ar"
          ? "ar-AE-u-nu-latn"
          : "en-GB",
      {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        timeZoneName: "short",
        timeZone: placeholders.region.timeZone,
      },
    ).format(new Date(value));
  const displayedAmount = isSetup
    ? validAmount.ok
      ? formatMoney(validAmount.amount_nano)
      : "—"
    : order
      ? formatMoney(order.amount_nano)
      : "—";
  const stage = isSetup ? 0 : isPaid ? 2 : 1;
  const resultReason =
    order?.status === "expired"
      ? "expiredReason"
      : order?.status === "cancelled"
        ? "cancelledReason"
        : order?.status === "disputed"
          ? "disputedReason"
          : "failedReason";
  const title = isSetup
    ? "title"
    : !order
      ? "missing"
      : order.status === "disputed"
        ? "disputed"
        : isPaid
          ? "successTitle"
          : isWaiting
            ? cashier
              ? "cashierTitle"
              : "orderTitle"
            : "incompleteTitle";
  const description = isSetup
    ? "intro"
    : !order
      ? "missingHint"
      : order.status === "disputed"
        ? "disputedReason"
        : isPaid
          ? "successIntro"
          : isWaiting
            ? cashier
              ? "cashierIntro"
              : "orderIntro"
            : "incompleteIntro";

  return (
    <RechargePageLayout
      onExit={onExit}
      title={t(title)}
      description={t(description)}
      footer={
        <div className="rf-footer">
          {isSetup ? (
            <>
              <Button onClick={onExit}>{t("cancel")}</Button>
              <Button variant="primary" type="submit" form={formId}>
                {t("createOrder")}
                <ArrowRight className="directional" size={15} />
              </Button>
            </>
          ) : isPaid ? (
            <>
              <Button onClick={onExit}>{t("close")}</Button>
              <Button variant="primary" onClick={viewRecords}>
                {t("viewRecords")}
                <ArrowRight className="directional" size={15} />
              </Button>
            </>
          ) : isWaiting ? (
            <>
              <Button onClick={onExit}>
                {t(cashier ? "continueLater" : "later")}
              </Button>
              <Button
                variant={cashier ? "secondary" : "primary"}
                onClick={cashier ? () => setCashier(false) : checkout}
              >
                {t(
                  cashier
                    ? "backOrder"
                    : order.status === "processing"
                      ? "resumeCheckout"
                      : "goCheckout",
                )}
                {!cashier && <ArrowRight className="directional" size={15} />}
              </Button>
            </>
          ) : (
            <>
              <Button onClick={viewRecords}>{t("viewRecords")}</Button>
              <Button
                variant="primary"
                onClick={(event) => {
                  // The next stage reuses this slot for a submit button.
                  event.preventDefault();
                  startAgain();
                }}
              >
                <RotateCcw size={15} />
                {t("newOrder")}
              </Button>
            </>
          )}
        </div>
      }
    >
      <div
        ref={stagePanel}
        tabIndex={-1}
        aria-label={t(title)}
        className="rf-flow"
        data-testid="recharge-flow"
        data-stage={isSetup ? "amount" : (order?.status ?? "missing")}
      >
        <ol className="rf-steps" aria-label={t("title")}>
          {(["amountStep", "paymentStep", "creditedStep"] as const).map(
            (key, index) => (
              <li
                key={key}
                className={
                  index === stage
                    ? "is-current"
                    : index < stage
                      ? "is-complete"
                      : ""
                }
                aria-current={index === stage ? "step" : undefined}
              >
                <span>{index < stage ? <Check size={12} /> : index + 1}</span>
                {t(key)}
              </li>
            ),
          )}
        </ol>
        {actionError && (
          <p className="rf-message is-error" role="alert">
            <TriangleAlert size={16} />
            <span>{t(actionError)}</span>
          </p>
        )}
        {notice && (
          <p className="rf-message" role="status">
            <Info size={16} />
            <span>{t(notice)}</span>
          </p>
        )}

        {isSetup ? (
          <form
            id={formId}
            className="rf-form"
            onSubmit={(event) => {
              event.preventDefault();
              confirmAmount();
            }}
            noValidate
          >
            <div className="rf-entry-fields">
              <fieldset className="rf-amounts">
                <legend>{t("amountLabel")}</legend>
                <div className="rf-presets">
                  {[50, 100, 500, 1000].map((value) => (
                    <button
                      type="button"
                      key={value}
                      aria-pressed={
                        validAmount.ok &&
                        Number(validAmount.amount_usd) === value
                      }
                      onClick={() => {
                        setAmount(String(value));
                        setAttempted(false);
                        setActionError(null);
                      }}
                    >
                      <bdi>${value.toLocaleString("en-US")}</bdi>
                      {validAmount.ok &&
                        Number(validAmount.amount_usd) === value && (
                          <Check size={14} />
                        )}
                    </button>
                  ))}
                </div>
              </fieldset>
              <div className="rf-amount-field">
                <label htmlFor={`${formId}-amount`}>{t("customLabel")}</label>
                <div
                  className={`rf-input-wrap${attempted && !validAmount.ok ? " is-invalid" : ""}`}
                >
                  <span aria-hidden="true">$</span>
                  <input
                    ref={amountInput}
                    id={`${formId}-amount`}
                    type="text"
                    inputMode="decimal"
                    autoComplete="off"
                    value={amount}
                    onChange={(event) => {
                      setAmount(event.target.value);
                      setActionError(null);
                    }}
                    aria-invalid={attempted && !validAmount.ok}
                    aria-describedby={`${formId}-amount-help`}
                  />
                </div>
                <p
                  id={`${formId}-amount-help`}
                  className={
                    attempted && !validAmount.ok ? "rf-field-error" : "rf-hint"
                  }
                  role={attempted && !validAmount.ok ? "alert" : undefined}
                >
                  {attempted && !validAmount.ok
                    ? t(validAmount.error)
                    : t("amountHint")}
                </p>
              </div>
              <fieldset className="rf-payment-method">
                <legend>{t("channel")}</legend>
                <label>
                  <input
                    type="radio"
                    name={`${formId}-channel`}
                    value="card"
                    checked
                    readOnly
                  />
                  <span className="rf-card-symbol">
                    <CreditCard size={21} />
                  </span>
                  <span>
                    <strong>
                      {t("card")}
                      <small>{t("demoChannel")}</small>
                    </strong>
                    <span>{t("cardHint")}</span>
                  </span>
                </label>
              </fieldset>
            </div>
            <aside className="rf-summary">
              <div className="rf-balance">
                <span>
                  <Wallet size={16} />
                  {t("currentBalance")}
                </span>
                <strong dir="ltr">
                  {balance === undefined ? "—" : formatMoney(balance)}
                </strong>
              </div>
              <div className="rf-cost-summary">
                <dl>
                  <div>
                    <dt>{t("creditedAmount")}</dt>
                    <dd dir="ltr">{displayedAmount}</dd>
                  </div>
                  <div>
                    <dt>{t("fee")}</dt>
                    <dd>{t("noFee")}</dd>
                  </div>
                  <div className="rf-total">
                    <dt>{t("payable")}</dt>
                    <dd dir="ltr">{displayedAmount}</dd>
                  </div>
                </dl>
                <p>{t("equalAmount")}</p>
              </div>
              <p className="rf-safe-note">
                <ShieldCheck size={15} />
                <span>{t("demoNote")}</span>
              </p>
              <p className="rf-hint rf-center">{t("createHint")}</p>
            </aside>
          </form>
        ) : !order ? (
          <div className="rf-result">
            <span className="rf-result-symbol is-failed">
              <Info size={28} />
            </span>
            <h3>{t("missing")}</h3>
            <p>{t("missingHint")}</p>
          </div>
        ) : (
          <div className="rf-order-stage">
            <div className="rf-order-primary">
              {isPaid ? (
                <div className="rf-result is-success">
                  <span className="rf-result-symbol">
                    <CheckCircle2 size={31} />
                  </span>
                  <span className="rf-result-label">{t("actualCredited")}</span>
                  <strong className="rf-credited" dir="ltr">
                    {displayedAmount}
                  </strong>
                  <p>{t("successIntro")}</p>
                </div>
              ) : !isWaiting ? (
                <div className="rf-result">
                  <span className={`rf-result-symbol is-${order.status}`}>
                    {order.status === "expired" ? (
                      <Clock3 size={29} />
                    ) : (
                      <XCircle size={29} />
                    )}
                  </span>
                  <h3>{t(order.status)}</h3>
                  <p>{t(resultReason)}</p>
                </div>
              ) : (
                <div className="rf-order-hero">
                  <span className="rf-state-pill">
                    <span />
                    {t(order.status)}
                  </span>
                  <strong dir="ltr">{displayedAmount}</strong>
                  <span>{t("payable")}</span>
                  <div
                    className="rf-countdown"
                    role="timer"
                    aria-label={`${t("remaining")} ${countdown}`}
                  >
                    <Clock3 size={14} />
                    <span>{t("remaining")}</span>
                    <bdi>{countdown}</bdi>
                  </div>
                </div>
              )}
              {isPaid && (
                <div className="rf-balance rf-receipt-balance">
                  <span>
                    <Wallet size={16} />
                    {t("latestBalance")}
                  </span>
                  <strong dir="ltr">
                    {balance === undefined ? "—" : formatMoney(balance)}
                  </strong>
                </div>
              )}
              {isPaid && order.late_payment && (
                <p className="rf-message">
                  <Info size={16} />
                  <span>{t("latePayment")}</span>
                </p>
              )}
              {isWaiting && cashier && (
                <section
                  className="rf-cashier"
                  aria-labelledby={`${formId}-cashier-title`}
                >
                  <div className="rf-cashier-heading">
                    <span className="rf-card-symbol">
                      <CreditCard size={22} />
                    </span>
                    <div>
                      <h3 id={`${formId}-cashier-title`}>{t("resultTitle")}</h3>
                      <p>{t("resultHint")}</p>
                    </div>
                  </div>
                  <div className="rf-cashier-actions">
                    <Button
                      variant="primary"
                      onClick={() => resolve("success")}
                    >
                      <CheckCircle2 size={16} />
                      {t("simulateSuccess")}
                    </Button>
                    <Button onClick={() => resolve("failed")}>
                      <XCircle size={16} />
                      {t("simulateFailure")}
                    </Button>
                    <Button
                      variant="ghost"
                      onClick={() => resolve("cancelled")}
                    >
                      {t("simulateCancel")}
                    </Button>
                  </div>
                </section>
              )}
            </div>
            <aside className="rf-order-aside">
              <dl className="rf-order-details">
                <div>
                  <dt>{t("orderNumber")}</dt>
                  <dd>
                    <bdi>{order.id}</bdi>
                  </dd>
                </div>
                <div>
                  <dt>{t("account")}</dt>
                  <dd>
                    <strong>{accountName}</strong>
                    <span>{t("prepaid")}</span>
                  </dd>
                </div>
                <div>
                  <dt>{t(isPaid ? "creditedAmount" : "expectedCredit")}</dt>
                  <dd dir="ltr">{displayedAmount}</dd>
                </div>
                <div>
                  <dt>{t("channel")}</dt>
                  <dd>
                    {t("card")}
                    <span>{t("demoChannel")}</span>
                  </dd>
                </div>
                {isWaiting && (
                  <div>
                    <dt>{t("validUntil")}</dt>
                    <dd>
                      <bdi>{formatDate(order.expires_at)}</bdi>
                    </dd>
                  </div>
                )}
              </dl>
              <dl className="rf-order-details" data-funding-source>
                <div>
                  <dt>{t("fundingSource")}</dt>
                  <dd>
                    {fundingSourceLabel(order.funding_source, (zh, en, ar) =>
                      language === "zh" ? zh : language === "ar" ? ar : en,
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t("quotaConversion")}</dt>
                  <dd>{t("conversionUnavailable")}</dd>
                </div>
              </dl>
              {isWaiting && (
                <p className="rf-safe-note">
                  <Clock3 size={15} />
                  <span>{t("keepOrder")}</span>
                </p>
              )}
              <p className="rf-safe-note">
                <ShieldCheck size={15} />
                <span>{t("demoNote")}</span>
              </p>
              <details className="rf-demo-scenarios">
                <summary>
                  {t("scenarios")}
                  <ChevronDown size={15} />
                </summary>
                <div>
                  <p>{t("scenariosHint")}</p>
                  <div className="rf-demo-actions">
                    <button
                      type="button"
                      disabled={!isWaiting}
                      onClick={() => {
                        setNow(expires + 1000);
                        setCashier(false);
                        setActionError(null);
                        setNotice("expiredDemo");
                      }}
                    >
                      {t("simulateExpiry")}
                    </button>
                    <button
                      type="button"
                      disabled={!isPaid}
                      onClick={() => resolve("success")}
                    >
                      {t("repeatSuccess")}
                    </button>
                    <button
                      type="button"
                      disabled={order.status !== "expired"}
                      onClick={() => resolve("success", true)}
                    >
                      {t("lateSuccess")}
                    </button>
                  </div>
                  {isWaiting && <p className="rf-hint">{t("expiryHint")}</p>}
                </div>
              </details>
            </aside>
          </div>
        )}
      </div>
    </RechargePageLayout>
  );
}
