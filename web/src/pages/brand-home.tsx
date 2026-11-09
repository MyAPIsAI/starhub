import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  ArrowUpRight,
  ShieldCheck,
  SlidersHorizontal,
  ListChecks,
} from "lucide-react";
import { PublicHeader, ProductBrand } from "../components/product-shell";
import { HomePortal } from "../components/home-portal";
import { CookieSettingsButton } from "../components/cookie-consent";
import { ListPagination, useListPage } from "../components/list-pagination";
import { Button } from "../components/ui/button";
import { usePublicHome } from "../lib/api/public-pages";
import { usePreferences } from "../lib/preferences";
import { legalKinds } from "../lib/api/legal";
import { legalTitleKeys } from "./legal/document";
import AuthDialog from "./auth/auth-dialog";
import "../components/home-redesign.css";
import "./public-data-pages.css";

export default function BrandHome() {
  const { t } = useTranslation();
  const language = usePreferences((s) => s.language);
  const query = usePublicHome();
  const data = query.isError ? undefined : query.data;
  const groups = useListPage(data?.groups ?? [], "", 6);
  const [copied, setCopied] = useState(false),
    [copyFailed, setCopyFailed] = useState(false);
  const model = data?.models.find(
    (m) => m.tags.includes("text") && m.protocols.includes("OpenAI"),
  );
  const code = `curl 'https://api.myapis.ai/v1/chat/completions' \\\n  -H "Authorization: Bearer $MYAPIS_API_KEY" \\\n  -H 'Content-Type: application/json' \\\n  --data-raw '${JSON.stringify({ model: model?.id ?? "MODEL_ID", messages: [{ role: "user", content: "Hello" }] }, null, 2).replaceAll("'", "'\"'\"'")}'`;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  };
  return (
    <div className="xy-home xy-home-portal public-home">
      <PublicHeader />
      <main>
        <HomePortal language={language}>
          <div className="public-page-width">
            <section
              className="public-home-section"
              aria-labelledby="home-models-title"
            >
              <div className="public-section-heading">
                <div>
                  <h2 id="home-models-title">{t("publicHomeModels")}</h2>
                  <p>{t("publicHomeModelsHelp")}</p>
                </div>
                <Link to="/models">
                  {t("publicHomeAllModels")} <ArrowUpRight size={16} />
                </Link>
              </div>
              {query.isPending && <p role="status">{t("loading")}</p>}
              {query.isError && (
                <div role="alert">
                  <p>{t("publicHomeLoadError")}</p>
                  <Button onClick={() => void query.refetch()}>
                    {t("retry")}
                  </Button>
                </div>
              )}
              {data && (
                <>
                  <p className="public-muted">
                    {t("publicHomeDirectoryCounts", {
                      models: data.models.length,
                      vendors: new Set(data.models.map((m) => m.vendor)).size,
                    })}
                  </p>
                  <div className="public-home-cards">
                    {data.models.slice(0, 6).map((m) => (
                      <Link
                        className="public-home-card"
                        key={m.id}
                        to={`/models/${encodeURIComponent(m.id)}`}
                      >
                        <span>{m.vendor}</span>
                        <h3>{m.name}</h3>
                        <code dir="ltr">{m.id}</code>
                        <p>
                          {m.protocols.length
                            ? m.protocols.join(" · ")
                            : t("publicHomeProtocolsPending")}
                        </p>
                        <ArrowUpRight size={18} />
                      </Link>
                    ))}
                  </div>
                  {!data.models.length && <p>{t("publicHomeNoModels")}</p>}
                </>
              )}
            </section>
            <section
              className="public-home-section"
              aria-labelledby="home-groups-title"
            >
              <h2 id="home-groups-title">{t("publicHomeGroups")}</h2>
              <p>{t("publicHomeGroupsHelp")}</p>
              <div className="public-home-cards">
                {groups.rows.map((g) => (
                  <article className="public-home-card" key={g.id}>
                    <h3>{g.name}</h3>
                    <code dir="ltr">{g.id}</code>
                    <p>{g.description || t("publicHomeGroupDescription")}</p>
                  </article>
                ))}
              </div>
              {data && !data.groups.length && <p>{t("publicHomeNoGroups")}</p>}
              <ListPagination {...groups} />
            </section>
            <section className="public-home-section">
              <h2>{t("publicHomeControl")}</h2>
              <div className="public-home-cards">
                {[ShieldCheck, SlidersHorizontal, ListChecks].map((Icon, i) => (
                  <article className="public-home-card" key={i}>
                    <Icon size={22} />
                    <h3>
                      {t(
                        [
                          "publicHomeAccess",
                          "publicHomeLimits",
                          "publicHomeRecords",
                        ][i],
                      )}
                    </h3>
                    <p>
                      {t(
                        [
                          "publicHomeAccessHelp",
                          "publicHomeLimitsHelp",
                          "publicHomeRecordsHelp",
                        ][i],
                      )}
                    </p>
                  </article>
                ))}
              </div>
            </section>
            <section className="public-home-section public-home-code">
              <div>
                <h2>{t("publicHomeCode")}</h2>
                <p>{t("publicHomeCodeHelp")}</p>
                {!model && <p>{t("publicHomeCodeModel")}</p>}
                <Link to="/docs">
                  {t("publicHomeDocs")} <ArrowUpRight size={16} />
                </Link>
              </div>
              <div className="public-code">
                <div>
                  <bdi>cURL</bdi>
                  <Button variant="ghost" onClick={() => void copy()}>
                    {t(copied ? "copied" : "copy")}
                  </Button>
                </div>
                <pre dir="ltr" tabIndex={0}>
                  <code>{code}</code>
                </pre>
                {copyFailed && <p role="alert">{t("copyFailed")}</p>}
              </div>
            </section>
            <section className="public-home-section public-home-status">
              <div>
                <h2>{t("publicHomeStatus")}</h2>
                <p>{t("publicHomeStatusHelp")}</p>
              </div>
              <Link className="button button-primary" to="/status">
                {t("publicHomeViewStatus")}
              </Link>
            </section>
          </div>
        </HomePortal>
      </main>
      <footer className="xy-footer public-page-width">
        <ProductBrand compact />
        <nav>
          <Link to="/docs">{t("publicHomeDocs")}</Link>
          {legalKinds.map((kind) => (
            <Link key={kind} to={`/legal/${kind}`}>
              {t(legalTitleKeys[kind])}
            </Link>
          ))}
          <CookieSettingsButton />
        </nav>
      </footer>
      <AuthDialog />
    </div>
  );
}
