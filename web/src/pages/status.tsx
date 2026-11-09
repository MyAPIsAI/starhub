import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  Activity,
  CheckCircle2,
  AlertCircle,
  HelpCircle,
  RefreshCw,
  Search,
} from "lucide-react";
import { PublicHeader } from "../components/product-shell";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ListPagination } from "../components/list-pagination";
import { PublicAnnouncements } from "../features/public-announcements";
import { usePublishedAnnouncements } from "../lib/api/announcements";
import {
  usePublicStatus,
  usePublicStatusHours,
  type PublicStatusModel,
} from "../lib/api/public-pages";
import { regionalLocale } from "../lib/locale-format";
import { statusPeriod } from "../lib/public-status";
import "./public-data-pages.css";
import "./status-history.css";
const states = {
  normal: "publicStatusNormal",
  degraded: "publicStatusDegraded",
  outage: "publicStatusOutage",
  insufficient_data: "publicStatusInsufficient",
} as const;
const typeKeys: Record<string, string> = {
  text: "statusTypeText",
  embedding: "statusTypeEmbedding",
  rerank: "statusTypeRerank",
  image: "statusTypeImage",
  audio: "statusTypeAudio",
  video: "statusTypeVideo",
};
const rate = (value: number | null) =>
  value === null ? "—" : `${(value * 100).toFixed(2)}%`;
const latency = (value: number | null) =>
  value === null
    ? "—"
    : value >= 1000
      ? `${(value / 1000).toFixed(2)} s`
      : `${value} ms`;
