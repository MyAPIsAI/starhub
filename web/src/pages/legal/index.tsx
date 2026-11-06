import { message as translatedMessage } from "../../i18n/messages";

import { useParams, Link } from "react-router-dom";
import { ArrowLeft, FileText, ShieldCheck, Scale } from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { usePreferences } from "../../lib/preferences";

type LegalDoc = "privacy" | "terms";

const docs: Record<
  LegalDoc,
  { title: [string, string, string]; updated: string }
> = {
  privacy: {
    title: translatedMessage("privacy_policy_759d9b374a"),
    updated: "2026-10-23",
  },
  terms: {
    title: translatedMessage("terms_of_service_327b56dce6"),
    updated: "2026-10-23",
  },
};

export default function LegalPage() {
  const { doc } = useParams<{ doc: string }>();
  const lang = usePreferences((s) => s.language);
  const t = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const key = (doc && doc in docs ? doc : "privacy") as LegalDoc;
  const info = docs[key];

  return (
    <div
      className="brand-home public-refined"
      style={{ minHeight: "100vh", display: "flex", flexDirection: "column" }}
    >
      <PublicHeader />
      <main
        className="public-page-container"
        style={{
          maxWidth: 780,
          padding: "var(--space-8) var(--space-6)",
          flex: 1,
        }}
      >
        <Link
          to="/"
          className="inline-flex items-center gap-1.5 text-sm"
          style={{ color: "var(--muted)" }}
        >
          <ArrowLeft size={15} />
          {t(...translatedMessage("back_to_home_f0dac3b02e"))}
        </Link>

        <nav
          style={{
            display: "flex",
            gap: "var(--space-2)",
            marginTop: "var(--space-6)",
            marginBottom: "var(--space-6)",
          }}
        >
          {(Object.keys(docs) as LegalDoc[]).map((id) => (
            <Link
              key={id}
              to={`/legal/${id}`}
              className="legal-nav-link"
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 6,
                padding: "6px 14px",
                borderRadius: "var(--radius-md)",
                fontSize: "var(--text-sm)",
                fontWeight: key === id ? 600 : 400,
                background: key === id ? "var(--brand)" : "var(--subtle)",
                color: key === id ? "#fff" : "var(--secondary)",
                textDecoration: "none",
                transition: "all .15s",
              }}
            >
              {id === "privacy" ? (
                <ShieldCheck size={14} />
              ) : (
                <Scale size={14} />
              )}
              {docs[id].title[lang === "zh" ? 0 : lang === "ar" ? 2 : 1]}
            </Link>
          ))}
        </nav>

        <article className="legal-article" style={{ lineHeight: 1.75 }}>
          <h1
            style={{
              fontSize: "var(--text-2xl)",
              fontWeight: "var(--weight-bold)",
              marginBottom: "var(--space-2)",
            }}
          >
            {info.title[lang === "zh" ? 0 : lang === "ar" ? 2 : 1]}
          </h1>
          <p
            style={{
              fontSize: "var(--text-xs)",
              color: "var(--muted)",
              marginBottom: "var(--space-8)",
            }}
          >
            {t(...translatedMessage("last_updated_ecf0f09811"))}: {info.updated}
          </p>

          {key === "privacy" ? (
            <>
              <h2>{t(...translatedMessage("overview_3b2443bd56"))}</h2>
              <p>
                {t(
                  ...translatedMessage(
                    "this_page_is_a_template_for_the_product_demo__7f29ba7736",
                  ),
                )}
              </p>
              <h2>
                {t(...translatedMessage("information_we_collect_d8d1d98c72"))}
              </h2>
              <ul>
                <li>
                  {t(
                    ...translatedMessage(
                      "account_information_username_email_address_e76fd1f50e",
                    ),
                  )}
                </li>
                <li>
                  {t(
                    ...translatedMessage(
                      "api_keys_only_hashes_are_stored_cde0f6e771",
                    ),
                  )}
                </li>
                <li>
                  {t(
                    ...translatedMessage(
                      "request_logs_model_token_counts_duration_cost_174a7af7e1",
                    ),
                  )}
                </li>
              </ul>
              <h2>{t(...translatedMessage("data_storage_515d3c38df"))}</h2>
              <p>
                {t(
                  ...translatedMessage(
                    "all_data_is_stored_in_a_local_sqlite_database_29264d558a",
                  ),
                )}
              </p>
              <h2>{t(...translatedMessage("contact_b4b1e6229e"))}</h2>
              <p>
                {t(
                  ...translatedMessage(
                    "for_questions_please_submit_an_issue_via_the__4a5a06c5e0",
                  ),
                )}
              </p>
            </>
          ) : (
            <>
              <h2>{t(...translatedMessage("overview_3b2443bd56"))}</h2>
              <p>
                {t(
                  ...translatedMessage(
                    "this_page_is_a_template_for_the_product_demo__a6ebed5743",
                  ),
                )}
              </p>
              <h2>{t(...translatedMessage("scope_of_service_6aee9a0d43"))}</h2>
              <p>
                {t(
                  ...translatedMessage(
                    "this_platform_provides_a_multi_model_api_gate_7ec50ba171",
                  ),
                )}
              </p>
              <h2>{t(...translatedMessage("billing_d224f95517"))}</h2>
              <p>
                {t(
                  ...translatedMessage(
                    "the_platform_bills_by_actual_token_usage_or_m_5a25bb5173",
                  ),
                )}
              </p>
              <h2>
                {t(...translatedMessage("limitation_of_liability_a161fb5cc2"))}
              </h2>
              <p>
                {t(
                  ...translatedMessage(
                    "the_demo_environment_provides_no_sla_commitme_d44d1cfeda",
                  ),
                )}
              </p>
            </>
          )}

          <footer
            style={{
              marginTop: "var(--space-10)",
              paddingTop: "var(--space-4)",
              borderTop: "1px solid var(--border)",
              display: "flex",
              alignItems: "center",
              gap: 6,
              fontSize: "var(--text-xs)",
              color: "var(--muted)",
            }}
          >
            <FileText size={13} />
            {t(
              ...translatedMessage(
                "demo_document_not_a_legal_commitment_39e8445dc1",
              ),
            )}
          </footer>
        </article>
      </main>
    </div>
  );
}
