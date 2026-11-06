import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "../components/ui/button";
import {
  downloadExport,
  useExports,
  type ExportRecord,
} from "../lib/api/exports";
import "../components/export-history.css";

export default function ExportHistory({ onCreate }: { onCreate: () => void }) {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const query = useExports();
  const [error, setError] = useState("");
  const download = async (row: ExportRecord, dictionary = false) => {
    setError("");
    try {
      await downloadExport(
        row.id,
        dictionary
          ? row.filename.replace(/\.csv$/, "-fields.csv")
          : row.filename,
        dictionary ? row.dictionary_url : row.download_url,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };
  const types = {
    requests: c(...translatedMessage("requests_bf1d814671")),
    ledger: c(...translatedMessage("ledger_663f871935")),
    usage: c(...translatedMessage("usage_8c28029dbc")),
  };
  return (
    <section className="export-history">
      <header>
        <div>
          <h1>{c(...translatedMessage("exports_b2df1f0c09"))}</h1>
          <p>
            {c(
              ...translatedMessage(
                "files_are_retained_for_7_days_csv_headers_and_77c18e94d9",
              ),
            )}
          </p>
        </div>
        <Button onClick={onCreate}>
          {c(...translatedMessage("create_export_ba0af46356"))}
        </Button>
      </header>
      {(error || query.isError) && (
        <p role="alert">
          {error ||
            c(...translatedMessage("could_not_load_exports_retry_faf75ef72d"))}
          {query.isError && (
            <Button onClick={() => void query.refetch()}>
              {c(...translatedMessage("retry_c16455021c"))}
            </Button>
          )}
        </p>
      )}
      {query.isLoading && (
        <p role="status">{c(...translatedMessage("loading_f076a9d0d1"))}</p>
      )}
      {query.data?.length === 0 && (
        <p>{c(...translatedMessage("no_exports_yet_39a30835d8"))}</p>
      )}
      <div className="export-history-list">
        {query.data?.map((row) => (
          <article key={row.id}>
            <div>
              <strong>{types[row.type]}</strong>
              <p dir="ltr">
                {row.start_date} — {row.end_date} · {row.timezone}
              </p>
              <small>
                {c(...translatedMessage("rows_c6b018fbba"))}: {row.row_count} ·{" "}
                {c(...translatedMessage("expires_faf14f50a3"))}:{" "}
                {new Date(row.expires_at * 1000).toLocaleDateString(
                  i18n.language,
                  { timeZone: row.timezone || "UTC" },
                )}
              </small>
            </div>
            <div className="export-history-actions">
              <span role="status">
                {row.status === "completed"
                  ? c(...translatedMessage("completed_190da088f0"))
                  : row.status === "processing"
                    ? c(...translatedMessage("processing_4d6bfc341f"))
                    : c(
                        ...translatedMessage(
                          "failed_create_another_export_84db85cc08",
                        ),
                      )}
              </span>
              <Button
                disabled={row.status !== "completed"}
                onClick={() => void download(row)}
              >
                {c(...translatedMessage("download_csv_d846ce8a1d"))}
              </Button>
              <Button
                disabled={row.status !== "completed" || !row.dictionary_url}
                onClick={() => void download(row, true)}
              >
                {c(...translatedMessage("field_descriptions_10060f8660"))}
              </Button>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
