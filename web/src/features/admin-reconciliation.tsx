import { message as translatedMessage } from "../i18n/messages";

import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Drawer } from "../components/ui/overlay";
import { apiGet, apiPost } from "../lib/api/http";
import { useStaffText } from "../lib/api/staff";
import { useSessionQueryIdentity } from "../lib/system-access";
import { formatMoney } from "../lib/money";
import {
  AdminFeedback,
  AdminPagination,
  AdminTable,
  adminAmount,
  adminDate,
} from "./admin-shared";

export interface ReconciliationDifference {
  kind: string;
  funding_source: string;
  token_id?: number;
  reference?: string;
  expected_nano: string;
  actual_nano: string;
  delta_nano: string;
}
export interface ReconciliationReport {
  id: number;
  date: string;
  user_id: number;
  username: string;
  timezone: string;
  usage_nano: string;
  api_ledger_nano: string;
  order_nano: string;
  recharge_ledger_nano: string;
  balance_expected_nano: string;
  balance_actual_nano: string;
  reserved_nano: string;
  differences: ReconciliationDifference[];
  provisional: boolean;
  created_at: number;
  external_status: string;
  imported_sources: string[];
  checks_version: number;
  admission_time_verified?: boolean;
}
interface Statement {
  id: number;
  provider: string;
  day: string;
  source: string;
  row_count: number;
  total_nano: string;
  created_at: number;
  supersedes: number;
}
type Text = ReturnType<typeof useStaffText>;
export function reconciliationStatus(
  r: Pick<
    ReconciliationReport,
    "checks_version" | "differences" | "provisional"
  >,
  t: Text,
) {
  if (r.checks_version !== 2)
    return t(...translatedMessage("recheck_required_85ada1f5d3"));
  if (r.differences.length)
    return t(...translatedMessage("differences_found_384b9ede6f"));
  return r.provisional
    ? t(...translatedMessage("no_differences_so_far_today_09bc70fed4"))
    : t(...translatedMessage("internal_checks_match_5188b672dc"));
}
export function differenceLabel(kind: string, t: Text) {
  const labels: Record<string, [string, string, string]> = {
    api_usage_vs_ledger: translatedMessage(
      "usage_versus_ledger_debits_7f2d138a49",
    ),
    balance_vs_ledger: translatedMessage(
      "account_balance_versus_ledger_0ff586870f",
    ),
    wallet_vs_ledger: translatedMessage(
      "simulated_wallet_versus_its_ledger_e75bf16b72",
    ),
    invalid_reservation: translatedMessage(
      "invalid_reserved_amount_c84dd775cd",
    ),
    key_usage_vs_details: translatedMessage(
      "daily_key_usage_versus_calls_db339f9685",
    ),
    key_counter_vs_details: translatedMessage(
      "key_total_usage_versus_calls_338ba22d9e",
    ),
    paid_orders_vs_ledger: translatedMessage(
      "local_paid_orders_versus_credits_32c3a11144",
    ),
    provider_order_vs_ledger: translatedMessage(
      "provider_payment_versus_credits_6a19b26f90",
    ),
    ledger_without_provider_order: translatedMessage(
      "credit_without_a_provider_payment_e41fe64e73",
    ),
    duplicate_provider_mapping: translatedMessage(
      "duplicate_platform_order_mapping_a7e629a274",
    ),
  };
  return labels[kind]
    ? t(...labels[kind])
    : t(...translatedMessage("other_reconciliation_difference_7e186da3eb"));
}
function externalLabel(status: string, t: Text) {
  if (status === "imported_scope")
    return t(...translatedMessage("imported_statements_checked_1d6b628be5"));
  if (status === "timezone_changed")
    return t(
      ...translatedMessage("timezone_changed_reimport_required_c542dbf562"),
    );
  return t(...translatedMessage("provider_statements_unverified_bf21ea1c5f"));
}
const csvHeader =
  "provider_order_id,platform_order_id,user_id,amount_cents,currency,paid_at";

