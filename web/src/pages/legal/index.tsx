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
    title: ["隐私政策", "Privacy Policy", "سياسة الخصوصية"],
    updated: "2026-10-23",
  },
  terms: {
    title: ["服务条款", "Terms of Service", "شروط الخدمة"],
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
          {t("返回首页", "Back to home", "العودة")}
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
            {t("最后更新", "Last updated", "آخر تحديث")}: {info.updated}
          </p>

          {key === "privacy" ? (
            <>
              <h2>{t("概述", "Overview", "نظرة عامة")}</h2>
              <p>
                {t(
                  "本页面为产品演示的隐私政策模板，不构成真实法律承诺。实际部署时请替换为经法律审核的正式文本。",
                  "This page is a template for the product demo and does not constitute a real legal commitment. Replace with legally reviewed content before production deployment.",
                  "هذه الصفحة نموذج للعرض التوضيحي ولا تشكل التزامًا قانونيًا حقيقيًا. استبدلها بنص مراجَع قانونيًا قبل النشر الفعلي.",
                )}
              </p>
              <h2>
                {t(
                  "收集的信息",
                  "Information we collect",
                  "المعلومات التي نجمعها",
                )}
              </h2>
              <ul>
                <li>
                  {t(
                    "账户信息：用户名、邮箱",
                    "Account information: username, email address",
                    "معلومات الحساب: اسم المستخدم والبريد الإلكتروني",
                  )}
                </li>
                <li>
                  {t(
                    "API 密钥（仅存储哈希值）",
                    "API keys (only hashes are stored)",
                    "مفاتيح API (تُخزن التجزئة فقط)",
                  )}
                </li>
                <li>
                  {t(
                    "调用日志：模型、Token 数、时长、费用",
                    "Request logs: model, token counts, duration, cost",
                    "سجلات الطلبات: النموذج وعدد الرموز والمدة والتكلفة",
                  )}
                </li>
              </ul>
              <h2>{t("数据存储", "Data storage", "تخزين البيانات")}</h2>
              <p>
                {t(
                  "所有数据存储在本地 SQLite 数据库中。本演示不将数据传输给任何第三方。",
                  "All data is stored in a local SQLite database. This demo does not transmit data to any third party.",
                  "تُخزن جميع البيانات في قاعدة بيانات SQLite المحلية. لا يرسل هذا العرض البيانات إلى أي طرف ثالث.",
                )}
              </p>
              <h2>{t("联系方式", "Contact", "التواصل")}</h2>
              <p>
                {t(
                  "如有疑问，请通过项目仓库提交 Issue。",
                  "For questions, please submit an issue via the project repository.",
                  "للاستفسارات، يرجى تقديم مشكلة عبر مستودع المشروع.",
                )}
              </p>
            </>
          ) : (
            <>
              <h2>{t("概述", "Overview", "نظرة عامة")}</h2>
              <p>
                {t(
                  "本页面为产品演示的服务条款模板，不构成真实法律承诺。实际部署时请替换为经法律审核的正式文本。",
                  "This page is a template for the product demo and does not constitute a real legal commitment. Replace with legally reviewed content before production deployment.",
                  "هذه الصفحة نموذج للعرض التوضيحي ولا تشكل التزامًا قانونيًا حقيقيًا. استبدلها بنص مراجَع قانونيًا قبل النشر الفعلي.",
                )}
              </p>
              <h2>{t("服务范围", "Scope of service", "نطاق الخدمة")}</h2>
              <p>
                {t(
                  "本平台提供多模型 API 网关服务，将多个上游模型渠道聚合为统一接口。演示环境中的调用为模拟，不产生真实费用。",
                  "This platform provides a multi-model API gateway service that aggregates upstream model channels into a unified interface. Requests in the demo environment are simulated and do not incur real charges.",
                  "توفر هذه المنصة خدمة بوابة API متعددة النماذج تجمع قنوات النماذج في واجهة موحدة. الطلبات في بيئة العرض محاكاة ولا تنتج رسومًا حقيقية.",
                )}
              </p>
              <h2>{t("计费", "Billing", "الفوترة")}</h2>
              <p>
                {t(
                  "平台按实际 Token 用量或计量单位计费。失败请求不扣费。充值金额在充值成功后即时到账。",
                  "The platform bills by actual token usage or metered units. Failed requests are not charged. Top-up amounts are credited immediately upon successful payment.",
                  "تحاسب المنصة على استخدام الرموز الفعلي أو الوحدات المقاسة. الطلبات الفاشلة مجانية. تُضاف مبالغ الشحن فور نجاح الدفع.",
                )}
              </p>
              <h2>
                {t("责任限制", "Limitation of liability", "حد المسؤولية")}
              </h2>
              <p>
                {t(
                  "演示环境不提供任何 SLA 承诺或赔付。生产环境的责任条款以正式合同为准。",
                  "The demo environment provides no SLA commitment or compensation. Production liability terms are governed by the formal contract.",
                  "لا توفر بيئة العرض أي التزام SLA أو تعويضًا. تحكم شروط المسؤولية للإنتاج بالعقد الرسمي.",
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
              "演示文档 · 不构成法律承诺",
              "Demo document · not a legal commitment",
              "مستند تجريبي · ليس التزامًا قانونيًا",
            )}
          </footer>
        </article>
      </main>
    </div>
  );
}
