import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { legalKinds, type LegalKind } from "../../lib/api/legal";
import { LegalDocumentContent, legalTitleKeys } from "./document";
import { CookieSettingsButton } from "../../components/cookie-consent";
export default function PublishedLegalPage() {
  const { t } = useTranslation();
  const { doc = "privacy" } = useParams();
  const valid = legalKinds.includes(doc as LegalKind);
  return (
    <div className="brand-home public-refined legal-page">
      <PublicHeader />
      <main className="public-page-container legal-container">
        <Link to="/" className="legal-back">
          <ArrowLeft className="directional" size={15} />
          {t("legalBackHome")}
        </Link>
        <nav className="legal-nav" aria-label={t("legalNavigation")}>
          {legalKinds.map((kind) => (
            <Link
              key={kind}
              to={`/legal/${kind}`}
              aria-current={kind === doc ? "page" : undefined}
            >
              {t(legalTitleKeys[kind])}
            </Link>
          ))}
        </nav>
        <h1>
          {valid ? t(legalTitleKeys[doc as LegalKind]) : t("legalNotFound")}
        </h1>
        {valid ? (
          <LegalDocumentContent kind={doc as LegalKind} />
        ) : (
          <p role="status">{t("legalNotFoundHelp")}</p>
        )}
        {doc === "cookies" && (
          <div className="legal-settings">
            <CookieSettingsButton />
          </div>
        )}
      </main>
    </div>
  );
}
