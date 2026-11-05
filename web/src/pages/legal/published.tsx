import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { PublicHeader } from "../../components/product-shell";
import { usePreferences } from "../../lib/preferences";
import { apiGet, ApiError } from "../../lib/api/http";
import { Button } from "../../components/ui/button";
import DemoLegalPage from "./index";

export default function PublishedLegalPage() {
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
  const titles: Record<string, [string, string]> = {
    terms: ["服务条款", "Terms of service"],
    privacy: ["隐私政策", "Privacy policy"],
    cookies: ["Cookie 政策", "Cookie policy"],
    refund: ["退款政策", "Refund policy"],
  };
  return (
    <div className="brand-home public-refined" style={{ minHeight: "100vh" }}>
      <PublicHeader />
      <main
        className="public-page-container"
        style={{ maxWidth: 780, padding: "var(--space-8) var(--space-6)" }}
      >
        <Link to="/">{lang === "zh" ? "返回首页" : "Back to home"}</Link>
        <nav className="adm-actions" style={{ marginBlock: 24 }}>
          {Object.entries(titles).map(([key, title]) => (
            <Link key={key} to={`/legal/${key}`}>
              {title[lang === "zh" ? 0 : 1]}
            </Link>
          ))}
        </nav>
        <h1>{titles[doc]?.[lang === "zh" ? 0 : 1] ?? doc}</h1>
        {query.isPending && (
          <p role="status">{lang === "zh" ? "正在加载…" : "Loading…"}</p>
        )}
        {query.error && (
          <div role="alert">
            <p>
              {lang === "zh"
                ? "暂时无法读取已发布的法律文本。"
                : "Unable to load the published legal document."}
            </p>
            <Button onClick={() => void query.refetch()}>
              {lang === "zh" ? "重试" : "Retry"}
            </Button>
          </div>
        )}
        {query.data && (
          <article>
            <p>
              {query.data.version} ·{" "}
              {new Intl.DateTimeFormat(lang, {
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
