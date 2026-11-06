import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "../components/ui/button";
import { useFinanceSummary } from "../lib/api/finance";
import { formatFinanceAmount } from "../lib/finance-format";
import { canReadFinance, useSessionQueryIdentity } from "../lib/system-access";
import "./finance-overview.css";

export function FinanceOverview() {
  const { i18n } = useTranslation();
  const text = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [params, setParams] = useSearchParams();
  const candidate = Number(params.get("days") || 30);
  const days = [7, 30, 90].includes(candidate) ? candidate : 30;
  const query = useFinanceSummary(days);
  const scope = useSessionQueryIdentity();
  if (scope[1] === null || !canReadFinance(scope[3]))
    return (
      <p role="alert">
        {text(
          ...translatedMessage("this_account_has_no_finance_access_912e309887"),
        )}
      </p>
    );
  const data = query.error ? undefined : query.data;
  const date = (timestamp: number) =>
    new Intl.DateTimeFormat(regionalLocale(i18n.language), {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: "UTC",
    }).format(new Date(timestamp * 1000));
  return (
    <section className="finance-overview">
      <header className="finance-heading">
        <div>
          <p className="finance-eyebrow">
            {text(...translatedMessage("read_only_finance_16b84934dc"))}
          </p>
          <h1>{text(...translatedMessage("finance_overview_f47c7aaf02"))}</h1>
          <p>
            {text(
              ...translatedMessage(
                "review_recorded_payments_wallet_balances_and__5e9f55e9cb",
              ),
            )}
          </p>
        </div>
        <div className="finance-tools">
          <label>
            {text(...translatedMessage("reporting_window_e03f76b62e"))}
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
                  {text(
                    ...translatedMessage("last_days_66813b38c3", { p0: n }),
                  )}
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
            {text(...translatedMessage("refresh_9c83c3600a"))}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_financial_records_6d3370bc8e"))}
        </p>
      )}
      {query.error && (
        <div className="finance-error" role="alert">
          {text(
            ...translatedMessage(
              "financial_records_are_unavailable_confirm_you_601d589421",
            ),
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
                ...translatedMessage(
                  "cash_wallets_and_api_quota_use_separate_accou_63e6de77de",
                ),
              )}
            </p>
          )}
          <div className="finance-grid">
            {[
              {
                key: "wallet",
                title: text(
                  ...translatedMessage(
                    "cash_wallet_balance_snapshot_ca613247af",
                  ),
                ),
                amount: data.wallet_balance_nano,
                note: text(
                  ...translatedMessage(
                    "recorded_balances_across_all_wallets_fef8f2aaf2",
                  ),
                ),
              },
              {
                key: "recharge",
                title: text(
                  ...translatedMessage("recorded_paid_top_ups_65464b1736"),
                ),
                amount: data.paid_recharge_nano,
                note: text(
                  ...translatedMessage("orders_by_paid_time_0e70498fd3", {
                    p0: data.paid_recharge_count,
                  }),
                ),
              },
              {
                key: "collected",
                title: text(
                  ...translatedMessage(
                    "api_quota_charged_usd_equivalent_56a31a498b",
                  ),
                ),
                amount: data.api_collected_nano,
                note: text(
                  ...translatedMessage(
                    "includes_charged_failed_and_cancelled_request_61b436a055",
                  ),
                ),
              },
              {
                key: "unfunded",
                title: text(
                  ...translatedMessage(
                    "unfunded_quota_usd_equivalent_eab32d97ce",
                  ),
                ),
                amount: data.api_unfunded_nano,
                note: text(
                  ...translatedMessage(
                    "priced_usage_that_could_not_be_collected_3a1aab1540",
                  ),
                ),
              },
              {
                key: "credits",
                title: text(
                  ...translatedMessage("wallet_ledger_credits_ca0e754660"),
                ),
                amount: data.ledger_credits_nano,
                note: text(
                  ...translatedMessage(
                    "positive_ledger_entries_in_this_window_ac86fc4f46",
                  ),
                ),
              },
              {
                key: "debits",
                title: text(
                  ...translatedMessage("wallet_ledger_debits_65fb4758ac"),
                ),
                amount: data.ledger_debits_nano,
                note: text(
                  ...translatedMessage(
                    "absolute_value_of_negative_ledger_entries_94e09ba135",
                  ),
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
                      ...translatedMessage("exact_amount_nano_usd_2b8f2146f6"),
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
              {text(...translatedMessage("api_request_outcomes_b848487fce"))}
            </h2>
            <dl>
              {[
                [
                  text(...translatedMessage("total_requests_e14692abbf")),
                  data.api_request_count,
                ],
                [
                  text(...translatedMessage("successful_63278519e1")),
                  data.api_success_count,
                ],
                [
                  text(...translatedMessage("failed_9a90056a7b")),
                  data.api_failed_count,
                ],
                [
                  text(...translatedMessage("cancelled_ac14ca071f")),
                  data.api_cancelled_count,
                ],
                [
                  text(...translatedMessage("unknown_status_1eec72e763")),
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
                ...translatedMessage(
                  "recorded_paid_orders_have_no_payment_timestam_4bd6b0d8a0",
                  { p0: data.paid_orders_without_timestamp },
                ),
              )}
            </p>
          )}
        </>
      )}
    </section>
  );
}
