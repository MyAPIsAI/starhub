import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { RefreshCw } from "lucide-react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { useStaffText } from "../lib/api/staff";
import { usePreferences } from "../lib/preferences";
import { formatMoney } from "../lib/money";
import { callTypeLabel } from "../lib/request-metadata";
import {
  operatingChart,
  formatOperatingAmount,
  operatingShare,
  useOperatingDashboard,
  type OperatingBucket,
} from "../lib/api/operating-dashboard";
import "./operating-dashboard.css";

const colors = [
  "#6366f1",
  "#f07838",
  "#16a886",
  "#e8a109",
  "#3b82f6",
  "#bf6ae0",
  "#db6387",
  "#8592a8",
];

function Distribution({
  title,
  rows,
  total,
  label,
}: {
  title: string;
  rows: OperatingBucket[];
  total: string;
  label: (row: OperatingBucket) => string;
}) {
  const t = useStaffText();
  const [expanded, setExpanded] = useState(false);
  const positive = rows.filter((row) => BigInt(row.amount_nano) > 0n);
  const visible = expanded ? positive : positive.slice(0, 5);
  const remaining = positive
    .slice(5)
    .reduce((sum, row) => sum + BigInt(row.amount_nano), 0n)
    .toString();
  const row = (name: string, amount: string, key: string, other = false) => (
    <div className={`op-distribution-row${other ? " is-other" : ""}`} key={key}>
      <div>
        <span>{name}</span>
        <bdi title={formatMoney(amount, "detail")}>
          {formatOperatingAmount(amount)}
        </bdi>
        <bdi>{operatingShare(amount, total).toFixed(1)}%</bdi>
      </div>
      <div className="op-share-track" aria-hidden="true">
        <span style={{ width: `${operatingShare(amount, total)}%` }} />
      </div>
    </div>
  );
  return (
    <article className="op-card op-distribution">
      <h2>{title}</h2>
      {!positive.length && (
        <p className="op-empty">
          {t("暂无消耗数据", "No consumption yet", "لا توجد بيانات استهلاك")}
        </p>
      )}
      {visible.map((item) => row(label(item), item.amount_nano, item.id))}
      {!expanded &&
        positive.length > 5 &&
        row(
          t(
            `其他 ${positive.length - 5} 项`,
            `${positive.length - 5} other items`,
            `${positive.length - 5} عناصر أخرى`,
          ),
          remaining,
          "__remaining",
          true,
        )}
      {positive.length > 5 && (
        <Button
          variant="ghost"
          size="small"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded
            ? t("收起", "Show less", "عرض أقل")
            : t("查看全部", "Show all", "عرض الكل")}
        </Button>
      )}
    </article>
  );
}

