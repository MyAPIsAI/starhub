import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import { Button } from "./ui/button";
import { Modal } from "./ui/overlay";
import { useLegalPolicy } from "../lib/api/legal";
import {
  hasNecessaryConsent,
  saveNecessaryConsent,
  useCookieSettings,
  consentKey,
} from "../lib/cookie-consent";
import "./cookie-consent.css";
export function CookieSettingsButton() {
  const { t } = useTranslation();
  const show = useCookieSettings((s) => s.show);
  return (
    <button type="button" className="cookie-settings-button" onClick={show}>
      {t("cookieSettings")}
    </button>
  );
}
export default function CookieConsent() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const query = useLegalPolicy();
  const { open, close } = useCookieSettings();
  const [, refresh] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [storageFailed, setStorageFailed] = useState(false);
  useEffect(() => {
    const update = (e: StorageEvent) => {
      if (e.key === consentKey || e.key === null) refresh((v) => v + 1);
    };
    window.addEventListener("storage", update);
    return () => window.removeEventListener("storage", update);
  }, []);
  useEffect(() => {
    setDismissed(false);
    close();
  }, [pathname, close]);
  const publicPage =
    !pathname.startsWith("/admin") &&
    !pathname.startsWith("/console") &&
    !pathname.startsWith("/preview/console") &&
    !["/finance", "/login", "/register", "/forgot-password"].includes(pathname);
  if (!publicPage) return null;
  const policy = query.data;
  const automatic = Boolean(
    policy?.cookie_consent && !hasNecessaryConsent(policy) && !dismissed,
  );
  if (!open && !automatic) return null;
  const accept = () => {
    if (!policy) return;
    const saved = saveNecessaryConsent(policy);
    setStorageFailed(!saved);
    refresh((v) => v + 1);
    if (saved) close();
    else useCookieSettings.getState().show();
  };
  const finish = () => {
    close();
    setDismissed(true);
  };
  const content = (
    <>
      <p>{t("cookieNecessaryBody")}</p>
      <Link to="/legal/cookies" onClick={finish}>
        {t("legalCookiesTitle")}
      </Link>
      {query.isPending && <p role="status">{t("loading")}</p>}
      {query.error && (
        <div role="alert">
          <p>{t("cookiePolicyFailed")}</p>
          <Button onClick={() => void query.refetch()}>{t("retry")}</Button>
        </div>
      )}
      {storageFailed && <p role="status">{t("cookieStorageFailed")}</p>}
    </>
  );
  const actions = (
    <Button
      variant="primary"
      disabled={!policy || !!query.error}
      onClick={accept}
    >
      {t("cookieNecessaryOnly")}
    </Button>
  );
  if (
    open ||
    (policy?.cookie_consent_mode === "dialog" && pathname !== "/legal/cookies")
  )
    return (
      <Modal
        open
        title={t("cookieTitle")}
        description={t("cookieNecessarySummary")}
        onOpenChange={(v) => {
          if (!v) finish();
        }}
        size="compact"
        className="cookie-dialog"
        footer={actions}
      >
        {content}
      </Modal>
    );
  return (
    <section
      className="cookie-banner"
      role="region"
      aria-label={t("cookieTitle")}
    >
      <div>
        <h2>{t("cookieTitle")}</h2>
        {content}
      </div>
      <div className="cookie-banner-actions">{actions}</div>
    </section>
  );
}
