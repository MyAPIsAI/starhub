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
import { useProductPreview } from "../lib/product-preview";
import { accountDisplayName } from "../lib/account-identity";
import { useSessionQueryIdentity } from "../lib/system-access";
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
  fundingSource: ["资金来源", "Funding source", "مصدر الأموال"],
  quotaConversion: ["API 配额兑换", "API quota conversion", "تحويل رصيد API"],
  conversionUnavailable: [
    "当前不可兑换",
    "Currently unavailable",
    "غير متاح حاليًا",
  ],
  title: ["账户充值", "Add account credit", "شحن الحساب"],
  intro: [
    "为当前账号的预付费余额充值。",
    "Add funds to this account’s prepaid balance.",
    "أضف رصيدًا مسبق الدفع إلى الحساب الحالي.",
  ],
  orderTitle: ["确认充值订单", "Your recharge order", "طلب شحن الرصيد"],
  orderIntro: [
    "订单已保留，完成支付后才会入账。",
    "Your order is saved. Credit is added only after payment succeeds.",
    "تم حفظ الطلب. يُضاف الرصيد بعد نجاح الدفع فقط.",
  ],
  cashierTitle: ["演示收银台", "Demo checkout", "صفحة دفع تجريبية"],
  cashierIntro: [
    "选择一个支付结果，体验完整的充值流程。",
    "Choose a payment result to explore the recharge flow.",
    "اختر نتيجة دفع لتجربة عملية الشحن.",
  ],
  successTitle: ["充值成功", "Credit added", "تمت إضافة الرصيد"],
  successIntro: [
    "充值金额已加入当前账号余额。",
    "The recharge amount is now in this account’s balance.",
    "أُضيف مبلغ الشحن إلى رصيد الحساب.",
  ],
  incompleteTitle: ["充值未完成", "Recharge not completed", "لم يكتمل الشحن"],
  incompleteIntro: [
    "此次订单未入账，你可以重新创建充值订单。",
    "This order has not added credit. You can create a new order.",
    "لم يُضف هذا الطلب رصيدًا. يمكنك إنشاء طلب جديد.",
  ],
  amountStep: ["选择金额", "Amount", "المبلغ"],
  paymentStep: ["确认支付", "Payment", "الدفع"],
  creditedStep: ["余额到账", "Credited", "إضافة الرصيد"],
  currentBalance: ["当前可用余额", "Available balance", "الرصيد المتاح"],
  amountLabel: ["充值金额", "Recharge amount", "مبلغ الشحن"],
  customLabel: [
    "或输入其他金额",
    "Or enter another amount",
    "أو أدخل مبلغًا آخر",
  ],
  amountHint: [
    "单次 $10–$10,000，最多保留两位小数。",
    "$10–$10,000 per order, with up to 2 decimal places.",
    "من 10 إلى 10,000 دولار للطلب، وبحد أقصى منزلتان عشريتان.",
  ],
  required: ["请输入充值金额。", "Enter an amount.", "أدخل مبلغ الشحن."],
  format: [
    "请输入有效金额，最多保留两位小数。",
    "Enter a valid amount with up to 2 decimal places.",
    "أدخل مبلغًا صالحًا بحد أقصى منزلتين عشريتين.",
  ],
  min: [
    "单次充值最低为 $10。",
    "The minimum recharge is $10.",
    "الحد الأدنى للشحن هو 10 دولارات.",
  ],
  max: [
    "单次充值最高为 $10,000。",
    "The maximum recharge is $10,000.",
    "الحد الأقصى للشحن هو 10,000 دولار.",
  ],
  channel: ["支付方式", "Payment method", "طريقة الدفع"],
  card: ["银行卡", "Bank card", "بطاقة مصرفية"],
  demoChannel: ["演示渠道", "Demo channel", "قناة تجريبية"],
  cardHint: [
    "通过本地演示收银台完成付款。",
    "Complete payment in the local demo checkout.",
    "أكمل الدفع في صفحة الدفع التجريبية المحلية.",
  ],
  creditedAmount: ["到账金额", "Amount credited", "المبلغ المضاف"],
  expectedCredit: ["预计到账金额", "Expected credit", "الرصيد المتوقع"],
  fee: ["额外手续费", "Additional fees", "رسوم إضافية"],
  noFee: ["无", "None", "لا توجد"],
  payable: ["实际支付", "Payment amount", "المبلغ المدفوع"],
  equalAmount: [
    "实付金额将全额计入预付费余额。",
    "The full payment amount is added to your prepaid balance.",
    "يُضاف المبلغ المدفوع كاملًا إلى رصيدك المسبق.",
  ],
  demoNote: [
    "前端演示，不收集真实卡号，也不会发生真实扣款。",
    "Frontend demo: no real card details or charges.",
    "عرض تجريبي: لا تُجمع بيانات بطاقات حقيقية ولا يتم خصم أموال.",
  ],
  createOrder: [
    "确认金额并创建订单",
    "Confirm and create order",
    "تأكيد وإنشاء الطلب",
  ],
  createHint: [
    "确认后生成订单，30 分钟内有效。",
    "Orders are valid for 30 minutes after confirmation.",
    "يسري الطلب لمدة 30 دقيقة بعد التأكيد.",
  ],
  cancel: ["取消", "Cancel", "إلغاء"],
  close: ["完成", "Done", "تم"],
  later: ["稍后支付", "Pay later", "الدفع لاحقًا"],
  continueLater: ["稍后继续", "Continue later", "المتابعة لاحقًا"],
  goCheckout: [
    "进入演示收银台",
    "Open demo checkout",
    "فتح صفحة الدفع التجريبية",
  ],
  resumeCheckout: [
    "继续演示支付",
    "Resume demo payment",
    "متابعة الدفع التجريبي",
  ],
  backOrder: ["返回订单", "Back to order", "العودة إلى الطلب"],
  pending: ["待支付", "Awaiting payment", "بانتظار الدفع"],
  processing: ["等待支付确认", "Awaiting confirmation", "بانتظار تأكيد الدفع"],
  paid: ["已到账", "Credited", "تمت الإضافة"],
  failed: ["支付失败", "Payment failed", "فشل الدفع"],
  cancelled: ["支付已取消", "Payment cancelled", "أُلغي الدفع"],
  expired: ["订单已过期", "Order expired", "انتهت صلاحية الطلب"],
  disputed: ["订单待核实", "Order under review", "الطلب قيد المراجعة"],
  orderNumber: ["充值订单号", "Recharge order", "رقم طلب الشحن"],
  account: ["到账账户", "Credit destination", "الحساب المستفيد"],
  prepaid: ["账号预付费余额", "Account prepaid balance", "رصيد الحساب المسبق"],
  validUntil: ["有效期至", "Valid until", "صالح حتى"],
  remaining: ["付款剩余时间", "Time left to pay", "الوقت المتبقي للدفع"],
  keepOrder: [
    "离开此页后订单仍会保留，可从充值记录继续支付。",
    "Closing keeps your order. Resume it from recharge records.",
    "يبقى الطلب محفوظًا بعد الإغلاق. تابعه من سجل الشحن.",
  ],
  resultTitle: [
    "选择演示支付结果",
    "Choose a demo payment result",
    "اختر نتيجة الدفع التجريبية",
  ],
  resultHint: [
    "此处模拟银行卡支付，不需要填写任何卡片资料。",
    "This simulates card payment. No card details are needed.",
    "هذه محاكاة لدفع البطاقة؛ لا يلزم إدخال بيانات بطاقة.",
  ],
  simulateSuccess: ["模拟支付成功", "Simulate success", "محاكاة نجاح الدفع"],
  simulateFailure: ["模拟支付失败", "Simulate failure", "محاكاة فشل الدفع"],
  simulateCancel: [
    "模拟取消支付",
    "Simulate cancellation",
    "محاكاة إلغاء الدفع",
  ],
  actualCredited: [
    "本次实际到账",
    "Credit added by this order",
    "الرصيد المضاف بهذا الطلب",
  ],
  latestBalance: [
    "当前账户余额",
    "Current account balance",
    "رصيد الحساب الحالي",
  ],
  viewRecords: ["查看充值记录", "View recharge records", "عرض سجل الشحن"],
  newOrder: ["重新充值", "Start a new recharge", "بدء شحن جديد"],
  failedReason: [
    "本次演示支付未成功，余额未变动。请重新创建订单后再试。",
    "The demo payment failed and no credit was added. Create a new order to try again.",
    "فشل الدفع التجريبي ولم يُضف رصيد. أنشئ طلبًا جديدًا للمحاولة مجددًا.",
  ],
  cancelledReason: [
    "你已取消本次演示支付，余额未变动。",
    "You cancelled this demo payment. No credit was added.",
    "ألغيت هذا الدفع التجريبي ولم يُضف رصيد.",
  ],
  expiredReason: [
    "订单超过 30 分钟有效期，不能继续付款。请创建一笔新订单。",
    "The 30-minute payment window has ended. Create a new order to pay.",
    "انتهت مهلة الدفع البالغة 30 دقيقة. أنشئ طلبًا جديدًا للدفع.",
  ],
  disputedReason: [
    "这笔订单需要进一步核实，请在充值记录中查看状态。",
    "This order needs review. Check its status in recharge records.",
    "يحتاج هذا الطلب إلى مراجعة. تحقق من حالته في سجل الشحن.",
  ],
  missing: [
    "未找到这笔充值订单",
    "Recharge order not found",
    "لم يُعثر على طلب الشحن",
  ],
  missingHint: [
    "演示数据可能已重置。你可以返回充值记录或创建新订单。",
    "Demo data may have been reset. View records or create a new order.",
    "ربما أُعيد ضبط البيانات التجريبية. راجع السجل أو أنشئ طلبًا جديدًا.",
  ],
  genericError: [
    "暂时无法完成此操作，请检查订单状态后重试。",
    "This action could not be completed. Check the order status and try again.",
    "تعذر إكمال العملية. تحقق من حالة الطلب وحاول مجددًا.",
  ],
  notPayable: [
    "这笔订单已不能继续付款，请查看当前状态或重新充值。",
    "This order can no longer accept payment. Check its status or start a new recharge.",
    "لم يعد الدفع متاحًا لهذا الطلب. تحقق من حالته أو ابدأ شحنًا جديدًا.",
  ],
  latePayment: [
    "付款在订单到期后确认，充值金额已全额到账。",
    "Payment was confirmed after expiry. The full amount has been credited.",
    "تأكد الدفع بعد انتهاء الصلاحية وأُضيف المبلغ كاملًا.",
  ],
  scenarios: ["演示场景", "Demo scenarios", "سيناريوهات تجريبية"],
  scenariosHint: [
    "仅用于验证本地演示流程，不会发起真实支付。",
    "For checking the local demo only; no real payment is initiated.",
    "لاختبار العرض المحلي فقط؛ لن تبدأ أي عملية دفع حقيقية.",
  ],
  simulateExpiry: ["模拟订单过期", "Simulate expiry", "محاكاة انتهاء الصلاحية"],
  repeatSuccess: [
    "重复成功通知",
    "Repeat success notification",
    "تكرار إشعار النجاح",
  ],
  lateSuccess: ["模拟晚到支付", "Simulate late payment", "محاكاة دفع متأخر"],
  repeated: [
    "这笔充值已入账。重复通知没有再次增加余额。",
    "This order was already credited. The repeated notification added no further credit.",
    "أُضيف رصيد هذا الطلب سابقًا. لم يضف الإشعار المتكرر رصيدًا آخر.",
  ],
  expiredDemo: [
    "已模拟订单到期。",
    "Order expiry simulated.",
    "تمت محاكاة انتهاء صلاحية الطلب.",
  ],
  expiryHint: [
    "模拟过期会推进演示时间，其他同时到期的订单也会失效。",
    "Simulating expiry advances demo time; other orders that are due also expire.",
    "تقدم محاكاة الانتهاء وقت العرض؛ تنتهي أيضًا الطلبات التي حل موعدها.",
  ],
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
        {text("返回上一页", "Back", "رجوع")}
      </button>
      <header className="rf-page-heading">
        <div>
          <span className="rf-page-eyebrow">
            <Wallet size={14} />
            {text("账户与资金", "ACCOUNT & BILLING", "الحساب والفوترة")}
          </span>
          <h1>{title}</h1>
          <p>{description}</p>
        </div>
        <span className="rf-page-currency">
          USD
          <span>
            {text(
              "美元预付费账户",
              "USD prepaid account",
              "حساب مسبق بالدولار",
            )}
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
          "当前为交互演示，不会真实扣款。已创建的订单可在充值记录中继续查看。",
          "Interactive demo; no real charges. Saved orders remain available in top-up history.",
          "عرض تفاعلي بلا خصم حقيقي. تبقى الطلبات المحفوظة في سجل الشحن.",
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
  const accountName = useProductPreview((state) =>
    accountDisplayName(state.user),
  );
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
