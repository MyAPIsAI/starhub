import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "./ui/button";
import "./list-pagination.css";
export function useListPage<T>(rows: T[], resetKey: unknown = "", size = 20) {
  const [selected, setPage] = useState(1);
  const page = Math.max(
    1,
    Math.min(selected, Math.ceil(rows.length / size) || 1),
  );
  useEffect(() => setPage(1), [resetKey]);
  useEffect(() => {
    if (selected !== page) setPage(page);
  }, [selected, page]);
  return {
    rows: rows.slice((page - 1) * size, page * size),
    page,
    total: rows.length,
    size,
    onChange: setPage,
  };
}
export function ListPagination({
  page,
  total,
  size,
  onChange,
}: {
  page: number;
  total: number;
  size: number;
  onChange: (page: number) => void;
}) {
  const { t } = useTranslation();
  if (total <= size) return null;
  return (
    <nav className="list-pagination" aria-label={t("listPagination")}>
      <span>{t("listTotal", { count: total })}</span>
      <Button
        variant="secondary"
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
      >
        {t("listPrevious")}
      </Button>
      <bdi>
        {page} / {Math.max(1, Math.ceil(total / size))}
      </bdi>
      <Button
        variant="secondary"
        disabled={page * size >= total}
        onClick={() => onChange(page + 1)}
      >
        {t("listNext")}
      </Button>
    </nav>
  );
}
