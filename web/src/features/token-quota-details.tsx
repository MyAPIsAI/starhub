import { useState } from "react";
import { Button } from "../components/ui/button";
import { useOwnedTokenUsage } from "../lib/api/token-usage";
import { quotaToNanoUsd } from "../lib/api/key-adapter";
import { formatMoney } from "../lib/money";

type Text = (zh: string, en: string, ar: string) => string;
const money = (quota: number) =>
  formatMoney(
    quotaToNanoUsd(quota),
    quota > 0 && quota < 50 ? "call" : "total",
  );

function Pages({
  page,
  total,
  size,
  onPage,
  text,
}: {
  page: number;
  total: number;
  size: number;
  onPage: (page: number) => void;
  text: Text;
}) {
  if (total <= size) return null;
  return (
    <div className="key-usage-pages">
      <Button
        size="small"
        disabled={page <= 1}
        onClick={() => onPage(page - 1)}
      >
        {text("上一页", "Previous", "السابق")}
      </Button>
      <span>
        {page} / {Math.ceil(total / size)}
      </span>
      <Button
        size="small"
        disabled={page * size >= total}
        onClick={() => onPage(page + 1)}
      >
        {text("下一页", "Next", "التالي")}
      </Button>
    </div>
  );
}

export default function TokenQuotaDetails({
  tokenID,
  locale,
  text,
}: {
  tokenID: number;
  locale: string;
  text: Text;
}) {
  const [periodPage, setPeriodPage] = useState(1),
    [modelPage, setModelPage] = useState(1);
  const usage = useOwnedTokenUsage(tokenID, periodPage, modelPage);
  if (usage.isError)
    return (
      <section role="alert">
        <h3>{text("额度与用量", "Quota and usage", "الحصة والاستخدام")}</h3>
        <p>
          {text(
            "用量暂不可用，请重新读取。",
            "Usage is unavailable. Reload to retry.",
            "الاستخدام غير متاح. أعد التحميل للمحاولة مجددًا.",
          )}
        </p>
        <Button size="small" onClick={() => void usage.refetch()}>
          {text("重新读取", "Reload", "إعادة التحميل")}
        </Button>
      </section>
    );
  if (!usage.data)
    return (
      <section aria-live="polite">
        <h3>{text("额度与用量", "Quota and usage", "الحصة والاستخدام")}</h3>
        <p>{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>
      </section>
    );
  const data = usage.data,
    current = data.current;
  const date = (stamp: number) =>
    new Intl.DateTimeFormat(locale, {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: current.timezone,
    }).format(new Date(stamp * 1000));
  const max = Math.max(1, ...data.trend.map((day) => day.quota));
  const label =
    current.period === "daily"
      ? text("每日重置", "Resets daily", "إعادة يومية")
      : current.period === "monthly"
        ? text("每月重置", "Resets monthly", "إعادة شهرية")
        : text("累计，不重置", "Lifetime, no reset", "تراكمي دون إعادة");
  return (
    <>
      <section data-token-quota-details={tokenID}>
        <h3>{text("消费额度", "Spending limit", "حد الإنفاق")}</h3>
        <dl>
          <div>
            <dt>
              {text("当前周期已用", "Used this period", "المستخدم خلال الفترة")}
            </dt>
            <dd dir="ltr">{money(current.used_quota)}</dd>
          </div>
          <div>
            <dt>{text("额度上限", "Limit", "الحد")}</dt>
            <dd>
              {current.quota_limit === null ? (
                text("不限额", "Unlimited", "بلا حد")
              ) : (
                <bdi dir="ltr">{money(current.quota_limit)}</bdi>
              )}
            </dd>
          </div>
          <div>
            <dt>
              {text("在途预扣", "Pending reservations", "الحجوزات المعلقة")}
            </dt>
            <dd dir="ltr">{money(current.reserved_quota)}</dd>
          </div>
          {current.remaining_quota !== null && (
            <div>
              <dt>{text("可用额度", "Available", "المتاح")}</dt>
              <dd dir="ltr">{money(current.remaining_quota)}</dd>
            </div>
          )}
          <div>
            <dt>{text("累计消费", "Lifetime spending", "إجمالي الإنفاق")}</dt>
            <dd dir="ltr">{money(current.lifetime_used_quota)}</dd>
          </div>
          <div>
            <dt>{text("额度周期", "Quota period", "فترة الحصة")}</dt>
            <dd>{label}</dd>
          </div>
          {current.end > 0 && (
            <div>
              <dt>{text("下次重置", "Next reset", "إعادة التعيين التالية")}</dt>
              <dd>{date(current.end)}</dd>
            </div>
          )}
          <div>
            <dt>{text("时区", "Timezone", "المنطقة الزمنية")}</dt>
            <dd>
              <bdi>{current.timezone}</bdi>
            </dd>
          </div>
        </dl>
        <p className="key-detail-hint">
          {text(
            "已用为实际结算金额，可用额度已扣除在途预扣。周期重置保留累计消费。",
            "Usage shows settled charges. Available quota excludes pending reservations. Lifetime spending is retained after reset.",
            "يعرض الاستخدام الرسوم المسوّاة. لا تشمل الحصة المتاحة الحجوزات المعلقة. يُحتفظ بإجمالي الإنفاق بعد إعادة التعيين.",
          )}
        </p>
      </section>
      <section data-key-usage-trend>
        <h3>
          {text("近 30 天消费趋势", "30-day spending", "الإنفاق خلال 30 يومًا")}
        </h3>
        <figure className="key-usage-chart">
          <div
            className="key-usage-bars"
            role="img"
            aria-label={text(
              "近 30 天每日消费，准确数值见每日明细。",
              "Daily spending over 30 days. Exact values are available in daily details.",
              "الإنفاق اليومي خلال 30 يومًا. القيم الدقيقة متاحة في التفاصيل اليومية.",
            )}
          >
            {data.trend.map((day) => (
              <div
                key={day.date}
                title={`${day.date}: ${money(day.quota)}`}
                aria-hidden="true"
                style={{
                  height:
                    day.quota > 0
                      ? `${Math.max(2, (day.quota / max) * 100)}%`
                      : "2px",
                }}
                className={day.quota === 0 ? "is-empty" : ""}
              />
            ))}
          </div>
          <figcaption dir="ltr">
            <span>{data.trend[0].date}</span>
            <span>{data.trend[29].date}</span>
          </figcaption>
        </figure>
        <p className="key-detail-hint">
          {text(
            "按当前额度时区和请求进入时间统计；旧记录缺少进入时间时使用完成时间。",
            "Grouped in the current quota timezone by admission time; legacy records without admission timestamps use completion time.",
            "مجمّع حسب وقت القبول في المنطقة الزمنية الحالية للحصة؛ تستخدم السجلات القديمة وقت الإكمال عند غياب وقت القبول.",
          )}
        </p>
        <details className="key-usage-daily">
          <summary>
            {text("每日明细", "Daily details", "التفاصيل اليومية")}
          </summary>
          <div
            className="key-usage-scroll"
            tabIndex={0}
            role="region"
            aria-label={text(
              "每日用量表",
              "Daily usage table",
              "جدول الاستخدام اليومي",
            )}
          >
            <table className="key-usage-table">
              <thead>
                <tr>
                  <th>{text("日期", "Date", "التاريخ")}</th>
                  <th>{text("请求数", "Requests", "الطلبات")}</th>
                  <th>{text("消费", "Spending", "الإنفاق")}</th>
                  <th>{text("失败/取消", "Failed/cancelled", "فاشل/ملغى")}</th>
                </tr>
              </thead>
              <tbody>
                {data.trend.map((day) => (
                  <tr key={day.date}>
                    <td>
                      <bdi>{day.date}</bdi>
                    </td>
                    <td>{day.requests}</td>
                    <td>
                      <bdi>{money(day.quota)}</bdi>
                    </td>
                    <td>{day.failed_count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      </section>
      <section data-key-model-usage>
        <h3>
          {text(
            "近 30 天模型拆分",
            "30-day model breakdown",
            "تفصيل النماذج خلال 30 يومًا",
          )}
        </h3>
        {data.models.length ? (
          <div
            className="key-usage-scroll"
            tabIndex={0}
            role="region"
            aria-label={text(
              "模型用量表",
              "Model usage table",
              "جدول استخدام النماذج",
            )}
          >
            <table className="key-usage-table">
              <thead>
                <tr>
                  <th>{text("模型", "Model", "النموذج")}</th>
                  <th>{text("请求数", "Requests", "الطلبات")}</th>
                  <th>{text("消费", "Spending", "الإنفاق")}</th>
                  <th>{text("令牌数", "Tokens", "الرموز")}</th>
                </tr>
              </thead>
              <tbody>
                {data.models.map((row) => (
                  <tr key={row.model}>
                    <td>
                      <code dir="ltr">{row.model || "—"}</code>
                    </td>
                    <td>{row.requests}</td>
                    <td>
                      <bdi>{money(row.quota)}</bdi>
                    </td>
                    <td>
                      {(
                        BigInt(row.prompt_tokens) +
                        BigInt(row.completion_tokens)
                      ).toLocaleString(locale)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="key-detail-hint">
            {text(
              "近 30 天暂无调用。",
              "No calls in the last 30 days.",
              "لا توجد استدعاءات خلال آخر 30 يومًا.",
            )}
          </p>
        )}
        <Pages
          page={data.model_page}
          total={data.model_total}
          size={data.page_size}
          onPage={setModelPage}
          text={text}
        />
      </section>
      <section data-key-period-history>
        <h3>{text("周期历史", "Period history", "سجل الفترات")}</h3>
        {data.periods.length ? (
          <div
            className="key-usage-scroll"
            tabIndex={0}
            role="region"
            aria-label={text(
              "周期历史表",
              "Quota period history table",
              "جدول سجل فترات الحصة",
            )}
          >
            <table className="key-usage-table">
              <thead>
                <tr>
                  <th>{text("周期开始", "Period start", "بداية الفترة")}</th>
                  <th>{text("已用", "Used", "المستخدم")}</th>
                  <th>{text("上限", "Limit", "الحد")}</th>
                  <th>{text("预扣", "Reserved", "المحجوز")}</th>
                </tr>
              </thead>
              <tbody>
                {data.periods.map((row) => (
                  <tr key={row.id}>
                    <td>
                      <bdi>
                        {new Intl.DateTimeFormat(locale, {
                          dateStyle: "medium",
                          timeZone: row.timezone,
                        }).format(new Date(row.start * 1000))}
                      </bdi>
                      <small>
                        {row.id === current.period_id
                          ? text("当前周期", "Current period", "الفترة الحالية")
                          : row.period === "daily"
                            ? text("日周期", "Daily", "يومي")
                            : text("月周期", "Monthly", "شهري")}
                      </small>
                      {row.carry_quota > 0 && (
                        <small>
                          {text(
                            "结转已用",
                            "Carried usage",
                            "الاستخدام المرحّل",
                          )}{" "}
                          <bdi>{money(row.carry_quota)}</bdi>
                        </small>
                      )}
                    </td>
                    <td>
                      <bdi>{money(row.used_quota)}</bdi>
                    </td>
                    <td>
                      <bdi>{money(row.quota_limit)}</bdi>
                    </td>
                    <td>
                      <bdi>{money(row.reserved_quota)}</bdi>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="key-detail-hint">
            {text(
              "尚无重置周期记录。",
              "No reset periods recorded yet.",
              "لم تُسجّل فترات إعادة تعيين بعد.",
            )}
          </p>
        )}
        <p className="key-detail-hint">
          {text(
            "在途请求按原周期结算，历史金额可能随后补齐。结转金额不重复计入累计消费。",
            "Pending requests settle in their original period, so historical charges can arrive later. Carried usage is not counted again in lifetime spending.",
            "تُسوّى الطلبات المعلقة في فترتها الأصلية، لذا قد تصل رسوم السجل لاحقًا. لا يُحتسب الاستخدام المرحّل مجددًا ضمن إجمالي الإنفاق.",
          )}
        </p>
        <Pages
          page={data.period_page}
          total={data.period_total}
          size={data.page_size}
          onPage={setPeriodPage}
          text={text}
        />
      </section>
    </>
  );
}
