import { useEffect, useMemo, useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";
import {
  ArrowDownLeft,
  ArrowDownToLine,
  ArrowRight,
  ArrowUpRight,
  BellRing,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Clock3,
  Copy,
  CreditCard,
  FileText,
  Info,
  Landmark,
  ReceiptText,
  RefreshCw,
  Search,
  ShieldCheck,
  Wallet,
  X,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Drawer, Modal } from "../components/ui/overlay";
import { usePreferences } from "../lib/preferences";
import {
  useProductPreview,
  type LedgerEntry,
  type RechargeOrder,
} from "../lib/product-preview";
import { formatMoney } from "../lib/money";
import {
  billingDate,
  filterLedger,
  filterRechargeOrders,
  serializeLedgerCsv,
  serializeRechargeCsv,
  validateBillingRange,
} from "../lib/billing-export";
import { copyText } from "./code-example";
import "../components/billing-center.css";

type CopyFn = (zh: string, en: string, ar: string) => string;
export function billingStatus(status: string, c: CopyFn) {
  const labels: Record<string, [string, string, string]> = {
    pending: ["待支付", "Awaiting payment", "بانتظار الدفع"],
    processing: ["支付确认中", "Confirming payment", "تأكيد الدفع"],
    paid: ["已到账", "Credited", "تمت الإضافة"],
    expired: ["已过期", "Expired", "منتهي"],
    failed: ["支付失败", "Payment failed", "فشل الدفع"],
    cancelled: ["已取消", "Cancelled", "ملغى"],
    disputed: ["争议处理中", "Disputed", "نزاع قيد المعالجة"],
  };
  return labels[status] ? c(...labels[status]) : status;
}
function entryType(type: string, c: CopyFn) {
  const words: Record<string, [string, string, string]> = {
    recharge: ["账户充值", "Top-up", "شحن الرصيد"],
    call_summary: ["调用消费", "Model usage", "استخدام النماذج"],
    adjustment: ["账户调整", "Adjustment", "تعديل"],
    refund: ["退款扣回", "Refund", "استرداد"],
  };
  return words[type] ? c(...words[type]) : type;
}
function Status({ order, c }: { order: RechargeOrder; c: CopyFn }) {
  return (
    <span className={`bc-status is-${order.status}`}>
      <i />
      {order.status === "paid" && order.late_payment
        ? c(
            "已到账 · 延迟确认",
            "Credited · confirmed late",
            "تمت الإضافة · تأكيد متأخر",
          )
        : billingStatus(order.status, c)}
    </span>
  );
}
function downloadCsv(csv: string, filename: string) {
  const url = URL.createObjectURL(
    new Blob([csv], { type: "text/csv;charset=utf-8" }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export default function BillingCenter({
  onRecharge,
}: {
  onRecharge: (orderId?: string) => void;
}) {
  const language = usePreferences((s) => s.language);
  const c: CopyFn = (zh, en, ar) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const tab =
    params.get("tab") === "ledger" || params.has("trace") ? "ledger" : "orders";
  const balance = useProductPreview((s) => s.balance);
  const ledger = useProductPreview((s) => s.ledger);
  const orders = useProductPreview((s) => s.rechargeOrders);
  const expireOrders = useProductPreview((s) => s.expireRechargeOrders);
  const today = billingDate(new Date().toISOString());
  const month = today.slice(0, 7);
  const [from, setFrom] = useState(`${month}-01`);
  const [to, setTo] = useState(today);
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState("all");
  const [page, setPage] = useState(1);
  const [guide, setGuide] = useState<"help" | "transfer" | null>(null);
  const [entry, setEntry] = useState<LedgerEntry | null>(null);
  const [message, setMessage] = useState("");
  useEffect(() => {
    expireOrders();
    const timer = setInterval(expireOrders, 15000);
    return () => clearInterval(timer);
  }, [expireOrders]);
  useEffect(() => {
    setPage(1);
    setMessage("");
  }, [from, to, query, kind, tab, language]);
  useEffect(() => {
    setKind("all");
    setQuery("");
  }, [tab]);
  useEffect(() => {
    const selected = orders.find(
      (order) => order.id === location.state?.billingOrder,
    );
    if (tab !== "orders" || !selected) return;
    const created = billingDate(selected.created_at);
    setFrom(created);
    setTo(created);
    setKind("all");
    setQuery(selected.id);
    setPage(1);
  }, [location.key]);
  useEffect(() => {
    const trace = params.get("trace");
    if (!trace) return;
    const matched = ledger.find((row) => row.trace_id === trace);
    if (matched) {
      setFrom(billingDate(matched.time));
      setTo(billingDate(matched.time));
      setQuery(trace);
      setEntry(matched);
      setKind("all");
    }
  }, [location.search, ledger]);
  const validRange = validateBillingRange(from, to);
  const filteredOrders = useMemo(
    () =>
      validRange
        ? filterRechargeOrders(orders, {
            from,
            to,
            status: kind === "all" ? undefined : kind,
            query,
          })
        : [],
    [orders, from, to, kind, query, validRange],
  );
  const filteredLedger = useMemo(
    () =>
      validRange
        ? filterLedger(ledger, {
            from,
            to,
            type: kind === "all" ? undefined : kind,
            query,
          })
        : [],
    [ledger, from, to, kind, query, validRange],
  );
  const count =
    tab === "orders" ? filteredOrders.length : filteredLedger.length;
  const pages = Math.max(1, Math.ceil(count / 6));
  const currentPage = Math.min(page, pages);
  const start = (currentPage - 1) * 6;
  const pending = orders.filter((o) =>
    ["pending", "processing"].includes(o.status),
  );
  const spend = ledger
    .filter(
      (e) => e.type === "call_summary" && billingDate(e.time).startsWith(month),
    )
    .reduce(
      (sum, e) =>
        sum + (BigInt(e.amount_nano) < 0n ? -BigInt(e.amount_nano) : 0n),
      0n,
    );
  const funded = ledger
    .filter(
      (e) => e.type === "recharge" && billingDate(e.time).startsWith(month),
    )
    .reduce((sum, e) => sum + BigInt(e.amount_nano), 0n);
  const isLow = BigInt(balance) < 5_000_000_000n;
  const isEmpty = BigInt(balance) <= 0n;
  const date = (value: string) =>
    new Intl.DateTimeFormat(
      language === "zh" ? "zh-CN" : language === "ar" ? "ar-AE" : "en-GB",
      {
        timeZone: "Asia/Dubai",
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      },
    ).format(new Date(value));
  const changeTab = (next: string) => {
    setKind("all");
    setQuery("");
    const nextParams = new URLSearchParams(params);
    nextParams.set("tab", next);
    nextParams.delete("trace");
    setParams(nextParams, { replace: true });
  };
  const resetFilters = () => {
    setFrom(`${month}-01`);
    setTo(today);
    setQuery("");
    setKind("all");
  };
  const exportRecords = () => {
    if (!validRange || !count) return;
    downloadCsv(
      tab === "orders"
        ? serializeRechargeCsv(filteredOrders, language)
        : serializeLedgerCsv(filteredLedger, language),
      `veyra-${tab}-${from}-${to}.csv`,
    );
    setMessage(
      c(
        `已导出当前筛选的 ${count} 条记录。`,
        `Exported ${count} matching records.`,
        `تم تصدير ${count} سجلًا مطابقًا.`,
      ),
    );
  };
  const description = (item: LedgerEntry) =>
    item.type === "recharge"
      ? c(
          "在线充值到账",
          "Online top-up credited",
          "تمت إضافة الشحن عبر الإنترنت",
        )
      : item.type === "call_summary"
        ? item.trace_id
          ? c("模型调用扣费", "Model request charge", "رسوم طلب النموذج")
          : c("模型用量结算", "Model usage settlement", "تسوية استخدام النماذج")
        : item.description === "注销清零"
          ? c(
              "注销余额结清",
              "Account closure settlement",
              "تسوية رصيد الإغلاق",
            )
          : item.description === "Enterprise Onboarding Credit Incentive"
            ? c("演示初始额度", "Initial demo credit", "الرصيد التجريبي الأولي")
            : item.description === "Demo scenario balance adjustment"
              ? c(
                  "演示场景余额调整",
                  "Demo balance adjustment",
                  "تعديل الرصيد التجريبي",
                )
              : item.description;

  return (
    <div className="bc-page">
      <header className="bc-heading">
        <div>
          <div className="bc-eyebrow">
            <Wallet size={13} />
            {c("账户与资金", "ACCOUNT & BILLING", "الحساب والفوترة")}
          </div>
          <h1>{c("余额与充值", "Balance & top-ups", "الرصيد والشحن")}</h1>
          <p>
            {c(
              "为下一次创造，准备充足的算力。",
              "Keep your next idea powered and ready.",
              "جهّز رصيدك لفكرتك القادمة.",
            )}
          </p>
        </div>
        <Button className="bc-help-button" onClick={() => setGuide("help")}>
          <CircleHelp size={15} />
          {c("充值说明", "How top-ups work", "دليل الشحن")}
        </Button>
      </header>
      <div className="bc-summary-grid">
        <section className="bc-wallet-card">
          <div className="bc-wallet-top">
            <span>
              {c("账户可用余额", "Available balance", "الرصيد المتاح")}
            </span>
            <span className={`bc-wallet-health ${isLow ? "is-low" : ""}`}>
              <i />
              {isEmpty
                ? c("余额不足", "Balance depleted", "الرصيد غير كافٍ")
                : isLow
                  ? c("余额偏低", "Low balance", "الرصيد منخفض")
                  : c("余额充足", "Ready to use", "جاهز للاستخدام")}
            </span>
          </div>
          <div className="bc-balance">
            <strong dir="ltr">{formatMoney(balance)}</strong>
            <span>{c("USD（美元）", "USD", "USD (دولار)")}</span>
          </div>
          <p>
            {c(
              "按量付费 · 多个模型共用一个账户余额",
              "Pay as you go · One balance for all your models",
              "الدفع حسب الاستخدام · رصيد واحد لجميع النماذج",
            )}
          </p>
          <div className="bc-wallet-actions">
            <Button variant="primary" onClick={() => onRecharge()}>
              <CreditCard size={16} />
              {c("在线充值", "Add funds", "شحن الرصيد")}
              <ArrowRight size={14} />
            </Button>
            <Link to="/console/usage">
              {c("查看用量", "View usage", "عرض الاستخدام")}
              <ArrowUpRight size={14} />
            </Link>
          </div>
          <div className="bc-wallet-footer">
            <ShieldCheck size={14} />
            {c(
              "不收额外手续费，充值金额全额到账",
              "No extra fees. Your full top-up is credited.",
              "لا رسوم إضافية. يُضاف مبلغ الشحن كاملًا.",
            )}
          </div>
        </section>
        <section className="bc-month-card">
          <div className="bc-small-heading">
            <span>{c("本月资金概览", "This month", "هذا الشهر")}</span>
            <bdi>{month}</bdi>
          </div>
          <div className="bc-month-metric">
            <span className="bc-metric-icon">
              <ArrowUpRight size={17} />
            </span>
            <div>
              <span>{c("本月消费", "Usage this month", "استهلاك الشهر")}</span>
              <strong dir="ltr">
                {formatMoney(spend.toString(), "total")}
              </strong>
            </div>
          </div>
          <div className="bc-month-metric">
            <span className="bc-metric-icon is-green">
              <ArrowDownLeft size={17} />
            </span>
            <div>
              <span>
                {c("本月充值到账", "Top-ups credited", "الشحن المضاف")}
              </span>
              <strong dir="ltr">{formatMoney(funded.toString())}</strong>
            </div>
          </div>
          <p>
            {c(
              "按迪拜时区的自然月统计",
              "Calendar month in Dubai time",
              "الشهر التقويمي بتوقيت دبي",
            )}
          </p>
        </section>
        <section className="bc-protection-card">
          <span className="bc-protection-icon">
            <BellRing size={20} />
          </span>
          <h2>{c("低余额提醒", "Balance protection", "تنبيه الرصيد")}</h2>
          <p>
            {c(
              "余额低于 $5 时，控制台会提醒你及时充值。",
              "The console alerts you when your balance drops below $5.",
              "تنبهك اللوحة عندما يقل الرصيد عن 5 دولارات.",
            )}
          </p>
          <div>
            <span>{c("提醒阈值", "Alert threshold", "حد التنبيه")}</span>
            <strong dir="ltr">$5.00</strong>
          </div>
          <small>
            {c(
              "余额耗尽后暂停调用，到账后可继续使用。",
              "Calls pause when funds run out and resume after a top-up.",
              "تتوقف الطلبات عند نفاد الرصيد وتستأنف بعد الشحن.",
            )}
          </small>
        </section>
      </div>
      <div className="bc-demo-note">
        <Info size={14} />
        <span>
          {c(
            "当前为交互演示：不会真实扣款；正式支付渠道待接入。",
            "Interactive demo: no real charges. Production payment integration is pending.",
            "عرض تفاعلي بلا خصم حقيقي. تكامل الدفع الفعلي لم يُفعّل بعد.",
          )}
        </span>
      </div>
      {!!pending.length && (
        <div className="bc-pending-banner">
          <Clock3 size={19} />
          <div>
            <strong>
              {c(
                `你有 ${pending.length} 笔充值尚未完成`,
                `${pending.length} top-up${pending.length > 1 ? "s" : ""} still in progress`,
                `لديك ${pending.length} عملية شحن غير مكتملة`,
              )}
            </strong>
            <span>
              {c(
                "关闭支付页不会代表支付成功，到账结果以订单状态为准。",
                "Closing the payment window does not confirm payment. Check the order status.",
                "إغلاق صفحة الدفع لا يؤكد نجاحه. تحقق من حالة الطلب.",
              )}
            </span>
          </div>
          <Button size="small" onClick={() => onRecharge(pending[0].id)}>
            {c("继续查看", "View order", "عرض الطلب")}
            <ArrowRight size={13} />
          </Button>
        </div>
      )}
      <section className="bc-history-card">
        <div className="bc-history-heading">
          <nav aria-label={c("账务记录", "Billing records", "السجلات المالية")}>
            <button
              aria-pressed={tab === "orders"}
              onClick={() => changeTab("orders")}
            >
              <ReceiptText size={16} />
              {c("充值记录", "Top-up history", "سجل الشحن")}
              <span>{orders.length}</span>
            </button>
            <button
              aria-pressed={tab === "ledger"}
              onClick={() => changeTab("ledger")}
            >
              <FileText size={16} />
              {c("资金流水", "Transactions", "حركات الرصيد")}
            </button>
          </nav>
          <div className="bc-history-actions">
            <button
              className="bc-icon-button"
              onClick={() => {
                expireOrders();
                setMessage(
                  c(
                    "已更新本地演示订单状态。",
                    "Local demo order status updated.",
                    "تم تحديث حالة الطلبات التجريبية.",
                  ),
                );
              }}
              aria-label={c(
                "刷新订单状态",
                "Refresh order status",
                "تحديث حالة الطلب",
              )}
            >
              <RefreshCw size={15} />
            </button>
            <Button
              size="small"
              disabled={!validRange || !count}
              onClick={exportRecords}
            >
              <ArrowDownToLine size={14} />
              {c("导出记录", "Export", "تصدير")}
            </Button>
          </div>
        </div>
        <div className="bc-filters">
          <label className="bc-search">
            <span>{c("搜索", "Search", "بحث")}</span>
            <div>
              <Search size={14} />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={
                  tab === "orders"
                    ? c("搜索订单编号", "Search order ID", "ابحث برقم الطلب")
                    : c(
                        "搜索流水或关联订单",
                        "Search transaction or order",
                        "ابحث عن حركة أو طلب",
                      )
                }
                aria-label={c(
                  "搜索账务记录",
                  "Search billing records",
                  "بحث في السجلات",
                )}
              />
              {query && (
                <button
                  onClick={() => setQuery("")}
                  aria-label={c("清空搜索", "Clear search", "مسح البحث")}
                >
                  <X size={13} />
                </button>
              )}
            </div>
          </label>
          <label>
            <span>
              {tab === "orders"
                ? c("订单状态", "Order status", "حالة الطلب")
                : c("流水类型", "Transaction type", "نوع الحركة")}
            </span>
            <select
              value={kind}
              onChange={(e) => setKind(e.target.value)}
              aria-label={c(
                "记录类型或状态",
                "Record type or status",
                "نوع السجل أو حالته",
              )}
            >
              <option value="all">{c("全部", "All", "الكل")}</option>
              {(tab === "orders"
                ? [
                    "pending",
                    "processing",
                    "paid",
                    "failed",
                    "cancelled",
                    "expired",
                    "disputed",
                  ]
                : [
                    // M4 §5.1/§5.3：线上渠道退款首发不实现，线下退款走「手动调整·扣回」。
                    // `refund` 保留为前向兼容的流水类型（见 product-preview.ts、billing.ts 校验白名单），
                    // 但当前没有任何生产者，故不作为筛选项，避免出现必然为空的假选项。
                    "recharge",
                    "call_summary",
                    "adjustment",
                  ]
              ).map((value) => (
                <option key={value} value={value}>
                  {tab === "orders"
                    ? billingStatus(value, c)
                    : entryType(value, c)}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>{c("开始日期", "From", "من")}</span>
            <input
              type="date"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              aria-label={c("开始日期", "From date", "تاريخ البدء")}
            />
          </label>
          <label>
            <span>{c("结束日期", "To", "إلى")}</span>
            <input
              type="date"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              aria-label={c("结束日期", "To date", "تاريخ الانتهاء")}
            />
          </label>
          <button className="bc-reset" onClick={resetFilters}>
            {c("重置", "Reset", "إعادة ضبط")}
          </button>
        </div>
        <div className="bc-filter-note">
          <span>
            {c(
              "迪拜时间 · UTC+4（协调世界时加四小时） · 单次查询最多 31 天",
              "Dubai time · UTC+4 · Up to 31 days per query",
              "توقيت دبي · UTC+4 · حتى 31 يومًا لكل استعلام",
            )}
          </span>
          <b>{c(`${count} 条记录`, `${count} records`, `${count} سجلًا`)}</b>
        </div>
        {message && (
          <p className="bc-feedback" role="status">
            <Check size={14} />
            {message}
          </p>
        )}
        {!validRange ? (
          <div className="bc-empty" role="alert">
            <Clock3 size={30} />
            <h3>
              {c(
                "请调整日期范围",
                "Adjust the date range",
                "عدّل النطاق الزمني",
              )}
            </h3>
            <p>
              {c(
                "结束日期不能早于开始日期，且含首尾最多 31 天。",
                "The end date must follow the start date, with no more than 31 days inclusive.",
                "يجب أن يلي تاريخ الانتهاء البداية وألا تتجاوز المدة 31 يومًا شاملة.",
              )}
            </p>
            <Button onClick={resetFilters}>
              {c("回到本月", "Return to this month", "العودة إلى هذا الشهر")}
            </Button>
          </div>
        ) : !count ? (
          <div className="bc-empty">
            <ReceiptText size={34} />
            <h3>
              {c(
                "当前条件下暂无记录",
                "No matching records",
                "لا توجد سجلات مطابقة",
              )}
            </h3>
            <p>
              {c(
                "你可以调整筛选条件，或完成第一笔充值。",
                "Try another filter or complete your first top-up.",
                "جرّب تصفية أخرى أو أكمل أول عملية شحن.",
              )}
            </p>
            <div>
              <Button onClick={resetFilters}>
                {c("重置筛选", "Reset filters", "إعادة ضبط التصفية")}
              </Button>
              <Button variant="primary" onClick={() => onRecharge()}>
                {c("去充值", "Add funds", "شحن الرصيد")}
              </Button>
            </div>
          </div>
        ) : (
          <>
            <div className="bc-table-scroll">
              <table>
                <thead>
                  <tr>
                    {(tab === "orders"
                      ? [
                          c(
                            "订单 / 创建时间",
                            "Order / Created",
                            "الطلب / الإنشاء",
                          ),
                          c("支付方式", "Payment method", "طريقة الدفع"),
                          c("充值金额", "Amount", "المبلغ"),
                          c("到账状态", "Status", "الحالة"),
                          c("到账时间", "Credited at", "وقت الإضافة"),
                          c("操作", "Action", "الإجراء"),
                        ]
                      : [
                          c(
                            "时间 / 流水编号",
                            "Time / Transaction",
                            "الوقت / الحركة",
                          ),
                          c("类型", "Type", "النوع"),
                          c(
                            "说明 / 关联订单",
                            "Description / Order",
                            "الوصف / الطلب",
                          ),
                          c("变动金额", "Amount", "المبلغ"),
                          c("变动后余额", "Balance after", "الرصيد بعد"),
                          c("操作", "Action", "الإجراء"),
                        ]
                    ).map((label) => (
                      <th key={label}>{label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {tab === "orders"
                    ? filteredOrders.slice(start, start + 6).map((order) => (
                        <tr key={order.id}>
                          <td>
                            <button
                              className="bc-record-id"
                              onClick={() => onRecharge(order.id)}
                            >
                              {order.id}
                              <ArrowUpRight size={11} />
                            </button>
                            <small>{date(order.created_at)}</small>
                          </td>
                          <td>
                            <span className="bc-method">
                              <CreditCard size={15} />
                              {c(
                                "银行卡 · 演示",
                                "Card · demo",
                                "بطاقة · تجريبي",
                              )}
                            </span>
                          </td>
                          <td className="bc-amount">
                            <bdi>{formatMoney(order.amount_nano)}</bdi>
                          </td>
                          <td>
                            <Status order={order} c={c} />
                          </td>
                          <td>
                            <span className="bc-date">
                              {order.paid_at ? date(order.paid_at) : "—"}
                            </span>
                          </td>
                          <td>
                            <button
                              className="bc-text-button"
                              onClick={() => onRecharge(order.id)}
                            >
                              {["pending", "processing"].includes(order.status)
                                ? c("继续支付", "Continue", "متابعة")
                                : c("订单详情", "Details", "التفاصيل")}
                              <ChevronRight size={13} />
                            </button>
                          </td>
                        </tr>
                      ))
                    : filteredLedger.slice(start, start + 6).map((item) => (
                        <tr key={item.id}>
                          <td>
                            <span className="bc-date">{date(item.time)}</span>
                            <small className="bc-code">{item.id}</small>
                          </td>
                          <td>
                            <span className={`bc-entry-type is-${item.type}`}>
                              {entryType(item.type, c)}
                            </span>
                          </td>
                          <td>
                            <span>{description(item)}</span>
                            {item.order_id && (
                              <button
                                className="bc-associated"
                                onClick={() => onRecharge(item.order_id)}
                              >
                                {item.order_id}
                                <ArrowUpRight size={11} />
                              </button>
                            )}
                          </td>
                          <td
                            className={`bc-amount ${BigInt(item.amount_nano) > 0n ? "is-positive" : ""}`}
                          >
                            <bdi>
                              {BigInt(item.amount_nano) > 0n ? "+" : ""}
                              {formatMoney(
                                item.amount_nano,
                                item.type === "call_summary"
                                  ? "call"
                                  : "balance",
                              )}
                            </bdi>
                          </td>
                          <td>
                            <bdi>{formatMoney(item.balance_after_nano)}</bdi>
                          </td>
                          <td>
                            <button
                              className="bc-text-button"
                              onClick={() => setEntry(item)}
                            >
                              {c("查看明细", "Details", "التفاصيل")}
                              <ChevronRight size={13} />
                            </button>
                          </td>
                        </tr>
                      ))}
                </tbody>
              </table>
            </div>
            <div className="bc-pagination">
              <span>
                {c(
                  `显示 ${start + 1}–${Math.min(start + 6, count)} 条，共 ${count} 条`,
                  `${start + 1}–${Math.min(start + 6, count)} of ${count}`,
                  `${start + 1}–${Math.min(start + 6, count)} من ${count}`,
                )}
              </span>
              <div>
                <button
                  aria-label={c("上一页", "Previous page", "الصفحة السابقة")}
                  disabled={currentPage === 1}
                  onClick={() => setPage(currentPage - 1)}
                >
                  <ChevronLeft size={15} />
                </button>
                <span>
                  {currentPage} / {pages}
                </span>
                <button
                  aria-label={c("下一页", "Next page", "الصفحة التالية")}
                  disabled={currentPage === pages}
                  onClick={() => setPage(currentPage + 1)}
                >
                  <ChevronRight size={15} />
                </button>
              </div>
            </div>
          </>
        )}
      </section>
      <div className="bc-bottom-grid">
        <button
          className="bc-support-card"
          onClick={() => setGuide("transfer")}
        >
          <span>
            <Landmark size={20} />
          </span>
          <div>
            <h3>
              {c(
                "大额充值与线下付款",
                "Large top-ups & offline payments",
                "الشحن بمبالغ كبيرة والدفع خارج المنصة",
              )}
            </h3>
            <p>
              {c(
                "查看汇款核验、入账及退款处理说明",
                "Learn about verification, crediting and refunds",
                "التعرف على التحقق والإضافة والاسترداد",
              )}
            </p>
          </div>
          <ArrowUpRight size={16} />
        </button>
        <div className="bc-receipt-note">
          <ReceiptText size={19} />
          <p>
            {c(
              "支付收据由支付渠道提供。资金流水导出用于对账，不作为税务发票。",
              "Payment receipts come from the payment provider. Transaction exports are reconciliation records, not tax invoices.",
              "إيصالات الدفع من مزود الدفع. تصدير الحركات للمطابقة وليس فاتورة ضريبية.",
            )}
          </p>
        </div>
      </div>
      <Modal
        size="default"
        open={guide !== null}
        onOpenChange={(open) => {
          if (!open) setGuide(null);
        }}
        title={
          guide === "transfer"
            ? c("付款与退款", "Payments & refunds", "الدفع والاسترداد")
            : c("充值与到账说明", "Top-ups & crediting", "الشحن وإضافة الرصيد")
        }
        description={c(
          "按当前平台的美元预付费规则说明。",
          "Based on the platform’s USD prepaid policy.",
          "وفق سياسة الرصيد المسبق بالدولار للمنصة.",
        )}
      >
        <div className="bc-guide">
          <div className="bc-guide-lead">
            <ShieldCheck size={22} />
            <p>
              {c(
                "一个余额，按量使用。充值资金用于平台内的模型调用。",
                "One prepaid balance for model usage across the platform.",
                "رصيد مسبق واحد لاستخدام النماذج على المنصة.",
              )}
            </p>
          </div>
          {(guide === "transfer"
            ? [
                [
                  c("线下付款", "Offline payments", "الدفع خارج المنصة"),
                  c(
                    "先与商务或财务确认收款主体、账户信息及金额，再完成汇款。财务核验实际到账后记入平台余额。",
                    "Confirm the receiving entity, banking details and amount with the finance or sales team before transferring. The balance is credited after finance verifies receipt.",
                    "أكد جهة الاستلام والتفاصيل المصرفية والمبلغ مع المالية أو المبيعات أولًا. يُضاف الرصيد بعد التحقق من وصول الحوالة.",
                  ),
                ],
                [
                  c(
                    "保留核验资料",
                    "Keep verification records",
                    "احتفظ بمستندات التحقق",
                  ),
                  c(
                    "保留汇款凭证、付款主体、金额与日期。当前演示不会创建真实汇款申请，也不提供收款账户。",
                    "Keep the transfer receipt, payer, amount and date. This demo does not submit a real transfer request or provide banking details.",
                    "احتفظ بإيصال التحويل والدافع والمبلغ والتاريخ. هذا العرض لا يرسل طلب تحويل حقيقيًا ولا يوفر حسابات مصرفية.",
                  ),
                ],
                [
                  c(
                    "退款由人工处理",
                    "Refunds are handled manually",
                    "تُعالج المبالغ المستردة يدويًا",
                  ),
                  c(
                    "联系客户支持核对付款订单和未使用余额；可退金额由财务核验，赠送额度不可退款。处理完成后可在资金流水查看扣回记录。",
                    "Contact support to review the payment order and unused balance. Finance determines the refundable amount; promotional credits are non-refundable. The resulting debit appears in transactions.",
                    "تواصل مع الدعم لمراجعة الطلب والرصيد غير المستخدم. تحدد المالية المبلغ القابل للاسترداد؛ الرصيد الممنوح غير قابل للاسترداد. يظهر الخصم في الحركات.",
                  ),
                ],
              ]
            : [
                [
                  c("金额与费用", "Amounts & fees", "المبالغ والرسوم"),
                  c(
                    "单笔 $10–$10,000，最多两位小数。平台不收额外手续费，支付渠道手续费由平台承担。",
                    "$10–$10,000 per top-up, with up to two decimal places. The platform absorbs payment processing fees.",
                    "من 10 إلى 10,000 دولار للعملية بخانتين عشريتين كحد أقصى. تتحمل المنصة رسوم معالجة الدفع.",
                  ),
                ],
                [
                  c("何时到账", "When funds are credited", "متى يُضاف الرصيد"),
                  c(
                    "完成支付后，等待渠道确认。返回平台或关闭支付窗口都不会直接增加余额；可在充值记录查看进度。",
                    "After payment, wait for provider confirmation. Returning to the platform or closing the window does not itself credit your balance. Check top-up history for progress.",
                    "بعد الدفع انتظر تأكيد المزود. العودة إلى المنصة أو إغلاق النافذة لا يضيف الرصيد تلقائيًا. راجع السجل لمعرفة التقدم.",
                  ),
                ],
                [
                  c("订单有效期", "Order validity", "صلاحية الطلب"),
                  c(
                    "订单有效期为 30 分钟，超时未支付会过期。若支付确认晚到，核实成功后仍会入账，无需重复付款。",
                    "Orders are valid for 30 minutes. Unpaid orders expire. A successful payment confirmed late will still be credited; do not pay twice.",
                    "الطلب صالح لمدة 30 دقيقة ثم ينتهي إن لم يُدفع. يُضاف الدفع الناجح المتأخر تأكيده؛ لا تدفع مرتين.",
                  ),
                ],
                [
                  c("收据与账单", "Receipts & records", "الإيصالات والسجلات"),
                  c(
                    "收据由支付渠道发送；平台提供订单详情与资金流水。首发版本不提供在线税务发票。",
                    "Receipts are sent by the payment provider. The platform supplies order details and transactions. Online tax invoices are not available in the initial release.",
                    "يرسل مزود الدفع الإيصال. توفر المنصة تفاصيل الطلب والحركات. الفواتير الضريبية عبر الإنترنت غير متاحة في الإصدار الأول.",
                  ),
                ],
              ]
          ).map(([title, body], index) => (
            <section key={title}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div>
                <h3>{title}</h3>
                <p>{body}</p>
              </div>
            </section>
          ))}
        </div>
      </Modal>
      <Drawer
        open={entry !== null}
        onOpenChange={(open) => {
          if (!open) setEntry(null);
        }}
        title={c("资金流水明细", "Transaction details", "تفاصيل الحركة")}
        description={entry ? entryType(entry.type, c) : ""}
      >
        {entry && (
          <div className="bc-entry-detail">
            <div
              className={`bc-entry-value ${BigInt(entry.amount_nano) > 0n ? "is-positive" : ""}`}
            >
              <small>
                {c("变动金额", "Transaction amount", "مبلغ الحركة")}
              </small>
              <strong dir="ltr">
                {BigInt(entry.amount_nano) > 0n ? "+" : ""}
                {formatMoney(entry.amount_nano, "detail")}
              </strong>
              <span>
                {c(
                  "USD（美元） · 完整记账精度",
                  "USD · Full accounting precision",
                  "USD (دولار) · الدقة المحاسبية الكاملة",
                )}
              </span>
            </div>
            <dl>
              {[
                [c("流水编号", "Transaction ID", "رقم الحركة"), entry.id],
                [
                  c("发生时间（迪拜）", "Time (Dubai)", "الوقت (دبي)"),
                  `${date(entry.time)} · UTC+4`,
                ],
                [
                  c("变动后余额", "Balance after", "الرصيد بعد"),
                  formatMoney(entry.balance_after_nano, "detail"),
                ],
                [c("说明", "Description", "الوصف"), description(entry)],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
            {entry.trace_id && (
              <Button asChild>
                <Link
                  to={`/console/requests?trace=${encodeURIComponent(entry.trace_id)}`}
                >
                  {c(
                    "查看关联调用",
                    "View related request",
                    "عرض الطلب المرتبط",
                  )}
                  <ArrowRight size={14} />
                </Link>
              </Button>
            )}
            {entry.order_id && (
              <Button
                onClick={() => {
                  const id = entry.order_id;
                  setEntry(null);
                  onRecharge(id);
                }}
              >
                <ReceiptText size={15} />
                {c(
                  "查看关联充值订单",
                  "View related top-up",
                  "عرض طلب الشحن المرتبط",
                )}
                <ArrowRight size={14} />
              </Button>
            )}
            <Button
              onClick={() =>
                copyText(
                  entry.id,
                  c("编号已复制", "ID copied", "تم نسخ الرقم"),
                  c("复制失败", "Copy failed", "فشل النسخ"),
                )
              }
            >
              <Copy size={14} />
              {c("复制流水编号", "Copy transaction ID", "نسخ رقم الحركة")}
            </Button>
            <p>
              {c(
                "此明细用于账户核对，不是支付收据或税务发票。",
                "This record is for reconciliation, not a payment receipt or tax invoice.",
                "هذا السجل للمطابقة وليس إيصال دفع أو فاتورة ضريبية.",
              )}
            </p>
          </div>
        )}
      </Drawer>
    </div>
  );
}