export function OperatingDashboard() {
  const t = useStaffText(),
    language = usePreferences((s) => s.language);
  const [params, setParams] = useSearchParams();
  const rawRange = params.get("range") || "7d",
    range = ["today", "7d", "30d", "custom"].includes(rawRange)
      ? rawRange
      : "7d";
  const [startDraft, setStartDraft] = useState(params.get("start_date") || ""),
    [endDraft, setEndDraft] = useState(params.get("end_date") || "");
  const [showModels, setShowModels] = useState(false);
  const [dateError, setDateError] = useState(false);
  const request = new URLSearchParams({ range });
  if (range === "custom") {
    request.set("start_date", params.get("start_date") || "");
    request.set("end_date", params.get("end_date") || "");
  }
  const valid =
    range !== "custom" ||
    Boolean(request.get("start_date") && request.get("end_date"));
  const query = useOperatingDashboard(request.toString(), valid);
  const data = query.error ? undefined : query.data;
  const chart = data ? operatingChart(data) : undefined;
  const label = (row: OperatingBucket) =>
    row.name || row.id || t("未记录", "Not recorded", "غير مسجل");
  const updateRange = (value: string) => {
    setDateError(false);
    setParams((old) => {
      const next = new URLSearchParams(old);
      next.set("range", value);
      if (value !== "custom") {
        next.delete("start_date");
        next.delete("end_date");
      }
      return next;
    });
  };
  const applyDates = (event: React.FormEvent) => {
    event.preventDefault();
    if (!startDraft || !endDraft || startDraft > endDraft) {
      setDateError(true);
      return;
    }
    setDateError(false);
    setParams((old) => {
      const next = new URLSearchParams(old);
      next.set("range", "custom");
      next.set("start_date", startDraft);
      next.set("end_date", endDraft);
      return next;
    });
  };
  const formatTime = (at: number, axis = false) => {
    const value = new Intl.DateTimeFormat(language, {
      timeZone: data?.timezone || "UTC",
      ...(axis
        ? data?.granularity === "hour"
          ? { hour: "2-digit" as const, minute: "2-digit" as const }
          : { month: "numeric" as const, day: "numeric" as const }
        : { dateStyle: "short" as const, timeStyle: "short" as const }),
    }).format(new Date(at * 1000));
    return axis ? value.replace(/[\u200e\u200f]/g, "") : value;
  };
  const rank = (title: string, rows: OperatingBucket[], accounts: boolean) => (
    <article className="op-card op-ranking">
      <header>
        <h2>{title}</h2>
        <p>
          {t(
            "前 20 名 · 按已结算消耗排序",
            "Top 20 · settled consumption",
            "أفضل 20 حسب الاستهلاك المسوّى",
          )}
        </p>
      </header>
      {!rows.some((row) => BigInt(row.amount_nano) > 0n) ? (
        <p className="op-empty">
          {t(
            "暂无消耗排名",
            "No consumption ranking yet",
            "لا يوجد ترتيب للاستهلاك بعد",
          )}
        </p>
      ) : (
        <ol>
          {rows
            .filter((row) => BigInt(row.amount_nano) > 0n)
            .map((row, index) => (
              <li key={row.id}>
                <span className="op-rank-number">{index + 1}</span>
                {accounts ? (
                  <Link
                    to={`/admin?tab=customers&customer=${encodeURIComponent(row.id)}`}
                  >
                    <bdi>{label(row)}</bdi>
                  </Link>
                ) : (
                  <span>
                    <bdi>{label(row)}</bdi>
                    {row.name && row.name !== row.id && (
                      <small>
                        <bdi>{row.id}</bdi>
                      </small>
                    )}
                  </span>
                )}
                <bdi
                  className="op-money"
                  title={formatMoney(row.amount_nano, "detail")}
                >
                  {formatOperatingAmount(row.amount_nano)}
                </bdi>
              </li>
            ))}
        </ol>
      )}
    </article>
  );
  return (
    <section className="op-dashboard">
      <header className="op-heading">
        <div>
          <h1>{t("运营看板", "Operations dashboard", "لوحة العمليات")}</h1>
          <p>
            {t(
              "客户经营概览",
              "Customer business overview",
              "نظرة عامة على نشاط العملاء",
            )}
            {data && (
              <>
                {" "}
                · <bdi>{data.region_id}</bdi> · <bdi>{data.timezone}</bdi>
              </>
            )}
          </p>
        </div>
        <div className="op-toolbar">
          <Button
            variant="ghost"
            size="small"
            disabled={query.isFetching || !valid}
            onClick={() => void query.refetch()}
            aria-label={t(
              "刷新经营数据",
              "Refresh dashboard",
              "تحديث لوحة العمليات",
            )}
          >
            <RefreshCw
              size={14}
              className={query.isFetching ? "op-spinning" : ""}
            />
            {t("刷新", "Refresh", "تحديث")}
          </Button>
          <div
            className="op-periods"
            role="group"
            aria-label={t("统计时间范围", "Reporting period", "فترة التقرير")}
          >
            {[
              ["today", t("今天", "Today", "اليوم")],
              ["7d", t("近 7 天", "7 days", "7 أيام")],
              ["30d", t("近 30 天", "30 days", "30 يومًا")],
              ["custom", t("自定义", "Custom", "مخصص")],
            ].map(([value, name]) => (
              <button
                key={value}
                aria-pressed={range === value}
                onClick={() => updateRange(value)}
              >
                {name}
              </button>
            ))}
          </div>
        </div>
      </header>
      {range === "custom" && (
        <form className="op-custom" onSubmit={applyDates}>
          <label>
            {t("开始日期", "Start date", "تاريخ البداية")}
            <Input
              type="date"
              required
              value={startDraft}
              max={endDraft || undefined}
              onChange={(e) => setStartDraft(e.target.value)}
            />
          </label>
          <label>
            {t("结束日期", "End date", "تاريخ النهاية")}
            <Input
              type="date"
              required
              value={endDraft}
              min={startDraft || undefined}
              onChange={(e) => setEndDraft(e.target.value)}
            />
          </label>
          <Button type="submit" variant="secondary">
            {t("应用", "Apply", "تطبيق")}
          </Button>
          <span>
            {t(
              "按地区时区统计，最多一年。",
              "Regional timezone, up to one year.",
              "حسب توقيت المنطقة، حتى سنة واحدة.",
            )}
          </span>
        </form>
      )}
      {dateError && (
        <p role="alert">
          {t(
            "请选择有效的开始和结束日期。",
            "Select valid start and end dates.",
            "اختر تاريخي بداية ونهاية صالحين.",
          )}
        </p>
      )}
      {query.isPending && valid && (
        <p role="status" className="op-empty">
          {t(
            "正在汇总经营数据…",
            "Loading business data…",
            "جارٍ تحميل بيانات النشاط…",
          )}
        </p>
      )}
      {query.error && (
        <div role="alert" className="op-error">
          {t(
            "经营数据暂时无法读取，请检查日期范围或重试。",
            "Business data is unavailable. Check the date range or retry.",
            "بيانات النشاط غير متاحة. تحقق من الفترة أو أعد المحاولة.",
          )}
          <Button variant="secondary" onClick={() => void query.refetch()}>
            {t("重试", "Retry", "إعادة المحاولة")}
          </Button>
        </div>
      )}
      {data && chart && (
        <>
          <div className="op-data-time">
            <span>
              <bdi>{data.start_date}</bdi> — <bdi>{data.end_date}</bdi>
            </span>
            <span>
              {t(
                "每 5 分钟更新 · 数据更新于",
                "Every 5 minutes · Updated",
                "كل 5 دقائق · آخر تحديث",
              )}{" "}
              <bdi>{formatTime(data.updated_at)}</bdi>
            </span>
          </div>
          <div className="op-kpis">
            {[
              [
                t("充值金额", "Top-ups", "مبلغ الشحن"),
                formatOperatingAmount(data.totals.recharge_nano),
                t(
                  "已登记的线下收款；其他资金来源见统计口径。",
                  "Registered offline receipts; see funding details below.",
                  "المبالغ المسجلة خارج الإنترنت؛ انظر تفاصيل التمويل أدناه.",
                ),
              ],
              [
                t("消耗金额", "Consumption", "مبلغ الاستهلاك"),
                formatOperatingAmount(data.totals.consumption_nano),
                t(
                  "调用明细中的实际结算金额。",
                  "Actual settled charges from call records.",
                  "الرسوم المسوّاة الفعلية من سجلات الطلبات.",
                ),
              ],
              [
                t("调用次数", "Calls", "عدد الطلبات"),
                data.totals.requests.toLocaleString(language),
                t(
                  "已完成并记录的调用，含失败和取消；内部重试不重复计数。",
                  "Recorded completed calls, including failures and cancellations; retries are not counted twice.",
                  "الطلبات المكتملة المسجلة بما فيها الفشل والإلغاء؛ دون تكرار محاولات الإعادة.",
                ),
              ],
              [
                t(
                  "客户最终成功率",
                  "Final success rate",
                  "معدل النجاح النهائي",
                ),
                data.totals.requests
                  ? `${((data.totals.successful / data.totals.requests) * 100).toFixed(2)}%`
                  : "—",
                t(
                  "成功调用 ÷ 已完成调用；进行中的异步任务不计入。",
                  "Successful / completed calls; pending asynchronous tasks are excluded.",
                  "الطلبات الناجحة ÷ المكتملة؛ دون المهام غير المتزامنة قيد التنفيذ.",
                ),
              ],
              [
                t("活跃账号", "Active accounts", "الحسابات النشطة"),
                data.totals.active_accounts.toLocaleString(language),
                t(
                  "所选时间内有调用的客户账号，去重统计。",
                  "Distinct customers with calls in this period.",
                  "عدد العملاء الفريدين ذوي الطلبات خلال الفترة.",
                ),
              ],
              [
                t("新注册账号", "New accounts", "الحسابات الجديدة"),
                data.totals.new_accounts.toLocaleString(language),
                t(
                  "所选时间内注册的客户账号。",
                  "Customers registered in this period.",
                  "العملاء المسجلون خلال الفترة.",
                ),
              ],
              [
                t("付费账号", "Paying accounts", "الحسابات الدافعة"),
                data.totals.paying_accounts.toLocaleString(language),
                t(
                  "所选时间内有登记收款的客户账号，去重统计。",
                  "Distinct customers with registered receipts in this period.",
                  "العملاء الفريدون ذوو المدفوعات المسجلة خلال الفترة.",
                ),
              ],
            ].map(([name, value, hint]) => (
              <article className="op-card op-kpi" key={name} title={hint}>
                <h2>{name}</h2>
                <strong>
                  <bdi>{value}</bdi>
                </strong>
              </article>
            ))}
          </div>
          <article className="op-card op-trend">
            <h2>{t("消耗趋势", "Consumption trend", "اتجاه الاستهلاك")}</h2>
            <p>
              {t(
                "按服务分组堆叠",
                "Stacked by service group",
                "مكدس حسب مجموعة الخدمة",
              )}
            </p>
            <div className="op-legend">
              {chart.groups.map((row, index) => (
                <span key={row.id}>
                  <i style={{ background: colors[index] }} />
                  <bdi>{label(row)}</bdi>
                </span>
              ))}
              {chart.hasOther && (
                <span>
                  <i style={{ background: colors[7] }} />
                  {t("其他", "Other", "أخرى")}
                </span>
              )}
            </div>
            {BigInt(data.totals.consumption_nano) === 0n ? (
              <div className="op-chart-empty">
                {t(
                  "该时间段暂无已结算消耗",
                  "No settled consumption in this period",
                  "لا يوجد استهلاك مسوّى خلال هذه الفترة",
                )}
              </div>
            ) : (
              <div className="op-chart" dir="ltr">
                <ResponsiveContainer width="100%" height={280}>
                  <BarChart
                    data={chart.points}
                    margin={{ top: 12, right: 12, left: 8, bottom: 4 }}
                    accessibilityLayer
                  >
                    <CartesianGrid
                      vertical={false}
                      stroke="var(--border)"
                      strokeDasharray="2 4"
                    />
                    <XAxis
                      dataKey="at"
                      tickFormatter={(value: number) => formatTime(value, true)}
                      tickLine={false}
                      axisLine={{ stroke: "var(--border)" }}
                      minTickGap={20}
                      tick={{ fill: "var(--secondary)", fontSize: 11 }}
                    />
                    <YAxis
                      tickFormatter={(value: number) =>
                        new Intl.NumberFormat(language, {
                          style: "currency",
                          currency: "USD",
                          notation: "compact",
                          maximumFractionDigits: 1,
                        }).format(value)
                      }
                      tickLine={false}
                      axisLine={false}
                      width={65}
                      tick={{ fill: "var(--secondary)", fontSize: 11 }}
                    />
                    <Tooltip
                      cursor={{ fill: "var(--subtle)" }}
                      content={({ active, payload }) => {
                        const point = payload?.[0]?.payload as
                          (typeof chart.points)[number] | undefined;
                        return active && point ? (
                          <div
                            className="op-chart-tooltip"
                            dir={language === "ar" ? "rtl" : "ltr"}
                          >
                            <strong>{formatTime(point.at)}</strong>
                            {chart.groups.map((row, index) => (
                              <div key={row.id}>
                                <span>{label(row)}</span>
                                <bdi>
                                  {formatMoney(
                                    point.exact[`g${index}`],
                                    "detail",
                                  )}
                                </bdi>
                              </div>
                            ))}
                            {chart.hasOther && (
                              <div>
                                <span>{t("其他", "Other", "أخرى")}</span>
                                <bdi>
                                  {formatMoney(point.exact.other, "detail")}
                                </bdi>
                              </div>
                            )}
                            <div>
                              <strong>{t("合计", "Total", "الإجمالي")}</strong>
                              <bdi>
                                {formatMoney(point.amount_nano, "detail")}
                              </bdi>
                            </div>
                          </div>
                        ) : null;
                      }}
                    />
                    {chart.groups.map((row, index) => (
                      <Bar
                        key={row.id}
                        dataKey={`g${index}`}
                        name={label(row)}
                        stackId="spend"
                        fill={colors[index]}
                        maxBarSize={40}
                        isAnimationActive={false}
                      />
                    ))}
                    {chart.hasOther && (
                      <Bar
                        dataKey="other"
                        stackId="spend"
                        fill={colors[7]}
                        maxBarSize={40}
                        isAnimationActive={false}
                      />
                    )}
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}
          </article>
          <div className="op-distributions">
            <Distribution
              title={t("按服务分组", "By service group", "حسب مجموعة الخدمة")}
              rows={data.groups}
              total={data.totals.consumption_nano}
              label={label}
            />
            <Distribution
              title={t("按厂商", "By vendor", "حسب المزوّد")}
              rows={data.vendors}
              total={data.totals.consumption_nano}
              label={label}
            />
            <Distribution
              title={t("按调用类型", "By call type", "حسب نوع الطلب")}
              rows={data.call_types}
              total={data.totals.consumption_nano}
              label={(row) =>
                row.id ? callTypeLabel(row.id, language) : label(row)
              }
            />
          </div>
          <div className="op-rankings">
            {rank(
              t(
                "消耗排名 · 账号",
                "Consumption ranking · accounts",
                "ترتيب الاستهلاك · الحسابات",
              ),
              data.top_accounts,
              true,
            )}
            <div>
              {rank(
                t(
                  "消耗排名 · 模型",
                  "Consumption ranking · models",
                  "ترتيب الاستهلاك · النماذج",
                ),
                data.top_models,
                false,
              )}
              {data.models.length > 0 && (
                <Button
                  variant="ghost"
                  onClick={() => setShowModels(!showModels)}
                >
                  {showModels
                    ? t(
                        "收起模型分布",
                        "Hide model distribution",
                        "إخفاء توزيع النماذج",
                      )
                    : t(
                        "查看全部模型分布",
                        "All model distribution",
                        "توزيع جميع النماذج",
                      )}
                </Button>
              )}
            </div>
          </div>
          {showModels && (
            <Distribution
              title={t("按模型", "By model", "حسب النموذج")}
              rows={data.models}
              total={data.totals.consumption_nano}
              label={label}
            />
          )}
          <details className="op-method">
            <summary>
              {t(
                "统计口径与资金来源",
                "Definitions and funding sources",
                "تعريف المقاييس ومصادر التمويل",
              )}
            </summary>
            <p>
              {t(
                "仅统计客户账号，排除内部员工。消耗按调用时的实际结算金额统计，不随当前价格变化。无调用时成功率显示为 —。",
                "Customer accounts only; internal employees are excluded. Consumption uses original settled charges, unaffected by current prices. No calls means no success-rate value.",
                "حسابات العملاء فقط، دون الموظفين الداخليين. يعتمد الاستهلاك على الرسوم المسوّاة الأصلية دون تأثير للأسعار الحالية. لا تُعرض نسبة نجاح عند غياب الطلبات.",
              )}
            </p>
            <p>
              {t(
                "充值金额与付费账号目前统计已登记的线下收款，尚未接入线上支付核验。以下模拟充值、未核验历史入账及赠送／补偿不计入充值指标，也不代表实际收入。",
                "Top-ups and paying accounts currently reflect registered offline receipts; online payment verification is not connected. Simulated top-ups, unverified historical credits and gifts/compensation below are excluded and do not represent revenue.",
                "يعكس الشحن والحسابات الدافعة الإيصالات المسجلة خارج الإنترنت؛ التحقق من الدفع الإلكتروني غير متصل. تُستبعد عمليات الشحن التجريبية والأرصدة التاريخية غير المتحققة والهدايا والتعويضات ولا تمثل إيرادات.",
              )}
            </p>
            <dl>
              {[
                [
                  t(
                    "登记线下收款",
                    "Registered offline receipts",
                    "إيصالات مسجلة خارج الإنترنت",
                  ),
                  data.funding.offline_nano,
                ],
                [
                  t("模拟充值", "Simulated top-ups", "شحن تجريبي"),
                  data.funding.simulation_nano,
                ],
                [
                  t(
                    "未核验历史入账",
                    "Unverified historical credits",
                    "أرصدة تاريخية غير متحققة",
                  ),
                  data.funding.unverified_nano,
                ],
                [
                  t(
                    "赠送／补偿等加款",
                    "Gifts / compensation credits",
                    "هدايا وتعويضات",
                  ),
                  data.funding.other_credit_nano,
                ],
              ].map(([name, value]) => (
                <div key={name}>
                  <dt>{name}</dt>
                  <dd>
                    <bdi>{formatOperatingAmount(value)}</bdi>
                  </dd>
                </div>
              ))}
            </dl>
          </details>
        </>
      )}
    </section>
  );
}