type Selection = { model: PublicStatusModel; date: string };
function StatusModel({
  model,
  days,
  onInspect,
}: {
  model: PublicStatusModel;
  days: number;
  onInspect: (selection: Selection) => void;
}) {
  const { t, i18n } = useTranslation();
  const daily = model.daily.slice(-days),
    period = statusPeriod(daily),
    metrics = model.last_24_hours;
  const [active, setActive] = useState(days - 1);
  useEffect(() => setActive(days - 1), [days]);
  const index = Math.min(active, daily.length - 1);
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const Icon =
    model.current.state === "normal"
      ? CheckCircle2
      : model.current.state === "insufficient_data"
        ? HelpCircle
        : AlertCircle;
  const describe = (d: (typeof daily)[number]) =>
    `${d.date} · ${t(states[d.state])} · ${rate(d.success_rate)} · ${t("publicStatusSamples", { count: d.samples })}`;
  const navigate = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const rtl = i18n.dir() === "rtl";
    const next =
      e.key === "Home"
        ? 0
        : e.key === "End"
          ? daily.length - 1
          : e.key === "ArrowRight"
            ? i + (rtl ? -1 : 1)
            : e.key === "ArrowLeft"
              ? i + (rtl ? 1 : -1)
              : null;
    if (next === null) return;
    e.preventDefault();
    const clamped = Math.max(0, Math.min(next, daily.length - 1));
    setActive(clamped);
    buttons.current[clamped]?.focus();
  };
  return (
    <article
      className="public-status-model"
      data-status-model={model.id}
      data-state={model.current.state}
    >
      <header>
        <h3>
          <Link to={`/models/${encodeURIComponent(model.id)}`} title={model.id}>
            {model.name}
          </Link>
        </h3>
        <div className="status-row-metrics">
          <div className="status-current">
            <small>{t("statusRecent15")}</small>
            <span
              className={`public-state is-${model.current.state}`}
              title={t("publicStatusSamples", { count: model.current.samples })}
            >
              <Icon size={13} />
              {t(states[model.current.state])}
            </span>
          </div>
          <dl className="public-status-metrics">
            <div>
              <dt>{t("publicStatusSuccess24")}</dt>
              <dd>
                <bdi>{rate(metrics.success_rate)}</bdi>
              </dd>
            </div>
            {model.tags.includes("text") && (
              <div className="status-latency">
                <dt>{t("statusLatency24")}</dt>
                <dd>
                  <span title={t("publicStatusP50")}>
                    P50 <bdi>{latency(metrics.ttft_p50_ms)}</bdi>
                  </span>
                  <span className="status-metric-separator">/</span>
                  <span title={t("publicStatusP95")}>
                    P95 <bdi>{latency(metrics.ttft_p95_ms)}</bdi>
                  </span>
                </dd>
              </div>
            )}
          </dl>
        </div>
      </header>
      <div
        className="public-daily-bars"
        role="group"
        aria-label={t("statusDayKeyboard")}
        style={{ "--history-days": days } as CSSProperties}
      >
        {daily.map((d, i) => (
          <button
            key={d.date}
            ref={(el) => {
              buttons.current[i] = el;
            }}
            type="button"
            className={`is-${d.state}`}
            title={describe(d)}
            aria-label={describe(d)}
            tabIndex={i === index ? 0 : -1}
            onFocus={() => setActive(i)}
            onKeyDown={(e) => navigate(e, i)}
            onClick={() => onInspect({ model, date: d.date })}
          />
        ))}
      </div>
      <div className="status-period">
        <time>{daily[0]?.date}</time>
        <span>
          {t("statusPeriodRate")} <bdi>{rate(period.successRate)}</bdi>
        </span>
        <time>{daily.at(-1)?.date}</time>
      </div>
      <div className="status-history-actions">
        <label className="status-date-select">
          <span>{t("statusDayHint")}</span>
          <select
            aria-label={t("statusPickDay")}
            value={daily[index]?.date ?? ""}
            onChange={(e) => {
              setActive(daily.findIndex((day) => day.date === e.target.value));
              onInspect({ model, date: e.target.value });
            }}
          >
            {daily.map((d) => (
              <option key={d.date} value={d.date}>
                {d.date}
              </option>
            ))}
          </select>
        </label>
        <button
          className="status-text-button"
          onClick={() => onInspect({ model, date: "" })}
        >
          <Activity size={14} />
          {t("statusLast24")}
        </button>
      </div>
    </article>
  );
}
function StatusHoursDialog({
  selection,
  onClose,
}: {
  selection: Selection | null;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const query = usePublicStatusHours(
    selection?.model.id ?? "",
    selection?.date ?? "",
    !!selection,
  );
  const data = query.isError ? undefined : query.data;
  const [active, setActive] = useState(0);
  useEffect(() => setActive(0), [selection?.model.id, selection?.date]);
  const hour = data?.history.hours[active];
  const format = (stamp: number) =>
    new Intl.DateTimeFormat(regionalLocale(i18n.language), {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
      timeZone: data?.timezone,
      timeZoneName: "shortOffset",
    }).format(new Date(stamp * 1000));
  return (
    <Modal
      open={!!selection}
      onOpenChange={(open) => !open && onClose()}
      title={`${selection?.model.name ?? ""} · ${t("statusHourlyTitle")}`}
      description={selection?.date || t("statusLast24")}
      className="status-hour-dialog"
      footer={<Button onClick={onClose}>{t("close")}</Button>}
    >
      {query.isFetching && <p role="status">{t("loading")}</p>}
      {query.isError && (
        <div role="alert">
          <p>{t("publicStatusError")}</p>
          <Button onClick={() => void query.refetch()}>{t("retry")}</Button>
        </div>
      )}
      {data && (
        <>
          <p className="status-hour-range">
            <bdi>
              {format(data.history.start)} — {format(data.history.end)}
            </bdi>
            <br />
            <bdi>{data.timezone}</bdi>
          </p>
          <div className="status-hour-summary">
            <span>
              {t("statusPeriodRate")}{" "}
              <strong>{rate(data.history.success_rate)}</strong>
            </span>
            <span>
              {t("publicStatusSamples", { count: data.history.samples })}
            </span>
          </div>
          <p className="public-muted">{t("statusHourHint")}</p>
          <div
            className="status-hour-bars"
            style={
              { "--hour-count": data.history.hours.length } as CSSProperties
            }
          >
            {data.history.hours.map((h, i) => (
              <button
                key={h.start}
                aria-pressed={i === active}
                aria-label={`${format(h.start)} · ${rate(h.success_rate)} · ${t("publicStatusSamples", { count: h.samples })}`}
                onClick={() => setActive(i)}
                className={h.samples ? "has-samples" : "is-empty"}
              >
                <span
                  style={{
                    height:
                      h.success_rate === null
                        ? "0"
                        : `${h.success_rate * 100}%`,
                  }}
                />
                <small>
                  {new Intl.DateTimeFormat("en", {
                    hour: "2-digit",
                    hourCycle: "h23",
                    timeZone: data.timezone,
                  }).format(new Date(h.start * 1000))}
                </small>
              </button>
            ))}
          </div>
          {hour && (
            <div className="status-hour-selected" aria-live="polite">
              <strong>
                <bdi>
                  {format(hour.start)} — {format(hour.end)}
                </bdi>
              </strong>
              <span>
                {t("statusHourSuccess")} <bdi>{rate(hour.success_rate)}</bdi> ·{" "}
                {t("publicStatusSamples", { count: hour.samples })}
              </span>
              {!hour.samples && <p>{t("statusHourNoData")}</p>}
            </div>
          )}
        </>
      )}
    </Modal>
  );
}
export default function StatusPage() {
  const { t, i18n } = useTranslation();
  const announcements = usePublishedAnnouncements();
  const [page, setPage] = useState(1),
    [search, setSearch] = useState(""),
    [queryText, setQueryText] = useState(""),
    [vendor, setVendor] = useState(""),
    [type, setType] = useState(""),
    [state, setState] = useState(""),
    [days, setDays] = useState(30),
    [selection, setSelection] = useState<Selection | null>(null);
  useEffect(() => {
    if (search === queryText) return;
    const timer = setTimeout(() => {
      setQueryText(search);
      setPage(1);
    }, 250);
    return () => clearTimeout(timer);
  }, [search, queryText]);
  const query = usePublicStatus(page, queryText, vendor, type, state),
    data = query.isError ? undefined : query.data;
  useEffect(() => {
    if (data && page > Math.max(1, Math.ceil(data.total / 20)))
      setPage(Math.max(1, Math.ceil(data.total / 20)));
  }, [data, page]);
  const vendors = [...new Set(data?.items.map((m) => m.vendor) ?? [])];
  const reset = () => {
    setSearch("");
    setQueryText("");
    setVendor("");
    setType("");
    setState("");
    setPage(1);
  };
  return (
    <div className="public-status status-demo">
      <PublicHeader />
      <main className="public-page-width status-content">
        <header className="status-page-heading">
          <div>
            <h1>{t("publicHomeStatus")}</h1>
            <p>{t("publicStatusIntro")}</p>
          </div>
          <div className="status-update">
            {data && (
              <span>
                {t("publicStatusUpdated")}{" "}
                <time dateTime={new Date(data.at * 1000).toISOString()}>
                  {new Intl.DateTimeFormat(regionalLocale(i18n.language), {
                    month: "2-digit",
                    day: "2-digit",
                    hour: "2-digit",
                    minute: "2-digit",
                    timeZone: data.timezone,
                  }).format(new Date(data.at * 1000))}
                </time>{" "}
                · <bdi>{data.timezone}</bdi>
              </span>
            )}
            <Button
              variant="secondary"
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
            >
              <RefreshCw size={14} />
              {t("publicStatusRefresh")}
            </Button>
          </div>
        </header>
        <PublicAnnouncements query={announcements} timezone={data?.timezone} />
        <section
          className="status-history-section"
          aria-labelledby="status-history-title"
        >
          <div className="status-history-heading">
            <div>
              <h2 id="status-history-title">{t("statusDailyTitle")}</h2>
              <p>{t("statusDailySubtitle")}</p>
            </div>
            <div className="status-segments" aria-label={t("statusDateRange")}>
              {[30, 90].map((n) => (
                <button
                  key={n}
                  aria-pressed={days === n}
                  onClick={() => setDays(n)}
                >
                  {t(n === 30 ? "statusDays30" : "statusDays90")}
                </button>
              ))}
            </div>
          </div>
          <div className="status-type-filters">
            <span>{t("statusModelType")}</span>
            <button
              aria-pressed={!type}
              onClick={() => {
                setType("");
                setPage(1);
              }}
            >
              {t("statusAllTypes")}{" "}
              <bdi>{data?.catalog_total ?? data?.total ?? 0}</bdi>
            </button>
            {(data?.types ?? []).map((item) => (
              <button
                key={item.id}
                aria-pressed={type === item.id}
                onClick={() => {
                  setType(item.id);
                  setPage(1);
                }}
              >
                {t(typeKeys[item.id] ?? item.id)} <bdi>{item.count}</bdi>
              </button>
            ))}
          </div>
          <div className="public-status-filters">
            <label className="status-search">
              <Search size={16} />
              <input
                aria-label={t("publicStatusSearch")}
                placeholder={t("statusSearchPlaceholder")}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </label>
            <div
              className="status-segments status-state-filters"
              aria-label={t("statusStateFilter")}
            >
              {[
                ["", "statusAllModels"],
                ["abnormal", "statusAbnormal"],
                ["unknown", "statusPending"],
              ].map(([id, key]) => (
                <button
                  key={id}
                  aria-pressed={state === id}
                  onClick={() => {
                    setState(id);
                    setPage(1);
                  }}
                >
                  {t(key)}
                </button>
              ))}
            </div>
            <select
              aria-label={t("publicStatusVendor")}
              value={vendor}
              onChange={(e) => {
                setVendor(e.target.value);
                setPage(1);
              }}
            >
              <option value="">{t("publicStatusAllVendors")}</option>
              {(data?.vendors ?? (vendor ? [vendor] : [])).map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
            <span className="status-model-count">
              {t("statusModelCount", { count: data?.total ?? 0 })}
            </span>
          </div>
          {query.isFetching && (
            <p role="status" className="public-muted">
              {t("loading")}
            </p>
          )}
          {query.isError && (
            <div className="public-status-empty" role="alert">
              <p>{t("publicStatusError")}</p>
              <Button onClick={() => void query.refetch()}>{t("retry")}</Button>
            </div>
          )}
          {data && !data.items.length && (
            <div className="public-status-empty">
              <h2>{t("publicStatusEmpty")}</h2>
              <p>{t("publicStatusEmptyHelp")}</p>
              {(search || vendor || type || state) && (
                <Button onClick={reset}>{t("publicStatusReset")}</Button>
              )}{" "}
              <Link to="/models">{t("publicHomeAllModels")}</Link>
            </div>
          )}
          {vendors.map((v, i) => (
            <section className="public-vendor-status" key={v}>
              <h2>
                <span
                  className="status-vendor-mark"
                  data-vendor={v.toLowerCase()}
                  style={
                    { "--vendor-hue": (i * 97 + 160) % 360 } as CSSProperties
                  }
                  aria-hidden="true"
                >
                  {v.slice(0, 1).toUpperCase()}
                </span>
                {v}
              </h2>
              <div className="status-vendor-models">
                {data?.items
                  .filter((m) => m.vendor === v)
                  .map((m) => (
                    <StatusModel
                      key={m.id}
                      model={m}
                      days={days}
                      onInspect={setSelection}
                    />
                  ))}
              </div>
            </section>
          ))}
          {data && (
            <ListPagination
              page={page}
              total={data.total}
              size={20}
              onChange={setPage}
            />
          )}
        </section>
        <details className="status-methodology">
          <summary>{t("statusMethodology")}</summary>
          <div className="public-status-legend">
            {Object.entries(states).map(([key, value]) => (
              <span className={`public-state is-${key}`} key={key}>
                {t(value)}
              </span>
            ))}
          </div>
          <p>{t("publicStatusScope")}</p>
          <p>{t("publicStatusLatencyScope")}</p>
          <p>{t("publicStatusDailyRule")}</p>
        </details>
        <StatusHoursDialog
          selection={selection}
          onClose={() => setSelection(null)}
        />
      </main>
    </div>
  );
}
