import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";

import { useCallback, useEffect, useMemo, useState } from "react";
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
import { useSessionQueryIdentity } from "../lib/system-access";
import { fundingSourceLabel } from "../lib/funding-source";
import { writtenLanguage } from "../lib/localize";
import type { LedgerEntry, RechargeOrder } from "../lib/product-preview";
import {
  useLedger,
  useRechargeOrders,
  useWallet,
  adaptRechargeOrder,
  adaptLedgerEntry,
} from "../lib/api/billing";
import { useCreateExport, downloadExport } from "../lib/api/exports";
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
    pending: translatedMessage("awaiting_payment_4166815c6e"),
    processing: translatedMessage("confirming_payment_7ae473563c"),
    paid: translatedMessage("credited_c144ccc983"),
    expired: translatedMessage("expired_3a836b996f"),
    failed: translatedMessage("payment_failed_54d80406b3"),
    cancelled: translatedMessage("cancelled_544275e39b"),
    disputed: translatedMessage("disputed_8003689954"),
  };
  return labels[status] ? c(...labels[status]) : status;
}
function entryType(type: string, c: CopyFn) {
  const words: Record<string, [string, string, string]> = {
    recharge: translatedMessage("top_up_009650f075"),
    call_summary: translatedMessage("model_usage_f384a76fff"),
    adjustment: translatedMessage("adjustment_dbc206baf1"),
    refund: translatedMessage("refund_70c2f58395"),
  };
  return words[type] ? c(...words[type]) : type;
}
function Status({ order, c }: { order: RechargeOrder; c: CopyFn }) {
  return (
    <span className={`bc-status is-${order.status}`}>
      <i />
      {order.status === "paid" && order.late_payment
        ? c(...translatedMessage("credited_confirmed_late_8fa2dba581"))
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
export default function BillingCenter(props: {
  onRecharge: (orderId?: string) => void;
}) {
  const identity = useSessionQueryIdentity();
  return <BillingCenterView key={JSON.stringify(identity)} {...props} />;
}

function BillingCenterView({
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
  // The wallet is authoritative; ledger rows may include other users for admins.
  const ledgerQuery = useLedger();
  const ordersQuery = useRechargeOrders();
  const walletQuery = useWallet();
  const exportMutation = useCreateExport();
  const ledger = useMemo(
    () =>
      (ledgerQuery.error ? [] : (ledgerQuery.data ?? [])).map(
        adaptLedgerEntry,
      ) as unknown as LedgerEntry[],
    [ledgerQuery.data, ledgerQuery.error],
  );
  const orders = useMemo(
    () =>
      (ordersQuery.error ? [] : (ordersQuery.data ?? [])).map(
        adaptRechargeOrder,
      ) as unknown as RechargeOrder[],
    [ordersQuery.data, ordersQuery.error],
  );
  const balanceKnown = Boolean(walletQuery.data && !walletQuery.error);
  const balance = balanceKnown
    ? (walletQuery.data!.api_balance_nano ?? walletQuery.data!.balance_nano)
    : "0";
  const ledgerKnown = Boolean(ledgerQuery.data && !ledgerQuery.error);
  const recordsKnown =
    tab === "orders"
      ? Boolean(ordersQuery.data && !ordersQuery.error)
      : ledgerKnown;
  const expireOrders = useCallback(() => {
    // Trigger a refetch so the backend lazily expires stale orders
    // and the list updates with the latest state.
    return Promise.all([
      ordersQuery.refetch(),
      ledgerQuery.refetch(),
      walletQuery.refetch(),
    ]);
  }, [ordersQuery.refetch, ledgerQuery.refetch, walletQuery.refetch]);
  const timezone = walletQuery.data?.timezone ?? "Asia/Dubai";
  const today = billingDate(new Date().toISOString(), timezone);
  const month = today.slice(0, 7);
  const [from, setFrom] = useState(`${month}-01`);
  const [to, setTo] = useState(today);
  useEffect(() => {
    setFrom(`${month}-01`);
    setTo(today);
  }, [timezone]);
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState("all");
  const [page, setPage] = useState(1);
  const [guide, setGuide] = useState<"help" | "transfer" | null>(null);
  const [entry, setEntry] = useState<LedgerEntry | null>(null);
  const [message, setMessage] = useState("");
  useEffect(() => {
    void expireOrders();
    const timer = setInterval(() => void expireOrders(), 15000);
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
    const created = billingDate(selected.created_at, timezone);
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
      setFrom(billingDate(matched.time, timezone));
      setTo(billingDate(matched.time, timezone));
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
            timezone,
            query,
          })
        : [],
    [orders, from, to, kind, query, validRange, timezone],
  );
  const filteredLedger = useMemo(
    () =>
      validRange
        ? filterLedger(ledger, {
            from,
            to,
            type: kind === "all" ? undefined : kind,
            timezone,
            query,
          })
        : [],
    [ledger, from, to, kind, query, validRange, timezone],
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
      (e) =>
        e.type === "call_summary" &&
        billingDate(e.time, timezone).startsWith(month),
    )
    .reduce(
      (sum, e) =>
        sum + (BigInt(e.amount_nano) < 0n ? -BigInt(e.amount_nano) : 0n),
      0n,
    );
  const funded = ledger
    .filter(
      (e) =>
        e.type === "recharge" &&
        billingDate(e.time, timezone).startsWith(month),
    )
    .reduce((sum, e) => sum + BigInt(e.amount_nano), 0n);
  const isLow = balanceKnown && BigInt(balance) < 5_000_000_000n;
  const isEmpty = balanceKnown && BigInt(balance) <= 0n;
  const date = (value: string) =>
    new Intl.DateTimeFormat(regionalLocale(language), {
      timeZone: timezone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(new Date(value));
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
  const exportRecords = async () => {
    if (!validRange || !count) return;
    if (tab === "ledger") {
      try {
        const record = await exportMutation.mutateAsync({
          type: "ledger",
          start_date: from,
          end_date: to,
          ledger_type: kind,
          query,
          language: writtenLanguage(language),
        });
        if (record.status === "completed")
          await downloadExport(record.id, record.filename, record.download_url);
        setMessage(
          c(
            ...translatedMessage(
              "export_created_files_and_field_descriptions_a_8a2ad986b6",
            ),
          ),
        );
      } catch (err) {
        setMessage(err instanceof Error ? err.message : String(err));
      }
      return;
    }
    downloadCsv(
      tab === "orders"
        ? serializeRechargeCsv(
            filteredOrders,
            writtenLanguage(language),
            timezone,
          )
        : serializeLedgerCsv(filteredLedger, writtenLanguage(language)),
      `veyra-${tab}-${from}-${to}.csv`,
    );
    setMessage(
      c(
        ...translatedMessage("exported_matching_records_703f168361", {
          p0: count,
        }),
      ),
    );
  };
  const description = (item: LedgerEntry) =>
    item.funding_source === "api_quota"
      ? c(
          ...translatedMessage("api_usage_key_requests_97f12ce347", {
            p0: item.token_id ?? "",
            p1: item.request_count ?? 0,
          }),
        )
      : item.type === "recharge"
        ? c(...translatedMessage("online_top_up_credited_6ebc06d35b"))
        : item.type === "call_summary"
          ? item.trace_id
            ? c(...translatedMessage("model_request_charge_2fea2623dd"))
            : c(...translatedMessage("model_usage_settlement_95e9db14cf"))
          : item.description === "注销清零"
            ? c(...translatedMessage("account_closure_settlement_c2b71cfe83"))
            : item.description === "Enterprise Onboarding Credit Incentive"
              ? c(...translatedMessage("initial_demo_credit_a8f44c82eb"))
              : item.description === "Demo scenario balance adjustment"
                ? c(...translatedMessage("demo_balance_adjustment_d8ab0473d6"))
                : item.description;

  return (
    <div className="bc-page">
      <header className="bc-heading">
        <div>
          <div className="bc-eyebrow">
            <Wallet size={13} />
            {c(...translatedMessage("account_billing_2dc8658792"))}
          </div>
          <h1>{c(...translatedMessage("balance_top_ups_3762ef545e"))}</h1>
          <p>
            {c(
              ...translatedMessage(
                "keep_your_next_idea_powered_and_ready_300b4871b7",
              ),
            )}
          </p>
        </div>
        <Button className="bc-help-button" onClick={() => setGuide("help")}>
          <CircleHelp size={15} />
          {c(...translatedMessage("how_top_ups_work_3fd4817914"))}
        </Button>
      </header>
      {(walletQuery.error || ledgerQuery.error || ordersQuery.error) && (
        <p
          role="alert"
          data-wallet-error={Boolean(walletQuery.error)}
          data-ledger-error={Boolean(ledgerQuery.error)}
        >
          {c(
            ...translatedMessage(
              "some_billing_data_is_unavailable_refresh_to_r_1cc358029d",
            ),
          )}
        </p>
      )}
      <div className="bc-summary-grid">
        <section className="bc-wallet-card">
          <div className="bc-wallet-top">
            <span>
              {c(...translatedMessage("available_api_balance_76c092280b"))}
            </span>
            <span className={`bc-wallet-health ${isLow ? "is-low" : ""}`}>
              <i />
              {!balanceKnown
                ? c(...translatedMessage("balance_unknown_e5b16838b6"))
                : isEmpty
                  ? c(...translatedMessage("balance_depleted_73bdc5960b"))
                  : isLow
                    ? c(...translatedMessage("low_balance_55e25339db"))
                    : c(...translatedMessage("wallet_funded_8c2c898f6a"))}
            </span>
          </div>
          <div className="bc-balance">
            <strong
              dir="ltr"
              tabIndex={balanceKnown ? 0 : undefined}
              className={
                balanceKnown && formatMoney(balance).length > 14
                  ? "is-wide"
                  : undefined
              }
            >
              {balanceKnown ? formatMoney(balance) : "—"}
            </strong>
            <span>{c(...translatedMessage("usd_ed29d2d0f6"))}</span>
          </div>
          <p>
            {c(
              ...translatedMessage(
                "payment_wallet_and_api_quota_are_separate_sim_89279fac4d",
              ),
            )}
          </p>
          {walletQuery.data?.api_balance_nano !== undefined && (
            <small>
              {c(...translatedMessage("payment_simulation_wallet_f427feb46e"))}:{" "}
              <bdi>{formatMoney(walletQuery.data.balance_nano)}</bdi>
            </small>
          )}
          <div className="bc-wallet-actions">
            <Button variant="primary" onClick={() => onRecharge()}>
              <CreditCard size={16} />
              {c(...translatedMessage("add_funds_8dc21b313b"))}
              <ArrowRight size={14} />
            </Button>
            <Link to="/console/usage">
              {c(...translatedMessage("view_usage_93bffc290a"))}
              <ArrowUpRight size={14} />
            </Link>
          </div>
          <div className="bc-wallet-footer">
            <ShieldCheck size={14} />
            {c(
              ...translatedMessage(
                "simulated_payments_do_not_charge_real_funds_a_3e2458ca28",
              ),
            )}
          </div>
        </section>

        <section className="bc-month-card">
          <div className="bc-small-heading">
            <span>{c(...translatedMessage("this_month_1b6c48aebc"))}</span>
            <bdi>{month}</bdi>
          </div>
          <div className="bc-month-metric">
            <span className="bc-metric-icon">
              <ArrowUpRight size={17} />
            </span>
            <div>
              <span>
                {c(...translatedMessage("usage_this_month_da9da6acf0"))}
              </span>
              <strong dir="ltr">
                {walletQuery.data?.month_spend_nano !== undefined
                  ? formatMoney(walletQuery.data.month_spend_nano, "total")
                  : ledgerKnown
                    ? formatMoney(spend.toString(), "total")
                    : "—"}
              </strong>
            </div>
          </div>
          <div className="bc-month-metric">
            <span className="bc-metric-icon is-green">
              <ArrowDownLeft size={17} />
            </span>
            <div>
              <span>
                {c(...translatedMessage("top_ups_credited_3dc4153f2a"))}
              </span>
              <strong dir="ltr">
                {ledgerKnown ? formatMoney(funded.toString()) : "—"}
              </strong>
            </div>
          </div>
          <p>
            {c(...translatedMessage("calendar_month_in_dubai_time_b5ee9fc18a"))}
          </p>
        </section>
        <section className="bc-protection-card">
          <span className="bc-protection-icon">
            <BellRing size={20} />
          </span>
          <h2>{c(...translatedMessage("balance_protection_fcaefdce7b"))}</h2>
          <p>
            {c(
              ...translatedMessage(
                "the_console_alerts_you_when_your_balance_drop_fb6bde686b",
              ),
            )}
          </p>
          <div>
            <span>{c(...translatedMessage("alert_threshold_a4a91891cb"))}</span>
            <strong dir="ltr">$5.00</strong>
          </div>
          <small>
            {c(
              ...translatedMessage(
                "alerts_use_the_available_api_balance_908d362826",
              ),
            )}
          </small>
        </section>
      </div>
      <div className="bc-demo-note">
        <Info size={14} />
        <span>
          {c(
            ...translatedMessage(
              "balance_and_recharge_are_backed_by_the_server_4b55b4e56d",
            ),
          )}
        </span>
      </div>
      {!!pending.length && (
        <div className="bc-pending-banner">
          <Clock3 size={19} />
          <div>
            <strong>
              {c(
                ...translatedMessage("top_ups_still_in_progress_999adc57d8", {
                  p0: pending.length,
                }),
              )}
            </strong>
            <span>
              {c(
                ...translatedMessage(
                  "closing_the_payment_window_does_not_confirm_p_40ee9cabfd",
                ),
              )}
            </span>
          </div>
          <Button size="small" onClick={() => onRecharge(pending[0].id)}>
            {c(...translatedMessage("view_order_dd72f0dcb4"))}
            <ArrowRight size={13} />
          </Button>
        </div>
      )}
      <section className="bc-history-card">
        <div className="bc-history-heading">
          <nav
            aria-label={c(...translatedMessage("billing_records_9ad2848531"))}
          >
            <button
              aria-pressed={tab === "orders"}
              onClick={() => changeTab("orders")}
            >
              <ReceiptText size={16} />
              {c(...translatedMessage("top_up_history_2c057b93c4"))}
              <span>{orders.length}</span>
            </button>
            <button
              aria-pressed={tab === "ledger"}
              onClick={() => changeTab("ledger")}
            >
              <FileText size={16} />
              {c(...translatedMessage("transactions_df2cc45b7b"))}
            </button>
          </nav>
          <div className="bc-history-actions">
            <button
              className="bc-icon-button"
              disabled={
                walletQuery.isFetching ||
                ledgerQuery.isFetching ||
                ordersQuery.isFetching
              }
              onClick={async () => {
                const results = await expireOrders();
                setMessage(
                  results.every((result) => !result.isError)
                    ? c(
                        ...translatedMessage(
                          "wallet_orders_and_transactions_refreshed_045f065b7e",
                        ),
                      )
                    : c(
                        ...translatedMessage(
                          "some_billing_data_could_not_be_refreshed_retr_817ab8c769",
                        ),
                      ),
                );
              }}
              aria-label={c(
                ...translatedMessage("refresh_billing_data_253c943648"),
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
              {c(...translatedMessage("export_83098582f0"))}
            </Button>
          </div>
        </div>
        <div className="bc-filters">
          <label className="bc-search">
            <span>{c(...translatedMessage("search_e31c158f20"))}</span>
            <div>
              <Search size={14} />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={
                  tab === "orders"
                    ? c(...translatedMessage("search_order_id_ff8b3e9404"))
                    : c(
                        ...translatedMessage(
                          "search_transaction_or_order_ec9d4bec81",
                        ),
                      )
                }
                aria-label={c(
                  ...translatedMessage("search_billing_records_38b5bebd02"),
                )}
              />
              {query && (
                <button
                  onClick={() => setQuery("")}
                  aria-label={c(
                    ...translatedMessage("clear_search_99d00f5436"),
                  )}
                >
                  <X size={13} />
                </button>
              )}
            </div>
          </label>
          <label>
            <span>
              {tab === "orders"
                ? c(...translatedMessage("order_status_a9f703c47a"))
                : c(...translatedMessage("transaction_type_4c1254794c"))}
            </span>
            <select
              value={kind}
              onChange={(e) => setKind(e.target.value)}
              aria-label={c(
                ...translatedMessage("record_type_or_status_bf01df8196"),
              )}
            >
              <option value="all">
                {c(...translatedMessage("all_ac49939df0"))}
              </option>
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
            <span>{c(...translatedMessage("from_7a16d6bf6a"))}</span>
            <input
              type="date"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              aria-label={c(...translatedMessage("from_date_13a58e62e4"))}
            />
          </label>
          <label>
            <span>{c(...translatedMessage("to_493a80b247"))}</span>
            <input
              type="date"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              aria-label={c(...translatedMessage("to_date_771f3e3bb3"))}
            />
          </label>
          <button className="bc-reset" onClick={resetFilters}>
            {c(...translatedMessage("reset_e2dc2cf181"))}
          </button>
        </div>
        <div className="bc-filter-note">
          <span>
            {c(
              ...translatedMessage("up_to_31_days_per_query_dc62f3294b", {
                p0: timezone,
              }),
            )}
          </span>
          <b>
            {recordsKnown
              ? c(...translatedMessage("records_2b9560427a", { p0: count }))
              : c(...translatedMessage("records_unconfirmed_6a813c423e"))}
          </b>
        </div>
        {message && (
          <p className="bc-feedback" role="status">
            <Check size={14} />
            {message}
            {exportMutation.data && (
              <Link to="/console/exports">
                {c(...translatedMessage("exports_b2df1f0c09"))}
              </Link>
            )}
          </p>
        )}
        {!validRange ? (
          <div className="bc-empty" role="alert">
            <Clock3 size={30} />
            <h3>
              {c(...translatedMessage("adjust_the_date_range_543d35f759"))}
            </h3>
            <p>
              {c(
                ...translatedMessage(
                  "the_end_date_must_follow_the_start_date_with__c4052d5ec4",
                ),
              )}
            </p>
            <Button onClick={resetFilters}>
              {c(...translatedMessage("return_to_this_month_06201b7fa4"))}
            </Button>
          </div>
        ) : !count ? (
          <div className="bc-empty">
            <ReceiptText size={34} />
            <h3>
              {recordsKnown
                ? c(...translatedMessage("no_matching_records_a5e6395ad2"))
                : c(
                    ...translatedMessage(
                      "billing_records_are_unconfirmed_f5f2ba5a14",
                    ),
                  )}
            </h3>
            <p>
              {recordsKnown
                ? c(
                    ...translatedMessage(
                      "try_another_filter_or_complete_your_first_top_ca3d627810",
                    ),
                  )
                : c(
                    ...translatedMessage(
                      "data_is_loading_or_unavailable_refresh_to_che_3df441ffd4",
                    ),
                  )}
            </p>
            <div>
              <Button onClick={resetFilters}>
                {c(...translatedMessage("reset_filters_a7e85112c6"))}
              </Button>
              <Button variant="primary" onClick={() => onRecharge()}>
                {c(...translatedMessage("add_funds_cfb6bb4a87"))}
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
                          c(...translatedMessage("order_created_80c64b5680")),
                          c(...translatedMessage("payment_method_3283479ba3")),
                          c(...translatedMessage("amount_6139a36f94")),
                          c(...translatedMessage("status_9c1a8aa41d")),
                          c(...translatedMessage("credited_at_2b093e265f")),
                          c(...translatedMessage("action_2a8e78b55a")),
                        ]
                      : [
                          c(
                            ...translatedMessage("time_transaction_9a2a5ed91a"),
                          ),
                          c(...translatedMessage("type_b91a24e5ad")),
                          c(
                            ...translatedMessage(
                              "description_order_4eb2a99819",
                            ),
                          ),
                          c(...translatedMessage("amount_5a46f9925f")),
                          c(...translatedMessage("balance_after_56f6e55ff1")),
                          c(...translatedMessage("action_2a8e78b55a")),
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
                              {c(...translatedMessage("card_27832da56e"))}
                            </span>
                            <small className="bc-funding-source">
                              {fundingSourceLabel(order.funding_source, c)}
                            </small>
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
                                ? c(...translatedMessage("continue_760d61dca0"))
                                : c(...translatedMessage("details_d35ec1ed25"))}
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
                            <small className="bc-funding-source">
                              {fundingSourceLabel(item.funding_source, c)}
                            </small>
                          </td>
                          <td>
                            <span>{description(item)}</span>
                            {item.order_id && (
                              <button
                                className="bc-associated"
                                onClick={() =>
                                  onRecharge(
                                    orders.find(
                                      (order) =>
                                        order.id === item.order_id ||
                                        order.provider_order_id ===
                                          item.order_id,
                                    )?.id ?? item.order_id,
                                  )
                                }
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
                              {c(...translatedMessage("details_a91e8f9bea"))}
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
                  ...translatedMessage("of_f84b3309ed", {
                    p0: start + 1,
                    p1: Math.min(start + 6, count),
                    p2: count,
                  }),
                )}
              </span>
              <div>
                <button
                  aria-label={c(
                    ...translatedMessage("previous_page_69cac17b46"),
                  )}
                  disabled={currentPage === 1}
                  onClick={() => setPage(currentPage - 1)}
                >
                  <ChevronLeft size={15} />
                </button>
                <span>
                  {currentPage} / {pages}
                </span>
                <button
                  aria-label={c(...translatedMessage("next_page_8a29f0d0d2"))}
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
                ...translatedMessage(
                  "large_top_ups_offline_payments_cbe5005247",
                ),
              )}
            </h3>
            <p>
              {c(
                ...translatedMessage(
                  "learn_about_verification_crediting_and_refund_44083a2326",
                ),
              )}
            </p>
          </div>
          <ArrowUpRight size={16} />
        </button>
        <div className="bc-receipt-note">
          <ReceiptText size={19} />
          <p>
            {c(
              ...translatedMessage(
                "payment_receipts_come_from_the_payment_provid_dea7263571",
              ),
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
            ? c(...translatedMessage("payments_refunds_e7182d948a"))
            : c(...translatedMessage("top_ups_crediting_ff976683cf"))
        }
        description={c(
          ...translatedMessage(
            "based_on_the_platform_s_usd_prepaid_policy_c67a98704d",
          ),
        )}
      >
        <div className="bc-guide">
          <div className="bc-guide-lead">
            <ShieldCheck size={22} />
            <p>
              {c(
                ...translatedMessage(
                  "payment_wallet_balance_and_api_quota_are_reco_deaea7346e",
                ),
              )}
            </p>
          </div>
          {(guide === "transfer"
            ? [
                [
                  c(...translatedMessage("offline_payments_f808a64147")),
                  c(
                    ...translatedMessage(
                      "confirm_the_receiving_entity_banking_details__3e253f80ef",
                    ),
                  ),
                ],
                [
                  c(
                    ...translatedMessage(
                      "keep_verification_records_7755af29ca",
                    ),
                  ),
                  c(
                    ...translatedMessage(
                      "keep_the_transfer_receipt_payer_amount_and_da_b72da61980",
                    ),
                  ),
                ],
                [
                  c(
                    ...translatedMessage(
                      "refunds_are_handled_manually_c0003d35a5",
                    ),
                  ),
                  c(
                    ...translatedMessage(
                      "contact_support_to_review_the_payment_order_a_375a7150b4",
                    ),
                  ),
                ],
              ]
            : [
                [
                  c(...translatedMessage("amounts_fees_0ea3385b4f")),
                  c(
                    ...translatedMessage(
                      "10_10_000_per_top_up_with_up_to_two_decimal_p_4dece3e3ba",
                    ),
                  ),
                ],
                [
                  c(...translatedMessage("when_funds_are_credited_de82d611c6")),
                  c(
                    ...translatedMessage(
                      "after_payment_wait_for_provider_confirmation__fd4e9bf577",
                    ),
                  ),
                ],
                [
                  c(...translatedMessage("order_validity_a7586b4ea7")),
                  c(
                    ...translatedMessage(
                      "orders_are_valid_for_30_minutes_unpaid_orders_f36f82594e",
                    ),
                  ),
                ],
                [
                  c(...translatedMessage("receipts_records_f8e409545e")),
                  c(
                    ...translatedMessage(
                      "receipts_are_sent_by_the_payment_provider_the_7adb8b4913",
                    ),
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
        title={c(...translatedMessage("transaction_details_05aee18152"))}
        description={entry ? entryType(entry.type, c) : ""}
      >
        {entry && (
          <div className="bc-entry-detail">
            <div
              className={`bc-entry-value ${BigInt(entry.amount_nano) > 0n ? "is-positive" : ""}`}
            >
              <small>
                {c(...translatedMessage("transaction_amount_cf28f76ab7"))}
              </small>
              <strong dir="ltr" tabIndex={0}>
                {BigInt(entry.amount_nano) > 0n ? "+" : ""}
                {formatMoney(entry.amount_nano, "detail")}
              </strong>
              <span>
                {c(
                  ...translatedMessage(
                    "usd_full_accounting_precision_6b2db5ffcd",
                  ),
                )}
              </span>
            </div>
            <dl>
              {[
                [
                  c(...translatedMessage("transaction_id_e8037d3348")),
                  entry.id,
                ],
                [
                  c(...translatedMessage("time_dubai_330adba455")),
                  `${date(entry.time)} · UTC+4`,
                ],
                [
                  c(...translatedMessage("balance_after_56f6e55ff1")),
                  formatMoney(entry.balance_after_nano, "detail"),
                ],
                [
                  c(...translatedMessage("description_c6bae5d4e7")),
                  description(entry),
                ],
                [
                  c(...translatedMessage("funding_source_355341b128")),
                  fundingSourceLabel(entry.funding_source, c),
                ],
                [
                  c(...translatedMessage("api_quota_conversion_78f6b675e2")),
                  c(...translatedMessage("currently_unavailable_fff1a9e4ff")),
                ],
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
                  {c(...translatedMessage("view_related_request_e89d2a2267"))}
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
                {c(...translatedMessage("view_related_top_up_6917f11f8d"))}
                <ArrowRight size={14} />
              </Button>
            )}
            <Button
              onClick={() =>
                copyText(
                  entry.id,
                  c(...translatedMessage("id_copied_2c36a64426")),
                  c(...translatedMessage("copy_failed_45c59affcc")),
                )
              }
            >
              <Copy size={14} />
              {c(...translatedMessage("copy_transaction_id_272961d02f"))}
            </Button>
            <p>
              {c(
                ...translatedMessage(
                  "this_record_is_for_reconciliation_not_a_payme_d9eefacf65",
                ),
              )}
            </p>
          </div>
        )}
      </Drawer>
    </div>
  );
}
