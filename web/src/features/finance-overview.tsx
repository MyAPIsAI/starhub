import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "../components/ui/button";
import { useFinanceSummary } from "../lib/api/finance";
import { formatFinanceAmount } from "../lib/finance-format";
import { canReadFinance, useSystemAccess } from "../lib/system-access";
import "./finance-overview.css";

export function FinanceOverview() {
  const { i18n } = useTranslation();
  const text = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [params, setParams] = useSearchParams();
  const candidate = Number(params.get("days") || 30);
  const days = [7, 30, 90].includes(candidate) ? candidate : 30;
  const query = useFinanceSummary(days);
  const role = useSystemAccess((s) => s.role);
  if (!canReadFinance(role))
    return (
      <p role="alert">
        {text(
          "该账户没有财务访问权限。",
          "This account has no finance access.",
          "هذا الحساب لا يملك صلاحية الوصول المالي.",
        )}
      </p>
    );
  const data = query.error ? undefined : query.data;
  const date = (timestamp: number) =>
    new Intl.DateTimeFormat(i18n.language, {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: "UTC",
    }).format(new Date(timestamp * 1000));
  return (
    <section className="finance-overview">
      <header className="finance-heading">
        <div>
          <p className="finance-eyebrow">
            {text("只读财务", "Read-only finance", "المالية للقراءة فقط")}
          </p>
          <h1>{text("财务概览", "Finance overview", "نظرة مالية")}</h1>
          <p>
            {text(
              "按平台已记录的付款、钱包及配额消费核对金额。",
              "Review recorded payments, wallet balances, and API quota charges.",
              "راجع المدفوعات المسجلة وأرصدة المحافظ ورسوم حصة الواجهة.",
            )}
          </p>
        </div>
        <div className="finance-tools">
          <label>
            {text("统计周期", "Reporting window", "فترة التقرير")}
            <select
              className="select-trigger"
              value={days}
              onChange={(e) => {
                const next = new URLSearchParams(params);
                next.set("days", e.target.value);
                setParams(next);
              }}
            >
              {[7, 30, 90].map((n) => (
                <option value={n} key={n}>
                  {text(`最近 ${n} 天`, `Last ${n} days`, `آخر ${n} يومًا`)}
                </option>
              ))}
            </select>
          </label>
          <Button
            variant="secondary"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw size={15} />
            {text("刷新", "Refresh", "تحديث")}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取财务记录…",
            "Loading financial records…",
            "جارٍ تحميل السجلات المالية…",
          )}
        </p>
      )}
      {query.error && (
        <div className="finance-error" role="alert">
          {text(
            "无法读取财务记录。请确认账户仍有财务权限后重试。",
            "Financial records are unavailable. Confirm your account still has finance access and retry.",
            "السجلات المالية غير متاحة. تأكد من استمرار صلاحية الوصول المالي ثم أعد المحاولة.",
          )}
        </div>
      )}
      {data && (
        <>
          <p className="finance-window" dir="auto">
            {date(data.since)} — {date(data.until - 1)} UTC
          </p>
          {!data.wallet_and_api_quota_linked && (
            <p className="finance-note">
              {text(
                "现金钱包与 API 配额目前分别记账，下列金额不能相加作为可用余额或收入。钱包为当前快照，其余指标按所选周期统计。",
                "Cash wallets and API quota use separate accounting. These amounts cannot be combined into an available balance or revenue. Wallet balances are a current snapshot; other metrics use the reporting window.",
                "تستخدم المحافظ النقدية وحصة الواجهة حسابات منفصلة. لا يمكن جمع هذه المبالغ كرصيد متاح أو إيراد. أرصدة المحافظ لقطة حالية؛ وتستخدم المقاييس الأخرى فترة التقرير.",
              )}
            </p>
          )}
          <div className="finance-grid">
            {[
              {
                key: "wallet",
                title: text(
                  "现金钱包余额快照",
                  "Cash wallet balance snapshot",
                  "لقطة رصيد المحافظ النقدية",
                ),
                amount: data.wallet_balance_nano,
                note: text(
                  "所有钱包的已记录余额",
                  "Recorded balances across all wallets",
                  "الأرصدة المسجلة لجميع المحافظ",
                ),
              },
              {
                key: "recharge",
                title: text(
                  "已记录的已付充值",
                  "Recorded paid top-ups",
                  "عمليات الشحن المدفوعة المسجلة",
                ),
                amount: data.paid_recharge_nano,
                note: text(
                  `${data.paid_recharge_count} 笔 · 按入账时间`,
                  `${data.paid_recharge_count} orders · by paid time`,
                  `${data.paid_recharge_count} طلبًا · حسب وقت الدفع`,
                ),
              },
              {
                key: "collected",
                title: text(
                  "API 配额消费折算",
                  "API quota charged (USD equivalent)",
                  "رسوم حصة الواجهة (بالدولار)",
                ),
                amount: data.api_collected_nano,
                note: text(
                  "包含已收费的失败与取消请求",
                  "Includes charged failed and cancelled requests",
                  "تشمل الطلبات الفاشلة والملغاة التي حُوسبت",
                ),
              },
              {
                key: "unfunded",
                title: text(
                  "未收配额折算",
                  "Unfunded quota (USD equivalent)",
                  "حصة غير محصلة (بالدولار)",
                ),
                amount: data.api_unfunded_nano,
                note: text(
                  "已定价但未能收取的用量",
                  "Priced usage that could not be collected",
                  "استخدام مسعّر تعذر تحصيله",
                ),
              },
              {
                key: "credits",
                title: text(
                  "钱包流水入账",
                  "Wallet ledger credits",
                  "قيود المحفظة الدائنة",
                ),
                amount: data.ledger_credits_nano,
                note: text(
                  "所选周期内的正向流水",
                  "Positive ledger entries in this window",
                  "القيود الموجبة في الفترة",
                ),
              },
              {
                key: "debits",
                title: text(
                  "钱包流水出账",
                  "Wallet ledger debits",
                  "قيود المحفظة المدينة",
                ),
                amount: data.ledger_debits_nano,
                note: text(
                  "所选周期内的负向流水绝对值",
                  "Absolute value of negative ledger entries",
                  "القيمة المطلقة للقيود السالبة",
                ),
              },
            ].map((metric) => (
              <article
                className={`finance-metric ${formatFinanceAmount(metric.amount).length > 18 ? "finance-metric--wide" : ""}`}
                key={metric.key}
                data-metric={metric.key}
              >
                <h2>{metric.title}</h2>
                <strong dir="ltr" tabIndex={0}>
                  {formatFinanceAmount(metric.amount)}
                </strong>
                <p>{metric.note}</p>
                <details>
                  <summary>
                    {text(
                      "精确金额（纳美元）",
                      "Exact amount (nano-USD)",
                      "المبلغ الدقيق (نانو دولار)",
                    )}
                  </summary>
                  <code dir="ltr" tabIndex={0}>
                    {metric.amount}
                  </code>
                </details>
              </article>
            ))}
          </div>
          <div className="finance-outcomes">
            <h2>
              {text(
                "API 请求状态",
                "API request outcomes",
                "نتائج طلبات الواجهة",
              )}
            </h2>
            <dl>
              {[
                [
                  text("总请求", "Total requests", "إجمالي الطلبات"),
                  data.api_request_count,
                ],
                [text("成功", "Successful", "ناجحة"), data.api_success_count],
                [text("失败", "Failed", "فاشلة"), data.api_failed_count],
                [
                  text("已取消", "Cancelled", "ملغاة"),
                  data.api_cancelled_count,
                ],
                [
                  text("未知状态", "Unknown status", "حالة غير معروفة"),
                  Math.max(
                    0,
                    data.api_request_count -
                      data.api_success_count -
                      data.api_failed_count -
                      data.api_cancelled_count,
                  ),
                ],
              ].map(([label, count]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{count.toLocaleString()}</dd>
                </div>
              ))}
            </dl>
          </div>
          {data.paid_orders_without_timestamp > 0 && (
            <p className="finance-note">
              {text(
                `全量记录中有 ${data.paid_orders_without_timestamp} 笔已付订单缺少入账时间，未分配到周期充值统计。`,
                `${data.paid_orders_without_timestamp} recorded paid orders have no payment timestamp and are excluded from windowed top-up totals.`,
                `${data.paid_orders_without_timestamp} طلبًا مدفوعًا بلا وقت دفع مستبعد من إجمالي الشحن للفترة.`,
              )}
            </p>
          )}
        </>
      )}
    </section>
  );
}
