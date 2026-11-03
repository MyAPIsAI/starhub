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
    requests: c("调用明细", "Requests", "الطلبات"),
    ledger: c("账本流水", "Ledger", "السجل المالي"),
    usage: c("用量报表", "Usage", "الاستخدام"),
  };
  return (
    <section className="export-history">
      <header>
        <div>
          <h1>{c("导出记录", "Exports", "سجل التصدير")}</h1>
          <p>
            {c(
              "文件保留 7 天；CSV 与字段说明使用创建任务时的界面语言。",
              "Files are retained for 7 days. CSV headers and field descriptions use the language selected at creation.",
              "تُحفظ الملفات 7 أيام. تستخدم العناوين وأوصاف الحقول لغة إنشاء المهمة.",
            )}
          </p>
        </div>
        <Button onClick={onCreate}>
          {c("创建导出", "Create export", "إنشاء تصدير")}
        </Button>
      </header>
      {(error || query.isError) && (
        <p role="alert">
          {error ||
            c(
              "导出记录加载失败，请重试。",
              "Could not load exports. Retry.",
              "تعذر تحميل السجلات. أعد المحاولة.",
            )}
          {query.isError && (
            <Button onClick={() => void query.refetch()}>
              {c("重试", "Retry", "إعادة المحاولة")}
            </Button>
          )}
        </p>
      )}
      {query.isLoading && (
        <p role="status">{c("正在加载…", "Loading…", "جارٍ التحميل…")}</p>
      )}
      {query.data?.length === 0 && (
        <p>{c("暂无导出记录。", "No exports yet.", "لا توجد ملفات تصدير.")}</p>
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
                {c("行数", "Rows", "الصفوف")}: {row.row_count} ·{" "}
                {c("到期", "Expires", "ينتهي")}:{" "}
                {new Date(row.expires_at * 1000).toLocaleDateString(
                  i18n.language,
                  { timeZone: row.timezone || "UTC" },
                )}
              </small>
            </div>
            <div className="export-history-actions">
              <span role="status">
                {row.status === "completed"
                  ? c("已完成", "Completed", "مكتمل")
                  : row.status === "processing"
                    ? c("生成中…", "Processing…", "قيد الإنشاء…")
                    : c(
                        "生成失败，请重新创建。",
                        "Failed. Create another export.",
                        "فشل. أنشئ تصديرًا جديدًا.",
                      )}
              </span>
              <Button
                disabled={row.status !== "completed"}
                onClick={() => void download(row)}
              >
                {c("下载 CSV", "Download CSV", "تنزيل CSV")}
              </Button>
              <Button
                disabled={row.status !== "completed" || !row.dictionary_url}
                onClick={() => void download(row, true)}
              >
                {c("字段说明", "Field descriptions", "أوصاف الحقول")}
              </Button>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
