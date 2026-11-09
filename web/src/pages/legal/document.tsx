import { useTranslation } from "react-i18next";
import { usePreferences } from "../../lib/preferences";
import { regionalLocale } from "../../lib/locale-format";
import { usePublishedLegal, type LegalKind } from "../../lib/api/legal";
import { ApiError } from "../../lib/api/http";
import { Button } from "../../components/ui/button";
import "./legal.css";
export const legalTitleKeys = {
  terms: "legalTermsTitle",
  privacy: "legalPrivacyTitle",
  cookies: "legalCookiesTitle",
  refund: "legalRefundTitle",
} as const;
export function LegalDocumentContent({ kind }: { kind: LegalKind }) {
  const { t } = useTranslation();
  const language = usePreferences((s) => s.language);
  const query = usePublishedLegal(kind, language);
  if (query.isPending) return <p role="status">{t("loading")}</p>;
  if (query.error) {
    const missing =
      query.error instanceof ApiError && query.error.status === 404;
    return (
      <div className="legal-feedback" role={missing ? "status" : "alert"}>
        <h2>{t(missing ? "legalUnpublished" : "legalLoadFailed")}</h2>
        <p>{t(missing ? "legalUnpublishedHelp" : "legalRetryHelp")}</p>
        <Button variant="secondary" onClick={() => void query.refetch()}>
          {t("retry")}
        </Button>
      </div>
    );
  }
  if (!query.data) return null;
  const doc = query.data;
  return (
    <article className="published-legal" data-legal-version={doc.version}>
      <dl className="legal-metadata">
        <div>
          <dt>{t("legalVersion")}</dt>
          <dd>
            <bdi>{doc.version}</bdi>
          </dd>
        </div>
        <div>
          <dt>{t("legalEffective")}</dt>
          <dd>
            <time dateTime={new Date(doc.effective_at * 1000).toISOString()}>
              {new Intl.DateTimeFormat(regionalLocale(language), {
                dateStyle: "medium",
                timeStyle: "short",
                timeZone: doc.timezone,
              }).format(new Date(doc.effective_at * 1000))}
            </time>{" "}
            · <bdi>{doc.timezone}</bdi>
          </dd>
        </div>
      </dl>
      <div
        className="legal-document-body"
        lang={doc.language}
        dir={doc.language === "ar" ? "rtl" : "ltr"}
      >
        {doc.content}
      </div>
    </article>
  );
}
