import { ResponsiveTable } from "../components/responsive-table";
import { message as translatedMessage } from "../i18n/messages";
import { useSearchParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { regionalLocale } from "../lib/locale-format";
import {
  CalendarDays,
  Download,
  Search,
  Check,
  CircleAlert,
  ChevronLeft,
  ChevronRight,
  RotateCcw,
  ArrowDown,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { TierBadge } from "../components/tier-badge";
import { formatMoney } from "../lib/money";
import type { PreviewRequest } from "../lib/product-preview";
import { callTypeLabel } from "../lib/request-metadata";
import {
  dubaiDate,
  requestTimeBounds,
  filterRequestHistory,
  type RequestTimeRange,
  type RequestHistoryStatus,
} from "../lib/request-history";
import "./request-history.css";

const ranges: RequestTimeRange[] = ["today", "7d", "30d", "custom", "all"];
const statuses: RequestHistoryStatus[] = [
  "all",
  "success",
  "failed",
  "cancelled",
];
const PAGE_SIZE = 20;
export default function RequestHistory({
  requests,
  timezone = "Asia/Dubai",
  serverPage,
  options,
  onSelect,
  onExport,
}: {
  requests: PreviewRequest[];
  timezone?: string;
  serverPage?: { total: number; loading: boolean; error?: string };
  options?: Record<string, { id: string; name: string }[]>;
  onSelect: (row: PreviewRequest) => void;
  onExport: (selection?: {
    start_date: string;
    end_date: string;
    status?: string;
    query?: string;
    key_id?: number;
    model?: string;
    trace_id?: string;
  }) => void;
}) {
  const { t, i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [params, setParams] = useSearchParams();
  const rawRange = params.get("range") as RequestTimeRange;
  const range = ranges.includes(rawRange) ? rawRange : "7d";
  const rawStatus = params.get("status") as RequestHistoryStatus;
  const status = statuses.includes(rawStatus) ? rawStatus : "all";
  const query = params.get("query") ?? "";
  const start = params.get("start") ?? "",
    end = params.get("end") ?? "";
  const localResult = filterRequestHistory(requests, {
    timezone,
    range,
    start,
    end,
    status,
    query,
  });
  const result = serverPage
    ? { rows: requests, error: localResult.error }
    : localResult;
  const count = serverPage ? serverPage.total : result.rows.length;
  const pages = Math.max(1, Math.ceil(count / PAGE_SIZE));
  const requestedPage = Number(params.get("page"));
  const page = Number.isInteger(requestedPage)
    ? Math.max(1, Math.min(pages, requestedPage))
    : 1;
  const visibleRows = serverPage
    ? requests
    : result.rows.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
  const change = (updates: Record<string, string | null>, replace = false) => {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        next.delete("page");
        Object.entries(updates).forEach(([key, value]) =>
          value ? next.set(key, value) : next.delete(key),
        );
        return next;
      },
      { replace },
    );
  };
  const clear = () =>
    change({
      range: "all",
      start: null,
      end: null,
      query: null,
      status: null,
      key_id: null,
      model: null,
    });
  const hasFilters =
    range !== "all" ||
    status !== "all" ||
    !!query.trim() ||
    !!params.get("key_id") ||
    !!params.get("model");
  const rangeLabel = {
    today: c(...translatedMessage("today_f692c6d48f")),
    "7d": c(...translatedMessage("last_7_days_1365f5fbb3")),
    "30d": c(...translatedMessage("last_30_days_24648573ce")),
    custom: c(...translatedMessage("custom_dates_15b2c69cd1")),
    all: c(...translatedMessage("last_year_dfdae1e760")),
  };
  const errorText =
    result.error === "missing_dates"
      ? c(...translatedMessage("choose_both_a_start_and_end_date_4a91577c48"))
      : result.error === "invalid_date"
        ? c(...translatedMessage("invalid_date_choose_a_valid_date_bf8c46e512"))
        : c(
            ...translatedMessage(
              "the_end_date_cannot_precede_the_start_date_b6d2f65af1",
            ),
          );
  return (
    <section className="request-history">
      <header className="request-history-header">
        <div>
          <h1>{t("pUsage")}</h1>
          <p>{t("requestTableNote")}</p>
        </div>
        <div className="request-export-action">
          <Button
            size="small"
            variant="secondary"
            onClick={() => {
              const bounds = requestTimeBounds({ range, start, end, timezone });
              onExport({
                start_date: Number.isFinite(bounds.from)
                  ? dubaiDate(bounds.from, timezone)
                  : "",
                end_date: Number.isFinite(bounds.until)
                  ? dubaiDate(bounds.until - 1, timezone)
                  : dubaiDate(Date.now(), timezone),
                status,
                query,
                trace_id: query,
                key_id: params.get("key_id")
                  ? Number(params.get("key_id"))
                  : undefined,
                model: params.get("model") ?? undefined,
              });
            }}
          >
            <Download size={14} />
            {c(...translatedMessage("create_export_task_e769ec073c"))}
          </Button>
          <small>
            {c(
              ...translatedMessage(
                "uses_the_current_filters_up_to_31_days_04c3534b67",
              ),
            )}
          </small>
        </div>
      </header>
      <div className="request-history-panel">
        <div className="request-history-toolbar">
          <label className="request-history-search">
            <span className="sr-only">
              {c(...translatedMessage("search_requests_3f6f554dd7"))}
            </span>
            <Search size={15} aria-hidden="true" />
            <input
              type="search"
              aria-label={c(...translatedMessage("search_requests_3f6f554dd7"))}
              value={query}
              onChange={(e) => change({ query: e.target.value }, true)}
              placeholder={c(...translatedMessage("exact_trace_id_feff830003"))}
            />
          </label>
          {serverPage &&
            ["key", "model"].map((dim) => (
              <label className="request-history-field" key={dim}>
                <span className="sr-only">
                  {dim === "key"
                    ? c(...translatedMessage("filter_key_6470a1a76d"))
                    : c(...translatedMessage("filter_model_279a4304b3"))}
                </span>
                <select
                  className="select-trigger"
                  aria-label={
                    dim === "key"
                      ? c(...translatedMessage("filter_key_6470a1a76d"))
                      : c(...translatedMessage("filter_model_279a4304b3"))
                  }
                  value={params.get(dim === "key" ? "key_id" : "model") ?? ""}
                  onChange={(e) =>
                    change({
                      [dim === "key" ? "key_id" : "model"]: e.target.value,
                    })
                  }
                >
                  <option value="">
                    {dim === "key"
                      ? c(...translatedMessage("all_keys_092be94fb9"))
                      : c(...translatedMessage("all_models_aa0962e85e"))}
                  </option>
                  {(options?.[dim] ?? []).map((v) => (
                    <option key={v.id} value={v.id}>
                      {v.name || v.id}
                    </option>
                  ))}
                </select>
              </label>
            ))}
          <label className="request-history-field">
            <CalendarDays size={15} aria-hidden="true" />
            <span className="sr-only">
              {c(...translatedMessage("request_date_range_171c1a1185"))}
            </span>
            <select
              className="select-trigger"
              aria-label={c(
                ...translatedMessage("request_date_range_171c1a1185"),
              )}
              value={range}
              onChange={(e) =>
                change({
                  range: e.target.value,
                  ...(e.target.value === "custom"
                    ? {
                        start: start || dubaiDate(Date.now(), timezone),
                        end: end || dubaiDate(Date.now(), timezone),
                      }
                    : {}),
                })
              }
            >
              {ranges.map((value) => (
                <option key={value} value={value}>
                  {rangeLabel[value]}
                </option>
              ))}
            </select>
          </label>
          <label className="request-history-status">
            <span className="sr-only">
              {c(...translatedMessage("request_status_3ec173bb98"))}
            </span>
            <select
              className="select-trigger"
              aria-label={c(...translatedMessage("request_status_3ec173bb98"))}
              value={status}
              onChange={(e) => change({ status: e.target.value })}
            >
              <option value="all">
                {c(...translatedMessage("all_statuses_df82d81776"))}
              </option>
              <option value="success">
                {c(...translatedMessage("success_7f82e1ddcc"))}
              </option>
              <option value="failed">
                {c(...translatedMessage("failed_5da76a883a"))}
              </option>
              <option value="cancelled">
                {c(...translatedMessage("cancelled_544275e39b"))}
              </option>
            </select>
          </label>
          {hasFilters && (
            <button className="request-clear-button" onClick={clear}>
              <RotateCcw size={13} />
              {c(...translatedMessage("clear_filters_bd434ebfc4"))}
            </button>
          )}
        </div>
        {range === "custom" && (
          <div className="request-custom-dates">
            <label>
              {c(...translatedMessage("start_date_761834f95f"))}
              <input
                type="date"
                aria-label={c(...translatedMessage("start_date_761834f95f"))}
                aria-invalid={!!result.error}
                aria-describedby={
                  result.error ? "request-date-error" : undefined
                }
                value={start}
                onChange={(e) => change({ start: e.target.value || null })}
              />
            </label>
            <span className="request-date-separator" aria-hidden="true">
              —
            </span>
            <label>
              {c(...translatedMessage("end_date_ddebce9446"))}
              <input
                type="date"
                aria-label={c(...translatedMessage("end_date_ddebce9446"))}
                aria-invalid={!!result.error}
                aria-describedby={
                  result.error ? "request-date-error" : undefined
                }
                value={end}
                onChange={(e) => change({ end: e.target.value || null })}
              />
            </label>
            <span>
              {c(
                ...translatedMessage(
                  "includes_both_full_calendar_days_b1aa9ef31f",
                ),
              )}
            </span>
          </div>
        )}
        {result.error && (
          <p
            className="request-date-error"
            id="request-date-error"
            role="alert"
          >
            <CircleAlert size={14} />
            {errorText}
          </p>
        )}
        <div className="request-history-summary">
          <span aria-live="polite">
            {c(...translatedMessage("requests_5805c076af", { p0: count }))}
            {!serverPage && requests.length !== count && (
              <small>
                {c(
                  ...translatedMessage("total_9de357c4ce", {
                    p0: requests.length,
                  }),
                )}
              </small>
            )}
          </span>
          <span>
            {c(
              ...translatedMessage("dates_and_times_use_161b9d770f", {
                p0: timezone,
              }),
            )}
          </span>
        </div>
        {visibleRows.length > 0 && (
          <div className="request-history-table-wrap">
            <ResponsiveTable className="request-history-table">
              <thead>
                <tr>
                  <th aria-sort="descending">
                    {c(
                      ...translatedMessage("time_9416f54517", { p0: timezone }),
                    )}
                    <ArrowDown
                      size={12}
                      aria-hidden="true"
                      style={{ display: "inline", marginInlineStart: 4 }}
                    />
                  </th>
                  <th>{c(...translatedMessage("trace_id_326806835b"))}</th>
                  <th>{c(...translatedMessage("key_type_8fb6f58a20"))}</th>
                  <th>
                    {c(
                      ...translatedMessage("requested_actual_model_05304874f7"),
                    )}
                  </th>
                  <th>{c(...translatedMessage("vendor_5dc28b7ea6"))}</th>
                  <th>{c(...translatedMessage("group_528d77dde5"))}</th>
                  <th>{c(...translatedMessage("status_dda39dcd63"))}</th>
                  <th>{c(...translatedMessage("switch_error_5c4477d117"))}</th>
                  <th>
                    {c(
                      ...translatedMessage(
                        "input_output_cache_tokens_38a3a3cdab",
                      ),
                    )}
                  </th>
                  <th>{c(...translatedMessage("first_token_4d9199c032"))}</th>
                  <th className="request-number">
                    {c(...translatedMessage("duration_aab6e26676"))}
                  </th>
                  <th className="request-number">
                    {c(...translatedMessage("cost_usd_9cd3ac1402"))}
                  </th>
                </tr>
              </thead>
              <tbody>
                {visibleRows.map((row) => (
                  <tr key={row.trace_id} onClick={() => onSelect(row)}>
                    <td className="mono">
                      {new Date(row.time).toLocaleString(
                        regionalLocale(i18n.language),
                        {
                          timeZone: timezone,
                          hour12: false,
                        },
                      )}
                    </td>
                    <td className="mono">
                      <button
                        className="request-trace-button"
                        onClick={(e) => {
                          e.stopPropagation();
                          onSelect(row);
                        }}
                      >
                        {row.trace_id}
                      </button>
                    </td>
                    <td>
                      {row.key_deleted
                        ? c(...translatedMessage("deleted_key_571647d876"))
                        : row.key_name || row.key_id || "—"}
                      <small>
                        {callTypeLabel(row.call_type, i18n.language)}
                      </small>
                    </td>
                    <td>
                      <strong dir="ltr">
                        {row.requested_model || row.model}
                      </strong>
                      <small dir="ltr">
                        →{" "}
                        {row.actual_model ||
                          (row.status === "success" ? row.model : "—")}
                      </small>
                    </td>
                    <td>{row.vendor}</td>
                    <td>
                      {row.service_group ||
                        (row.tier ? <TierBadge id={row.tier} /> : "—")}
                    </td>
                    <td>
                      <span
                        className={`result-pill ${row.status === "failed" ? "is-failed" : ""}`}
                      >
                        {row.status === "failed" ? (
                          <CircleAlert size={13} />
                        ) : (
                          <Check size={13} />
                        )}
                        {row.outcome === "cancelled"
                          ? c(...translatedMessage("cancelled_544275e39b"))
                          : t(
                              row.status === "failed"
                                ? "pFailed"
                                : "pSucceeded",
                            )}
                      </span>
                    </td>
                    <td>
                      {row.fallback
                        ? c(
                            ...translatedMessage(
                              "automatically_switched_cd94edcbb0",
                            ),
                          )
                        : "—"}
                      <small>{row.error_code || "—"}</small>
                    </td>
                    <td className="mono">
                      {row.input_tokens} / {row.output_tokens}
                      <small>
                        {row.cache_read_tokens ?? 0} /{" "}
                        {row.cache_write_tokens ?? 0}
                      </small>
                      {row.billing_detail?.mode !== "tokens" &&
                        row.call_type !== "chat" && (
                          <small>
                            {row.billing_detail?.parts
                              .map((p) => `${p.quantity} ${p.unit}`)
                              .join(" · ")}
                          </small>
                        )}
                    </td>
                    <td className="mono">
                      {row.ttft_ms ? `${row.ttft_ms} ms` : "—"}
                    </td>
                    <td className="mono request-number">
                      {row.duration_ms}{" "}
                      {c(...translatedMessage("ms_7184894997"))}
                    </td>
                    <td className="mono request-number request-cost">
                      {formatMoney(row.cost_nano, "call")}
                    </td>
                  </tr>
                ))}
              </tbody>
            </ResponsiveTable>
          </div>
        )}
        {(serverPage?.loading || serverPage?.error) && (
          <p role={serverPage.error ? "alert" : "status"}>
            {serverPage.error ||
              c(...translatedMessage("loading_requests_bbb4221e07"))}
          </p>
        )}
        {!visibleRows.length && !serverPage?.loading && !serverPage?.error && (
          <div className="request-empty" role="status">
            {result.error ? (
              <>
                <strong>
                  {c(...translatedMessage("check_the_date_range_52d6975253"))}
                </strong>
                <p>{errorText}</p>
              </>
            ) : !requests.length ? (
              <>
                <strong>
                  {c(...translatedMessage("no_requests_yet_7e097cd000"))}
                </strong>
                <p>
                  {c(
                    ...translatedMessage(
                      "connect_a_tool_to_review_costs_and_errors_her_1eebf97789",
                    ),
                  )}
                </p>
                <Link to="/console/connect">
                  {c(...translatedMessage("connect_a_tool_ef1a3d8013"))}
                </Link>
              </>
            ) : (
              <>
                <strong>
                  {c(...translatedMessage("no_matching_requests_71efebcf83"))}
                </strong>
                <p>
                  {c(
                    ...translatedMessage(
                      "adjust_the_date_status_or_search_your_records_6da21b58e8",
                    ),
                  )}
                </p>
                <Button size="small" variant="secondary" onClick={clear}>
                  {c(
                    ...translatedMessage(
                      "clear_filters_and_show_all_4d14a05fa8",
                    ),
                  )}
                </Button>
              </>
            )}
          </div>
        )}
        <footer className="request-history-pagination">
          <span>
            {count
              ? c(
                  ...translatedMessage("of_d11efb7ec9", {
                    p0: (page - 1) * PAGE_SIZE + 1,
                    p1: Math.min(page * PAGE_SIZE, count),
                    p2: count,
                  }),
                )
              : c(...translatedMessage("0_records_ddc08efba4"))}
          </span>
          <div>
            <Button
              size="small"
              variant="secondary"
              disabled={page <= 1}
              aria-label={c(...translatedMessage("previous_page_69cac17b46"))}
              onClick={() => change({ page: String(page - 1) })}
            >
              <ChevronLeft size={14} />
            </Button>
            <span>
              {page} / {pages}
            </span>
            <Button
              size="small"
              variant="secondary"
              disabled={page >= pages}
              aria-label={c(...translatedMessage("next_page_8a29f0d0d2"))}
              onClick={() => change({ page: String(page + 1) })}
            >
              <ChevronRight size={14} />
            </Button>
          </div>
        </footer>
      </div>
    </section>
  );
}
