import { useState } from "react";
import { useMoneyLedger, useMoneyLedgerExport } from "../lib/api/money-wallet";
import { formatMoney } from "../lib/money";
import { Button } from "../components/ui/button";
import "../components/money-ledger.css";

type Text = (zh: string, en: string, ar: string) => string;
export default function MoneyLedger({ text }: { text: Text }) {
  const [page, setPage] = useState(1),
    query = useMoneyLedger(page),
    exportLedger = useMoneyLedgerExport();
  const data = query.error ? undefined : query.data;
  const labels: Record<string, string> = {
    opening: text("期初余额", "Opening balance", "الرصيد الافتتاحي"),
    reservation: text("调用预扣", "Request reservation", "حجز الطلب"),
    release: text("预扣退回", "Reservation released", "تحرير الحجز"),
    consumption: text("调用结算", "Usage settlement", "تسوية الاستخدام"),
    admin_credit: text("后台入账", "Administrator credit", "إضافة المسؤول"),
    admin_debit: text("后台扣减", "Administrator debit", "خصم المسؤول"),
    redemption: text("兑换码入账", "Redemption credit", "رصيد الاسترداد"),
    affiliate_rebate: text("返佣入账", "Referral credit", "رصيد الإحالة"),
    legacy_adjustment: text("余额调整", "Balance adjustment", "تعديل الرصيد"),
  };
  return (
    <section
      className="money-ledger"
      aria-labelledby="money-ledger-title"
      data-money-ledger
    >
      <header>
        <div>
          <h2 id="money-ledger-title">
            {text("API 余额账本", "API balance ledger", "دفتر رصيد الواجهة")}
          </h2>
          <p>
            {text(
              "可用余额、预扣占用与已结算消费分别记录，金额精确到纳美元。",
              "Available funds, pending reservations and settled spending are recorded separately, to nine decimal places.",
              "تُسجّل الأموال المتاحة والحجوزات المعلقة والإنفاق المسوّى بشكل منفصل بدقة تسع منازل عشرية.",
            )}
          </p>
        </div>
        <div className="money-ledger-actions">
          <Button
            size="small"
            disabled={exportLedger.isPending}
            onClick={() => exportLedger.mutate()}
          >
            {text(
              "导出近 31 天账本",
              "Export last 31 days",
              "تصدير آخر 31 يومًا",
            )}
          </Button>
          <Button
            size="small"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {text("刷新账本", "Refresh ledger", "تحديث الدفتر")}
          </Button>
        </div>
      </header>
      {exportLedger.isError && (
        <p role="alert">
          {text(
            "导出未确认，请重试。",
            "Export was not confirmed. Retry.",
            "لم يتم تأكيد التصدير. أعد المحاولة.",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "账本暂不可用，请刷新重试。",
            "The ledger is unavailable. Refresh to retry.",
            "الدفتر غير متاح. حدّث لإعادة المحاولة.",
          )}
        </p>
      )}
      {!data && !query.isError && (
        <p aria-live="polite">{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>
      )}
      {data?.items.map((row) => (
        <article key={row.id} data-money-entry={row.sequence}>
          <div className="money-entry-heading">
            <strong>{labels[row.type] ?? row.type}</strong>
            <time
              dir="ltr"
              dateTime={new Date(row.created_at * 1000).toISOString()}
            >
              {new Date(row.created_at * 1000)
                .toISOString()
                .replace("T", " ")
                .replace(".000Z", " UTC")}
            </time>
          </div>
          <dl>
            {[
              [
                text("可用变动", "Available change", "تغير المتاح"),
                row.available_delta_nano,
              ],
              [
                text("预扣变动", "Reservation change", "تغير الحجز"),
                row.reserved_delta_nano,
              ],
              [
                text("结算消费", "Settled spending", "الإنفاق المسوّى"),
                row.spent_delta_nano,
              ],
              [
                text("可用余额", "Available balance", "الرصيد المتاح"),
                row.available_after_nano,
              ],
              [
                text("预扣余额", "Reserved balance", "الرصيد المحجوز"),
                row.reserved_after_nano,
              ],
            ].map(([label, amount]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd>
                  <bdi dir="ltr">{formatMoney(amount, "detail")}</bdi>
                </dd>
              </div>
            ))}
          </dl>
          <details>
            <summary>
              {text("记账依据", "Posting details", "تفاصيل القيد")}
            </summary>
            <p>{row.reason}</p>
            <dl>
              <div>
                <dt>{text("业务编号", "Operation ID", "معرّف العملية")}</dt>
                <dd>
                  <bdi>{row.operation_id}</bdi>
                </dd>
              </div>
              {row.log_id && (
                <div>
                  <dt>{text("调用记录", "Usage log", "سجل الاستخدام")}</dt>
                  <dd>{row.log_id}</dd>
                </div>
              )}
              {row.reservation_id && (
                <div>
                  <dt>{text("预扣编号", "Reservation ID", "معرّف الحجز")}</dt>
                  <dd>
                    <bdi>{row.reservation_id}</bdi>
                  </dd>
                </div>
              )}
              <div>
                <dt>{text("资金来源", "Funding source", "مصدر التمويل")}</dt>
                <dd>{row.funding_source}</dd>
              </div>
            </dl>
          </details>
        </article>
      ))}
      {data && (
        <footer>
          <Button
            size="small"
            disabled={page === 1 || query.isFetching}
            onClick={() => setPage((p) => p - 1)}
          >
            {text("上一页", "Previous", "السابق")}
          </Button>
          <span>
            {page} / {Math.max(1, Math.ceil(data.total / data.page_size))}
          </span>
          <Button
            size="small"
            disabled={page * data.page_size >= data.total || query.isFetching}
            onClick={() => setPage((p) => p + 1)}
          >
            {text("下一页", "Next", "التالي")}
          </Button>
        </footer>
      )}
    </section>
  );
}
