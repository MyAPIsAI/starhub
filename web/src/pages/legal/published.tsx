import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { PublicHeader } from "../../components/product-shell";
import { usePreferences } from "../../lib/preferences";
import { apiGet, ApiError } from "../../lib/api/http";
import { Button } from "../../components/ui/button";
import DemoLegalPage from "./index";
import { useTranslation } from "react-i18next";
import { regionalLocale } from "../../lib/locale-format";

export default function PublishedLegalPage() {
  const { t } = useTranslation();
  const { doc = "privacy" } = useParams(),
    lang = usePreferences((s) => s.language);
  const query = useQuery({
    queryKey: ["published-legal", doc, lang],
    queryFn: () =>
      apiGet<{
        kind: string;
        language: string;
        version: string;
        content: string;
        effective_at: number;
        timezone: string;
      }>(`/api/legal/${encodeURIComponent(doc)}?language=${lang}`),
    retry: false,
    staleTime: 0,
    refetchInterval: 30000,
  });
  if (query.error instanceof ApiError && query.error.status === 404)
    return <DemoLegalPage />;
  const titles: Record<string, string> = {
    terms: t("legalTermsTitle"),
    privacy: t("legalPrivacyTitle"),
    cookies: t("legalCookiesTitle"),
    refund: t("legalRefundTitle"),
  };
  return (
    <div className="brand-home public-refined" style={{ minHeight: "100vh" }}>
      <PublicHeader />
      <main
        className="public-page-container"
        style={{ maxWidth: 780, padding: "var(--space-8) var(--space-6)" }}
      >
        <Link to="/">{t("legalBackHome")}</Link>
        <nav className="adm-actions" style={{ marginBlock: 24 }}>
          {Object.entries(titles).map(([key, title]) => (
            <Link key={key} to={`/legal/${key}`}>
              {title}
            </Link>
          ))}
        </nav>
        <h1>{titles[doc] ?? doc}</h1>
        {query.isPending && <p role="status">{t("loading")}</p>}
        {query.error && (
          <div role="alert">
            <p>{t("legalLoadFailed")}</p>
            <Button onClick={() => void query.refetch()}>{t("retry")}</Button>
          </div>
        )}
        {query.data && (
          <article>
            <p>
              {query.data.version} ·{" "}
              {new Intl.DateTimeFormat(regionalLocale(lang), {
                dateStyle: "medium",
                timeStyle: "short",
                timeZone: query.data.timezone,
              }).format(new Date(query.data.effective_at * 1000))}{" "}
              · {query.data.timezone}
            </p>
            <div
              style={{
                whiteSpace: "pre-wrap",
                overflowWrap: "anywhere",
                lineHeight: 1.8,
              }}
            >
              {query.data.content}
            </div>
          </article>
        )}
      </main>
    </div>
  );
}
