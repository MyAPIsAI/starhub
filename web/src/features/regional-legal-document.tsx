import { useEffect, useState } from "react";
import { apiGet } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import { Button } from "../components/ui/button";

interface PublishedLegalDocument {
  type: string;
  version: string;
  language: string;
  content: string;
  published_at: number;
}

export default function RegionalLegalDocument({
  policy,
  expectedVersion,
  headingLevel = 2,
}: {
  policy: "terms" | "privacy";
  expectedVersion: string;
  headingLevel?: 1 | 2;
}) {
  const Heading = headingLevel === 1 ? "h1" : "h2";
  const language = usePreferences((state) => state.language);
  const [document, setDocument] = useState<PublishedLegalDocument | null>(null);
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  useEffect(() => {
    const abort = new AbortController();
    setDocument(null);
    setError(false);
    const kind = policy === "terms" ? "user_agreement" : "privacy_policy";
    apiGet<PublishedLegalDocument>(
      `/api/legal/${kind}?lang=${encodeURIComponent(language)}`,
      abort.signal,
    )
      .then((value) => {
        if (abort.signal.aborted) return;
        if (
          value.version !== expectedVersion ||
          typeof value.content !== "string"
        ) {
          setError(true);
          return;
        }
        setDocument(value);
      })
      .catch(() => {
        if (!abort.signal.aborted) setError(true);
      });
    return () => abort.abort();
  }, [policy, expectedVersion, language, attempt]);
  return (
    <article className="regional-legal-document">
      <Heading>
        {policy === "terms"
          ? c("服务协议", "Terms of service", "اتفاقية الخدمة")
          : c("隐私政策", "Privacy policy", "سياسة الخصوصية")}
      </Heading>
      {error ? (
        <div role="alert">
          <p>
            {c(
              "无法加载当前法律文本，请重试。",
              "Unable to load the current legal document. Please retry.",
              "تعذر تحميل الوثيقة القانونية الحالية. حاول مجددًا.",
            )}
          </p>
          <Button
            variant="ghost"
            onClick={() => setAttempt((value) => value + 1)}
          >
            {c("重试", "Retry", "إعادة المحاولة")}
          </Button>
        </div>
      ) : !document ? (
        <p role="status">{c("正在加载…", "Loading…", "جارٍ التحميل…")}</p>
      ) : (
        <>
          <p>
            {c("版本", "Version", "الإصدار")}: <bdi>{document.version}</bdi>
          </p>
          <div
            lang={document.language}
            dir={document.language === "ar" ? "rtl" : "ltr"}
            style={{
              whiteSpace: "pre-wrap",
              overflowWrap: "anywhere",
              lineHeight: 1.75,
            }}
          >
            {document.content}
          </div>
        </>
      )}
    </article>
  );
}