function importFailure(error: Error | null, t: Text): Error | null {
  if (!error) return null;
  const row = error.message.match(/CSV row (\d+)/)?.[1];
  if (row)
    return new Error(
      t(
        ...translatedMessage(
          "row_is_invalid_check_duplicate_orders_account_8eea0710f5",
          { p0: row },
        ),
      ),
    );
  if (error.message.includes("version changed"))
    return new Error(
      t(
        ...translatedMessage(
          "the_statement_version_changed_refresh_import__7d4ec28f99",
        ),
      ),
    );
  if (error.message.includes("account does not exist"))
    return new Error(
      t(
        ...translatedMessage(
          "a_customer_account_id_in_the_statement_does_n_0a36585a88",
        ),
      ),
    );
  if (error.message.includes("another day"))
    return new Error(
      t(
        ...translatedMessage(
          "a_provider_order_already_appears_on_another_d_bb230a9620",
        ),
      ),
    );
  return new Error(
    t(
      ...translatedMessage(
        "statement_processing_did_not_complete_check_i_c9af03393d",
      ),
    ),
  );
}

export function AdminReconciliation() {
  const fileInput = useRef<HTMLInputElement>(null);
  const t = useStaffText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const [user, setUser] = useState(""),
    [end, setEnd] = useState(""),
    [page, setPage] = useState(1),
    [selectedID, setSelectedID] = useState<number | null>(null);
  const [importOpen, setImportOpen] = useState(false),
    [provider, setProvider] = useState(""),
    [importDay, setImportDay] = useState(""),
    [file, setFile] = useState<{ name: string; csv: string } | null>(null),
    [fileError, setFileError] = useState("");
  const params = new URLSearchParams({ days: "14" });
  if (user) params.set("user_id", user);
  if (end) params.set("end", end);
  const query = useQuery({
    queryKey: ["admin-reconciliation", ...identity, params.toString()],
    queryFn: () =>
      apiGet<{
        items: ReconciliationReport[];
        timezone: string;
        end: string;
        alert_webhook_configured: boolean;
        pending_alerts: number;
      }>(`/api/admin/reconciliation?${params}`),
    retry: false,
  });
  const customers = useQuery({
    queryKey: ["reconciliation-customers", ...identity],
    queryFn: () =>
      apiGet<{ items: { id: number; username: string }[]; total: number }>(
        "/api/admin/customers?page_size=100",
      ),
    retry: false,
  });
  const statements = useQuery({
    queryKey: ["payment-statements", ...identity, importDay],
    queryFn: () =>
      apiGet<{ items: Statement[] }>(
        `/api/admin/reconciliation/statements?day=${encodeURIComponent(importDay)}`,
      ),
    retry: false,
  });
  const refresh = async () => {
    await Promise.all([
      cache.invalidateQueries({ queryKey: ["admin-reconciliation"] }),
      cache.invalidateQueries({ queryKey: ["payment-statements"] }),
    ]);
  };
  const run = useMutation({
    mutationFn: () => apiPost(`/api/admin/reconciliation?${params}`, {}),
    onSuccess: refresh,
    retry: false,
  });
  const previous = statements.data?.items.find(
    (s) => s.provider === provider && s.day === importDay,
  );
  const upload = useMutation({
    mutationFn: async () => {
      await apiPost("/api/admin/reconciliation/statements", {
        provider,
        day: importDay,
        source: file!.name,
        csv: file!.csv,
        expected_previous_id: previous?.id ?? 0,
      });
      await apiPost(`/api/admin/reconciliation?days=1&end=${importDay}`, {});
    },
    onSuccess: async () => {
      await refresh();
      setImportOpen(false);
      setFile(null);
    },
    retry: false,
  });
  const rows = query.data?.items ?? [],
    selected = rows.find((r) => r.id === selectedID);
  const amount = (value: string) => (
    <bdi dir="ltr" className="reconciliation-money">
      {formatMoney(value, "detail")}{" "}
      <small>
        {value} {t(...translatedMessage("nano_usd_3e3a4d2472"))}
      </small>
    </bdi>
  );
  return (
    <section className="adm-page adm-reconciliation">
      <header className="adm-heading">
        <div>
          <h1>
            {t(...translatedMessage("financial_reconciliation_e3ba8c2ef5"))}
          </h1>
          <p>
            {t(
              ...translatedMessage(
                "daily_checks_at_02_00_regional_time_late_sett_1409d199f8",
              ),
            )}
          </p>
        </div>
        <div className="reconciliation-actions">
          <Button
            onClick={() => {
              setImportDay(end || query.data?.end || "");
              upload.reset();
              setImportOpen(true);
            }}
          >
            {t(...translatedMessage("import_statement_1557250744"))}
          </Button>
          <Button
            variant="primary"
            disabled={run.isPending || !query.data}
            onClick={() => run.mutate()}
          >
            {run.isPending
              ? t(...translatedMessage("reconciling_36f92a34c0"))
              : t(...translatedMessage("reconcile_ced0b1407e"))}
          </Button>
        </div>
      </header>
      <div className="adm-notice">
        {t(
          ...translatedMessage(
            "checks_cover_balances_charges_credits_and_key_5cad991db4",
          ),
        )}
      </div>
      {query.data && !query.data.alert_webhook_configured && (
        <p className="reconciliation-warning">
          {t(
            ...translatedMessage(
              "difference_alerts_are_retained_external_notif_182cb6ea11",
            ),
          )}
        </p>
      )}
      <div className="adm-filters">
        <label>
          {t(...translatedMessage("account_b00965aefc"))}
          <select
            value={user}
            onChange={(e) => {
              setUser(e.target.value);
              setPage(1);
            }}
          >
            <option value="">
              {t(...translatedMessage("all_accounts_5a521a4986"))}
            </option>
            {customers.data?.items.map((c) => (
              <option key={c.id} value={c.id}>
                {c.username} · {c.id}
              </option>
            ))}
          </select>
        </label>
        {(customers.data?.total ?? 0) > 100 && (
          <label>
            {t(...translatedMessage("account_id_267663f396"))}
            <Input
              type="number"
              min={1}
              value={user}
              onChange={(e) => {
                setUser(e.target.value);
                setPage(1);
              }}
            />
          </label>
        )}
        <label>
          {t(...translatedMessage("end_date_last_14_days_ff8609c3c9"))}
          <Input
            type="date"
            value={end || query.data?.end || ""}
            onChange={(e) => {
              setEnd(e.target.value);
              setPage(1);
            }}
          />
        </label>
        <span>{query.data?.timezone}</span>
      </div>
      <AdminFeedback
        pending={query.isPending}
        error={query.error ?? run.error ?? customers.error}
        retry={() => void query.refetch()}
      />
      {query.data && (
        <>
          <AdminTable
            label={t(
              ...translatedMessage("daily_reconciliation_reports_27de52296f"),
            )}
          >
            <thead>
              <tr>
                {[
                  t(...translatedMessage("day_account_b5d6021c3b")),
                  t(...translatedMessage("usage_ledger_debits_2cf1405bd3")),
                  t(...translatedMessage("paid_orders_credits_7d67beb3aa")),
                  t(...translatedMessage("internal_checks_6b88e61fb4")),
                  t(...translatedMessage("provider_checks_562ccb1286")),
                  t(...translatedMessage("actions_d83f4ed14a")),
                ].map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.slice((page - 1) * 20, page * 20).map((r) => (
                <tr key={r.id}>
                  <td>
                    <bdi>{r.date}</bdi>
                    <small>
                      <bdi>{r.username || `usr_${r.user_id}`}</bdi>
                    </small>
                  </td>
                  <td>
                    <bdi>{adminAmount(r.usage_nano)}</bdi>
                    <small>
                      <bdi>{adminAmount(r.api_ledger_nano)}</bdi>
                    </small>
                  </td>
                  <td>
                    <bdi>{adminAmount(r.order_nano)}</bdi>
                    <small>
                      <bdi>{adminAmount(r.recharge_ledger_nano)}</bdi>
                    </small>
                  </td>
                  <td>
                    <span
                      className={
                        r.differences.length ? "reconciliation-warning" : ""
                      }
                    >
                      {reconciliationStatus(r, t)}
                    </span>
                    {r.differences.length > 0 && (
                      <small>
                        {r.differences.length}{" "}
                        {t(...translatedMessage("differences_02d8cbe1c8"))}
                      </small>
                    )}
                    {r.admission_time_verified === false && (
                      <small>
                        {t(
                          ...translatedMessage(
                            "historical_admission_times_unverified_55a88fda7f",
                          ),
                        )}
                      </small>
                    )}
                    {r.provisional && r.differences.length > 0 && (
                      <small>
                        {t(
                          ...translatedMessage(
                            "today_still_in_progress_c42cf7050c",
                          ),
                        )}
                      </small>
                    )}
                  </td>
                  <td>{externalLabel(r.external_status, t)}</td>
                  <td>
                    <Button variant="ghost" onClick={() => setSelectedID(r.id)}>
                      {t(...translatedMessage("details_a91e8f9bea"))}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
          {!rows.length && (
            <p>
              {t(
                ...translatedMessage(
                  "no_reports_yet_choose_reconcile_to_generate_t_faf9c69c05",
                ),
              )}
            </p>
          )}
          <AdminPagination page={page} total={rows.length} onChange={setPage} />
        </>
      )}
      <Drawer
        open={selectedID !== null}
        onOpenChange={(v) => {
          if (!v) setSelectedID(null);
        }}
        title={t(...translatedMessage("reconciliation_details_dc5ae1ef3e"))}
        description={
          selected
            ? `${selected.date} · ${selected.username || selected.user_id}`
            : ""
        }
      >
        {selected && (
          <div className="adm-page">
            <p>
              {adminDate(selected.created_at, selected.timezone)} ·{" "}
              {selected.timezone}
            </p>
            <strong>{reconciliationStatus(selected, t)}</strong>
            {selected.checks_version === 2 && (
              <article className="adm-card">
                <h2>
                  {t(
                    ...translatedMessage(
                      "balance_snapshot_at_verification_06b48b01e0",
                    ),
                  )}
                </h2>
                <dl className="adm-info">
                  <div>
                    <dt>
                      {t(...translatedMessage("cumulative_ledger_cbd1ea9960"))}
                    </dt>
                    <dd>{amount(selected.balance_expected_nano)}</dd>
                  </div>
                  <div>
                    <dt>
                      {t(...translatedMessage("available_reserved_6927e4ae4c"))}
                    </dt>
                    <dd>{amount(selected.balance_actual_nano)}</dd>
                  </div>
                  <div>
                    <dt>
                      {t(...translatedMessage("reserved_portion_d06bf392be"))}
                    </dt>
                    <dd>{amount(selected.reserved_nano)}</dd>
                  </div>
                </dl>
              </article>
            )}
            {selected.differences.map((d, i) => (
              <article className="adm-card" key={i}>
                <h2>{differenceLabel(d.kind, t)}</h2>
                {d.token_id && (
                  <p>
                    {t(...translatedMessage("key_id_3e7cfc556d"))}{" "}
                    <bdi>{d.token_id}</bdi>
                  </p>
                )}
                {d.reference && (
                  <p>
                    {t(...translatedMessage("order_reference_a35feac16c"))}{" "}
                    <bdi>{d.reference}</bdi>
                  </p>
                )}
                <dl className="adm-info">
                  {[
                    [
                      t(...translatedMessage("expected_5de582cd3d")),
                      d.expected_nano,
                    ],
                    [
                      t(...translatedMessage("actual_fc419f1bb3")),
                      d.actual_nano,
                    ],
                    [
                      t(...translatedMessage("difference_be1a0c97aa")),
                      d.delta_nano,
                    ],
                  ].map(([label, value]) => (
                    <div key={label}>
                      <dt>{label}</dt>
                      <dd>{amount(value)}</dd>
                    </div>
                  ))}
                </dl>
              </article>
            ))}
            <article className="adm-card">
              <h2>{externalLabel(selected.external_status, t)}</h2>
              {selected.imported_sources?.map((s) => (
                <p key={s}>
                  <bdi>{s}</bdi>
                </p>
              ))}
            </article>
          </div>
        )}
      </Drawer>
      <Drawer
        open={importOpen}
        onOpenChange={(v) => {
          if (!upload.isPending) setImportOpen(v);
        }}
        title={t(
          ...translatedMessage(
            "import_successful_payment_statement_71cb9e78f8",
          ),
        )}
        description={t(
          ...translatedMessage(
            "use_the_complete_usd_statement_for_one_provid_d8f29b377b",
          ),
        )}
      >
        <form
          className="adm-page"
          onSubmit={(e) => {
            e.preventDefault();
            upload.mutate();
          }}
        >
          <label>
            {t(...translatedMessage("provider_identifier_16118f0c4c"))}
            <Input
              required
              value={provider}
              pattern="[A-Za-z0-9][A-Za-z0-9._-]{0,63}"
              placeholder="payment-provider"
              onChange={(e) => setProvider(e.target.value)}
            />
          </label>
          <label>
            {t(...translatedMessage("statement_date_86676bc86f"))}
            <Input
              required
              type="date"
              max={query.data?.end}
              value={importDay}
              onChange={(e) => setImportDay(e.target.value)}
            />
          </label>
          <p>
            {t(
              ...translatedMessage(
                "amounts_are_integer_cents_in_usd_paid_at_uses_958e1fe662",
              ),
            )}
          </p>
          <a
            download="payment-statement-template.csv"
            href={`data:text/csv;charset=utf-8,${encodeURIComponent(csvHeader + "\n")}`}
          >
            {t(...translatedMessage("download_csv_template_f70098f43c"))}
          </a>
          <label>
            {t(
              ...translatedMessage("statement_file_csv_up_to_2_mb_1f4ef3795c"),
            )}
            <input
              ref={fileInput}
              hidden
              type="file"
              accept=".csv,text/csv"
              onChange={async (e) => {
                setFile(null);
                setFileError("");
                const f = e.target.files?.[0];
                if (!f) return;
                if (f.size > 2 * 1024 * 1024) {
                  setFileError(
                    t(...translatedMessage("file_exceeds_2_mb_7af3d4928a")),
                  );
                  return;
                }
                try {
                  setFile({ name: f.name, csv: await f.text() });
                } catch {
                  setFileError(
                    t(...translatedMessage("cannot_read_file_e698f2a5a3")),
                  );
                }
              }}
            />
            <Button type="button" onClick={() => fileInput.current?.click()}>
              {t(...translatedMessage("choose_csv_file_e9ae7e1d76"))}
            </Button>
          </label>
          {file && (
            <p>
              <bdi>{file.name}</bdi> ·{" "}
              {t(
                ...translatedMessage(
                  "every_row_is_validated_before_import_balances_853e524fdd",
                ),
              )}
            </p>
          )}
          {previous && (
            <p className="reconciliation-warning">
              {t(
                ...translatedMessage(
                  "a_statement_already_exists_this_imports_a_com_2403d37c26",
                ),
              )}{" "}
              #{previous.id}
            </p>
          )}
          {fileError && <p role="alert">{fileError}</p>}
          <AdminFeedback
            pending={false}
            error={importFailure(upload.error, t) ?? statements.error}
            retry={() => void statements.refetch()}
          />
          <Button
            type="submit"
            variant="primary"
            disabled={
              !file ||
              !provider ||
              !importDay ||
              statements.isPending ||
              !!statements.error ||
              upload.isPending
            }
          >
            {upload.isPending
              ? t(...translatedMessage("importing_and_checking_f08191f3fd"))
              : t(...translatedMessage("import_and_reconcile_0f32d7cc77"))}
          </Button>
          <h2>
            {t(...translatedMessage("import_history_for_this_day_31786af210"))}
          </h2>
          {statements.data?.items.map((s) => (
            <article className="adm-card" key={s.id}>
              <strong>
                <bdi>
                  {s.provider} · #{s.id}
                </bdi>
              </strong>
              <p>
                <bdi>{s.source}</bdi>
              </p>
              <p>
                {s.row_count}{" "}
                {t(...translatedMessage("successful_payments_96d5efd6b9"))} ·{" "}
                <bdi>{adminAmount(s.total_nano)}</bdi>
              </p>
              <small>{adminDate(s.created_at, query.data?.timezone)}</small>
            </article>
          ))}
        </form>
      </Drawer>
    </section>
  );
}
