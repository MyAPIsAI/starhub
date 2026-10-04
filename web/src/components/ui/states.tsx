import { Inbox, CloudOff, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { DataState } from "../../lib/preferences";
import { Button } from "./button";

export function DataStateView({
  state,
  onReset,
  filtered = false,
}: {
  state: Exclude<DataState, "ready">;
  onReset: () => void;
  filtered?: boolean;
}) {
  const { t } = useTranslation();
  if (state === "loading")
    return (
      <div
        className="data-state loading-state"
        role="status"
        aria-label={t("loading")}
      >
        <span className="sr-only">{t("loading")}</span>
        <div className="skeleton skeleton-title" />
        <div className="skeleton-row">
          <div className="skeleton" />
          <div className="skeleton" />
          <div className="skeleton" />
        </div>
        <div className="skeleton skeleton-line" />
        <div className="skeleton skeleton-line" />
        <div className="skeleton skeleton-line short" />
      </div>
    );
  const error = state === "error";
  const Icon = error ? CloudOff : Inbox;
  return (
    <div className="data-state" role={error ? "alert" : "status"}>
      <span className="state-icon">
        <Icon size={22} />
      </span>
      <h3>{t(error ? "errorTitle" : "emptyTitle")}</h3>
      <p>{t(error ? "errorDescription" : "emptyDescription")}</p>
      <Button onClick={onReset}>
        {error && <RefreshCw />}
        {t(error ? "retry" : filtered ? "clearFilters" : "loadSample")}
      </Button>
    </div>
  );
}
