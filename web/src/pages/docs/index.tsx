import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  KeyRound,
  Terminal,
  Cpu,
  ShieldCheck,
  Coins,
  Copy,
  Sparkles,
  ArrowUpRight,
  BookOpen,
  ChevronRight,
} from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { copyText } from "../../features/code-example";
import { usePreferences } from "../../lib/preferences";

type ArticleId =
  "quickstart" | "auth" | "tiers" | "claudeCode" | "errors" | "billing";
const curlExample = `curl https://api.veyra.example/v1/chat/completions \\
  -H "Authorization: Bearer $VEYRA_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4.1",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'`;
const pythonExample = `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url="https://api.veyra.example/v1"
)

response = client.chat.completions.create(
    model="gpt-4.1",
    messages=[{"role": "user", "content": "Hello!"}],
    stream=True
)

for chunk in response:
    print(chunk.choices[0].delta.content or "", end="")`;

export default function DocsPage() {
  const { t } = useTranslation();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const [activeTab, setActiveTab] = useState<ArticleId>("quickstart");
  const articles: { id: ArticleId; title: string; Icon: typeof Terminal }[] = [
    { id: "quickstart", title: t("docsQuickstart"), Icon: Terminal },
    { id: "auth", title: t("docsAuth"), Icon: KeyRound },
    { id: "tiers", title: t("docsTiers"), Icon: Cpu },
    { id: "claudeCode", title: t("docsClaudeCode"), Icon: Sparkles },
    { id: "errors", title: t("docsErrors"), Icon: ShieldCheck },
    { id: "billing", title: t("docsBilling"), Icon: Coins },
  ];
  const current = articles.find((article) => article.id === activeTab)!;
  const CodeBlock = ({ title, code }: { title: string; code: string }) => (
    <div className="docs-code">
      <div>
        <span>{title}</span>
        <button
          className="button button-small button-ghost"
          onClick={() => copyText(code, t("copied"), t("copyFailed"))}
        >
          <Copy size={14} />
          {t("copy")}
        </button>
      </div>
      <pre dir="ltr">
        <code>{code}</code>
      </pre>
    </div>
  );

  return (
    <div className="brand-home public-refined docs-refined">
      <PublicHeader />
      <div className="docs-layout">
        <aside className="docs-sidebar">
          <div className="docs-sidebar-title">
            <BookOpen size={17} />
            {t("docsStationTitle")}
          </div>
          <nav aria-label={t("navDocs")}>
            {articles.map(({ id, title, Icon }) => (
              <button
                key={id}
                className={activeTab === id ? "is-active" : ""}
                onClick={() => setActiveTab(id)}
                aria-current={activeTab === id ? "page" : undefined}
              >
                <Icon size={17} />
                <span>{title}</span>
                {activeTab === id && <ChevronRight size={14} />}
              </button>
            ))}
          </nav>
          <div className="docs-sidebar-note">
            <Sparkles size={20} />
            <strong>
              {text(
                "准备好开始了吗？",
                "Ready to build?",
                "هل أنت مستعد للبناء؟",
              )}
            </strong>
            <p>
              {text(
                "在控制台创建密钥，发起你的第一次调用。",
                "Create a key and make your first request.",
                "أنشئ مفتاحًا وأرسل طلبك الأول.",
              )}
            </p>
            <Link to="/console/keys">
              {t("pConsole")}
              <ArrowUpRight size={14} />
            </Link>
          </div>
        </aside>
        <main className="docs-article">
          <div className="docs-breadcrumb">
            {t("navDocs")}
            <ChevronRight size={13} />
            <span>{current.title}</span>
          </div>
          <h1>{current.title}</h1>
          {activeTab === "quickstart" && (
            <>
              <p className="docs-lead">
                {text(
                  "从一个请求开始。配置接口地址与密钥，即可使用统一的调用方式连接模型。",
                  "Start with a single request. Configure your endpoint and key to connect models through a consistent interface.",
                  "ابدأ بطلب واحد. اضبط عنوان الواجهة ومفتاحك للاتصال بالنماذج.",
                )}
              </p>
              <div className="docs-callout">
                <BookOpen size={18} />
                <span>
                  {text(
                    "本文使用演示域名。实际接入时，请替换为正式接口地址和你自己的密钥。",
                    "Examples use a demo domain. Replace it with the production endpoint and your own key.",
                    "تستخدم الأمثلة نطاقًا تجريبيًا. استبدله بعنوان الخدمة ومفتاحك.",
                  )}
                </span>
              </div>
              <h2>
                <span className="docs-step">1</span>
                {text(
                  "发送第一个请求",
                  "Send your first request",
                  "أرسل طلبك الأول",
                )}
              </h2>
              <p>
                {text(
                  "在终端中设置密钥环境变量，然后发送一个对话请求。",
                  "Set your key as an environment variable, then send a chat request from your terminal.",
                  "اضبط مفتاحك كمتغير بيئة ثم أرسل طلب محادثة.",
                )}
              </p>
              <CodeBlock
                title={text("cURL（命令行请求）", "cURL", "cURL")}
                code={curlExample}
              />
              <h2>
                <span className="docs-step">2</span>
                {text(
                  "接入你的应用",
                  "Connect your application",
                  "اربط تطبيقك",
                )}
              </h2>
              <p>
                {text(
                  "下方示例展示了流式响应的基本用法。",
                  "The example below shows a basic streaming response.",
                  "يوضح المثال التالي استجابة متدفقة أساسية.",
                )}
              </p>
              <CodeBlock
                title={text("Python（编程语言）", "Python", "Python")}
                code={pythonExample}
              />
            </>
          )}
          {activeTab === "auth" && (
            <>
              <p className="docs-lead">
                {text(
                  "在一个地方管理密钥、访问范围与用量配额。",
                  "Manage keys, access boundaries, and usage quotas in one place.",
                  "أدر المفاتيح وحدود الوصول وحصص الاستخدام في مكان واحد.",
                )}
              </p>
              <div className="docs-info-grid">
                {[
                  [
                    text(
                      "妥善保存密钥",
                      "Store keys securely",
                      "احفظ المفاتيح بأمان",
                    ),
                    text(
                      "密钥创建后请及时保存，并通过环境变量加载。不要将密钥提交到代码仓库。",
                      "Save your key after creation and load it through environment variables. Keep it out of source control.",
                      "احفظ المفتاح واستخدم متغيرات البيئة. لا تضفه إلى مستودع الشفرة.",
                    ),
                  ],
                  [
                    text(
                      "按项目设置配额",
                      "Set project quotas",
                      "حدد حصص المشاريع",
                    ),
                    text(
                      "账户余额与单个密钥配额分别计算；请根据项目需要设置日限额和月限额。",
                      "Account balance and individual key quotas are evaluated separately. Set daily and monthly limits for each project.",
                      "يتم حساب رصيد الحساب وحصص المفاتيح بشكل منفصل.",
                    ),
                  ],
                  [
                    text("限制访问来源", "Restrict access", "تقييد الوصول"),
                    text(
                      "通过来源地址白名单限制密钥的访问范围，并定期检查调用记录。",
                      "Use an address allowlist to restrict key access and review request logs regularly.",
                      "استخدم قائمة العناوين المسموح بها وراجع سجلات الطلبات.",
                    ),
                  ],
                ].map(([title, description]) => (
                  <section key={title}>
                    <ShieldCheck size={20} />
                    <h2>{title}</h2>
                    <p>{description}</p>
                  </section>
                ))}
              </div>
            </>
          )}
          {activeTab === "claudeCode" && (
            <>
              <p className="docs-lead">
                {text(
                  "为 Claude Code（编程助手）配置兼容的服务地址和密钥。",
                  "Configure a compatible endpoint and key for Claude Code.",
                  "اضبط عنوان خدمة متوافقًا ومفتاحًا لأداة Claude Code.",
                )}
              </p>
              <div className="docs-callout">
                <BookOpen size={18} />
                <span>
                  {text(
                    "以下是演示配置，使用前请核对正式服务的兼容性与地址。",
                    "This is a demo configuration. Verify production compatibility and endpoints before use.",
                    "هذا إعداد تجريبي. تحقق من توافق الخدمة وعنوانها قبل الاستخدام.",
                  )}
                </span>
              </div>
              <h2>
                {text("环境变量", "Environment variables", "متغيرات البيئة")}
              </h2>
              <CodeBlock
                title={text(
                  "终端配置",
                  "Terminal configuration",
                  "إعداد الطرفية",
                )}
                code={
                  'export ANTHROPIC_BASE_URL="https://api.veyra.example/v1"\nexport ANTHROPIC_API_KEY="YOUR_API_KEY"\n\nclaude'
                }
              />
              <Link className="docs-next-link" to="/console/keys">
                {t("createKeyWithModel")}
                <ArrowUpRight size={15} />
              </Link>
            </>
          )}
          {activeTab === "tiers" && (
            <>
              <p className="docs-lead">
                {text(
                  "按场景选择服务等级，平衡价格与服务目标。以下为演示配置，并非实测结果。",
                  "Choose a service level for your workload. The following demo configuration describes targets, not measured results.",
                  "اختر مستوى الخدمة المناسب. القيم التالية أهداف تجريبية وليست نتائج مقاسة.",
                )}
              </p>
              <div className="docs-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{t("tier")}</th>
                      <th>
                        {text("价格系数", "Price multiplier", "معامل السعر")}
                      </th>
                      <th>{t("target")}</th>
                      <th>{t("latency")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[
                      ["Ultra", "1.00×", "99.5%", "800 ms"],
                      ["Pro", "0.80×", "99.0%", "1500 ms"],
                      ["Standard", "0.60×", "97.5%", "3000 ms"],
                      ["Lite", "0.40×", "97.5%", "3000 ms"],
                    ].map((row) => (
                      <tr key={row[0]}>
                        {row.map((value, i) => (
                          <td key={i}>{value}</td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <Link className="docs-next-link" to="/models">
                {t("navModels")}
                <ArrowUpRight size={15} />
              </Link>
            </>
          )}
          {activeTab === "errors" && (
            <>
              <p className="docs-lead">
                {text(
                  "根据返回状态定位问题，再选择合适的恢复方式。",
                  "Use the response status to identify issues and choose a recovery path.",
                  "استخدم حالة الاستجابة لتحديد المشكلة وطريقة حلها.",
                )}
              </p>
              <div className="docs-errors">
                {[
                  [
                    "403",
                    "key_quota_exceeded",
                    text(
                      "密钥配额已用尽，请调整日限额或月限额，或等待下个计费周期。",
                      "The key has reached its quota. Adjust its limit or wait for the next cycle.",
                      "وصل المفتاح إلى حصته. عدّل الحد أو انتظر الدورة التالية.",
                    ),
                  ],
                  [
                    "402",
                    "account_balance_depleted",
                    text(
                      "账户可用余额不足，请在控制台完成充值后重试。",
                      "The account balance is insufficient. Top up in the console and retry.",
                      "رصيد الحساب غير كافٍ. أضف رصيدًا وأعد المحاولة.",
                    ),
                  ],
                  [
                    "503",
                    "upstream_unavailable",
                    text(
                      "上游服务暂时不可用。请稍后重试，并检查服务状态页。",
                      "The upstream service is temporarily unavailable. Retry later and check the status page.",
                      "الخدمة غير متوفرة مؤقتًا. أعد المحاولة لاحقًا وراجع صفحة الحالة.",
                    ),
                  ],
                ].map(([status, code, description]) => (
                  <section key={status}>
                    <span>{status}</span>
                    <div>
                      <code>{code}</code>
                      <p>{description}</p>
                    </div>
                  </section>
                ))}
              </div>
            </>
          )}
          {activeTab === "billing" && (
            <>
              <p className="docs-lead">
                {text(
                  "账户余额、报表与调用明细使用不同的显示精度，便于阅读和核对。",
                  "Balances, reports, and request details use different display precision for clarity and reconciliation.",
                  "تستخدم الأرصدة والتقارير وتفاصيل الطلبات درجات مختلفة من الدقة.",
                )}
              </p>
              <div className="docs-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{text("位置", "View", "العرض")}</th>
                      <th>{text("显示精度", "Precision", "الدقة")}</th>
                      <th>{text("示例", "Example", "مثال")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[
                      [
                        text("账户余额", "Account balance", "رصيد الحساب"),
                        "2",
                        "$128.40",
                      ],
                      [
                        text("用量报表", "Usage totals", "إجمالي الاستخدام"),
                        "4",
                        "$14.2850",
                      ],
                      [
                        text("单次调用", "Request logs", "سجلات الطلبات"),
                        "6",
                        "$0.001850",
                      ],
                      [
                        text("导出明细", "Export details", "تفاصيل التصدير"),
                        "9",
                        "$0.000000296",
                      ],
                    ].map((row) => (
                      <tr key={row[0]}>
                        {row.map((value, i) => (
                          <td key={i}>{value}</td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
          <footer className="docs-article-footer">
            {text(
              "Veyra开发者文档 · 产品演示",
              "Veyra developer documentation · Product preview",
              "وثائق مطوري Veyra · معاينة المنتج",
            )}
          </footer>
        </main>
      </div>
    </div>
  );
}
