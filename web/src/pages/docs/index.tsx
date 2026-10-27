import { useState, useMemo, useEffect } from "react";
import { Link, useSearchParams } from "react-router-dom";
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
  Search,
} from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { copyText } from "../../features/code-example";
import { usePreferences } from "../../lib/preferences";

type ArticleId =
  "quickstart" | "auth" | "tiers" | "claudeCode" | "errors" | "billing" | "protocols" | "routing" | "limits" | "faq";
const curlExample = `curl https://api.myapis.ai/v1/chat/completions \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4.1",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'`;
  const pythonExample = `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url="https://api.myapis.ai/v1"
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
  const [searchParams, setSearchParams] = useSearchParams();
  const articles: { id: ArticleId; title: string; Icon: typeof Terminal }[] = [
    { id: "quickstart", title: t("docsQuickstart"), Icon: Terminal },
    { id: "auth", title: t("docsAuth"), Icon: KeyRound },
    { id: "tiers", title: t("docsTiers"), Icon: Cpu },
    { id: "protocols", title: text("协议", "Protocols", "البروتوكولات"), Icon: ArrowUpRight },
    { id: "claudeCode", title: t("docsClaudeCode"), Icon: Sparkles },
    { id: "errors", title: t("docsErrors"), Icon: ShieldCheck },
    { id: "billing", title: t("docsBilling"), Icon: Coins },
    { id: "routing", title: text("路由与权限", "Routing & authz", "التوجيه والصلاحيات"), Icon: ArrowUpRight },
    { id: "limits", title: text("配额与限流", "Limits & rate", "الحدود والمعدل"), Icon: BookOpen },
    { id: "faq", title: text("常见问题", "FAQ", "الأسئلة الشائعة"), Icon: ChevronRight },
  ];
  const validIds = articles.map((article) => article.id);
  const urlArticle = searchParams.get("article");
  const initialArticle: ArticleId =
    urlArticle && validIds.includes(urlArticle as ArticleId)
      ? (urlArticle as ArticleId)
      : "quickstart";
  const [activeTab, setActiveTab] = useState<ArticleId>(initialArticle);
  const [searchQuery, setSearchQuery] = useState("");
  const current = articles.find((article) => article.id === activeTab)!;

  // Article search: filter articles by title, keywords, and content tags.
  const articleKeywords: Record<ArticleId, string[]> = {
    quickstart: ["curl", "python", "node", "api key", "base url", "getting started", "快速开始", "入门", "创建密钥"],
    auth: ["authentication", "authorization", "bearer", "token", "api key", "认证", "授权", "密钥", "bearer"],
    tiers: ["service tier", "group", "ultra", "pro", "standard", "lite", "分组", "等级", "服务"],
    claudeCode: ["claude", "anthropic", "codex", "cursor", "gemini", "工具集成", "claude code", "编程工具"],
    errors: ["error", "400", "401", "403", "429", "502", "503", "错误", "错误码", "故障排除"],
    billing: ["billing", "quota", "cost", "charge", "balance", "top-up", "计费", "费用", "余额", "充值"],
    protocols: ["protocol", "endpoint", "api", "curl", "url", "path", "协议", "端点", "路径", "接口"],
    routing: ["routing", "authorization", "group", "model", "allowlist", "auto", "路由", "授权", "分组", "白名单"],
    limits: ["limit", "rate", "quota", "concurrency", "throttle", "配额", "限流", "并发", "额度"],
    faq: ["faq", "question", "answer", "常见问题", "问题", "解答"],
  };
  const filteredArticles = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return articles;
    return articles.filter(({ id, title }) => {
      const keywords = articleKeywords[id] || [];
      return (
        title.toLowerCase().includes(q) ||
        keywords.some((kw) => kw.toLowerCase().includes(q) || q.includes(kw.toLowerCase()))
      );
    });
  }, [searchQuery, lang]);

  // When search matches exactly one article, auto-select it.
  useEffect(() => {
    if (searchQuery.trim() && filteredArticles.length === 1) {
      setActiveTab(filteredArticles[0].id);
    }
  }, [searchQuery, filteredArticles]);

  // Sync active article to the URL so links are shareable and refresh-safe.
  useEffect(() => {
    const current = searchParams.get("article");
    if (current === activeTab) return;
    const next = new URLSearchParams(searchParams);
    if (activeTab === "quickstart") next.delete("article");
    else next.set("article", activeTab);
    setSearchParams(next, { replace: true });
  }, [activeTab]);

  // React to browser back/forward navigation.
  useEffect(() => {
    const urlArticle = searchParams.get("article");
    const target: ArticleId =
      urlArticle && validIds.includes(urlArticle as ArticleId)
        ? (urlArticle as ArticleId)
        : "quickstart";
    if (target !== activeTab) setActiveTab(target);
  }, [searchParams]);
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
            <div className="docs-search-box" style={{ marginBottom: 8 }}>
              <Search size={14} style={{ opacity: 0.5, position: "absolute", marginInlineStart: 8, marginTop: 7 }} />
              <input
                type="search"
                placeholder={text("搜索文档…", "Search docs…", "ابحث في المستندات…")}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                style={{
                  width: "100%",
                  padding: "6px 10px 6px 28px",
                  fontSize: "var(--text-xs)",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-sm)",
                  background: "var(--surface)",
                  color: "var(--text)",
                  boxSizing: "border-box",
                }}
              />
            </div>
            {filteredArticles.map(({ id, title, Icon }) => (
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
            {filteredArticles.length === 0 && (
              <p style={{ padding: "8px 4px", fontSize: "var(--text-xs)", color: "var(--muted)" }}>
                {text("未找到匹配的文档", "No matching docs", "لا مستندات مطابقة")}
              </p>
            )}
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
                  'export ANTHROPIC_BASE_URL="https://api.myapis.ai/v1"\nexport ANTHROPIC_API_KEY="YOUR_API_KEY"\n\nclaude'
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
                    "401",
                    "invalid_api_key",
                    text(
                      "API 密钥无效或不存在。检查 Authorization 请求头中的 Bearer token 是否正确。",
                      "The API key is invalid or not found. Check the Bearer token in the Authorization header.",
                      "مفتاح API غير صالح أو غير موجود. تحقق من رمز Bearer في ترويسة Authorization.",
                    ),
                  ],
                  [
                    "401",
                    "token_disabled",
                    text(
                      "API 密钥已被禁用。在控制台的密钥管理中启用该密钥，或创建新密钥。",
                      "The API key has been disabled. Enable it in key management, or create a new key.",
                      "تم تعطيل مفتاح API. فعّله في إدارة المفاتيح أو أنشئ مفتاحًا جديدًا.",
                    ),
                  ],
                  [
                    "401",
                    "token_expired",
                    text(
                      "API 密钥已过期。创建新密钥或延长原密钥的有效期。",
                      "The API key has expired. Create a new key or extend its validity.",
                      "انتهت صلاحية مفتاح API. أنشئ مفتاحًا جديدًا أو مدّد صلاحيته.",
                    ),
                  ],
                  [
                    "403",
                    "insufficient_quota",
                    text(
                      "密钥或账户额度不足以完成本次调用。失败请求不扣费。请充值或调整密钥限额后重试。",
                      "The key or account quota is insufficient for this request. Failed requests are not charged. Top up or adjust the key limit, then retry.",
                      "حصة المفتاح أو الحساب غير كافية لهذا الطلب. الطلبات الفاشلة مجانية. أضف رصيدًا أو عدّل حد المفتاح ثم أعد المحاولة.",
                    ),
                  ],
                  [
                    "403",
                    "token_quota_exhausted",
                    text(
                      "密钥配额已用尽。调整密钥的日/月限额，或等待下个计费周期。",
                      "The key has reached its quota. Adjust its daily or monthly limit, or wait for the next cycle.",
                      "وصل المفتاح إلى حصته. عدّل الحد اليومي أو الشهري أو انتظر الدورة التالية.",
                    ),
                  ],
                  [
                    "403",
                    "auto_group_empty",
                    text(
                      "密钥的自动分组中没有可用模型。检查密钥的分组配置。",
                      "The key's auto group has no available models. Check the key's group configuration.",
                      "لا توجد نماذج متاحة في المجموعة التلقائية للمفتاح. تحقق من إعدادات المجموعة.",
                    ),
                  ],
                  [
                    "403",
                    "group_not_available",
                    text(
                      "该密钥未授权所请求的服务分组。检查密钥的分组权限设置。",
                      "The key is not authorized for the requested service group. Check the key's group permissions.",
                      "المفتاح غير مصرح له بالمجموعة المطلوبة. تحقق من أذونات المجموعة.",
                    ),
                  ],
                  [
                    "400",
                    "invalid_request_body",
                    text(
                      "请求体格式错误。检查 JSON / multipart 格式是否正确，以及必填字段是否缺失。",
                      "The request body is invalid. Check the JSON or multipart format and required fields.",
                      "نص الطلب غير صالح. تحقق من تنسيق JSON أو multipart والحقول المطلوبة.",
                    ),
                  ],
                  [
                    "400",
                    "invalid_json",
                    text(
                      "JSON 解析失败。确保请求体是有效的 JSON 文本。",
                      "JSON parsing failed. Ensure the body is valid JSON.",
                      "فشل تحليل JSON. تأكد من أن النص JSON صالح.",
                    ),
                  ],
                  [
                    "400",
                    "invalid_multipart",
                    text(
                      "multipart 表单解析失败。检查文件上传的表单格式。",
                      "Multipart form parsing failed. Check the file upload form format.",
                      "فشل تحليل نموذج multipart. تحقق من تنسيق نموذج رفع الملفات.",
                    ),
                  ],
                  [
                    "400",
                    "model_required",
                    text(
                      "缺少 model 字段。在请求中指定要调用的模型标识。",
                      "The model field is missing. Specify the model to call in the request.",
                      "حقل model مفقود. حدد النموذج المطلوب في الطلب.",
                    ),
                  ],
                  [
                    "429",
                    "rate_limit_exceeded",
                    text(
                      "请求频率超出限制。降低并发或等待窗口重置后重试。",
                      "Rate limit exceeded. Reduce concurrency or wait for the window to reset.",
                      "تم تجاوز حد المعدل. قلل التزامن أو انتظر إعادة تعيين النافذة.",
                    ),
                  ],
                  [
                    "502",
                    "upstream_unavailable",
                    text(
                      "上游服务暂时不可用。系统会自动重试其他渠道；全失败时退款。请稍后重试。",
                      "The upstream service is temporarily unavailable. The system auto-retries other channels; full failure refunds. Retry later.",
                      "الخدمة غير متوفرة مؤقتًا. يعيد النظام المحاولة تلقائيًا عبر قنوات أخرى؛ الرفض الكامل يسترد الرسوم. أعد المحاولة لاحقًا.",
                    ),
                  ],
                  [
                    "502",
                    "retry_budget_exhausted",
                    text(
                      "重试预算耗尽，所有渠道均失败。预扣额度已全额退还。请检查渠道配置或稍后重试。",
                      "Retry budget exhausted; all channels failed. The pre-charged quota has been fully refunded. Check channel configuration or retry later.",
                      "استنفدت ميزانية المحاولة؛ فشلت جميع القنوات. تم استرداد الحجز بالكامل. تحقق من إعدادات القناة أو أعد المحاولة لاحقًا.",
                    ),
                  ],
                  [
                    "502",
                    "upstream_result_uncertain",
                    text(
                      "上游结果不确定（请求可能已执行但结果未知）。为避免重复生成，不自动重试。请检查调用明细确认实际状态。",
                      "The upstream result is uncertain (the request may have executed but the result is unknown). No auto-retry to avoid duplicate generation. Check the request log for actual status.",
                      "نتيجة المنبع غير مؤكدة (قد يكون الطلب نُفذ دون معرفة النتيجة). لا إعادة محاولة تلقائية لتفادي التكرار. راجع سجل الطلبات للتأكد.",
                    ),
                  ],
                  [
                    "503",
                    "no_available_channel",
                    text(
                      "当前没有可用渠道为该模型提供服务。可能是渠道被禁用或无匹配分组。请检查渠道和服务分组配置。",
                      "No available channel serves this model. The channel may be disabled or no group matches. Check channel and service group configuration.",
                      "لا توجد قناة متاحة لهذا النموذج. قد تكون معطلة أو لا تطابق المجموعة. تحقق من إعدادات القناة والمجموعة.",
                    ),
                  ],
                  [
                    "500",
                    "pre_consume_failed",
                    text(
                      "预扣额度时发生内部错误。请求未执行，不会产生费用。请稍后重试。",
                      "An internal error occurred during the quota pre-charge. The request was not executed and no charges apply. Retry later.",
                      "حدث خطأ داخلي أثناء حجز الحصة. لم يُنفذ الطلب ولا رسوم. أعد المحاولة لاحقًا.",
                    ),
                  ],
                  [
                    "500",
                    "channel_selection_failed",
                    text(
                      "渠道选择时发生内部错误。请稍后重试或联系管理员。",
                      "An internal error occurred during channel selection. Retry later or contact the administrator.",
                      "حدث خطأ داخلي أثناء اختيار القناة. أعد المحاولة لاحقًا أو اتصل بالمسؤول.",
                    ),
                  ],
                ].map(([status, code, description]) => (
                  <section key={code}>
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
          {activeTab === "protocols" && (
            <>
              <p className="docs-lead">
                {text(
                  "平台支持多种 API 协议，下方表格列出各协议的端点地址与调用路径。",
                  "The platform supports multiple API protocols. The table below lists each protocol's base URL and endpoint path.",
                  "تدعم المنصة عدة بروتوكولات API. يسرد الجدول أدناه عنوان الأساس ومسار كل بروتوكول.",
                )}
              </p>
              <div className="docs-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{text("协议", "Protocol", "البروتوكول")}</th>
                      <th>Base URL</th>
                      <th>{text("路径", "Path", "المسار")}</th>
                      <th>{text("认证方式", "Auth", "المصادقة")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[
                      [text("Chat Completions（OpenAI 兼容）", "Chat Completions (OpenAI)", "Chat Completions (OpenAI)"), "https://api.myapis.ai/v1", "/chat/completions", "Bearer token"],
                      [text("Messages（Anthropic 兼容）", "Messages (Anthropic)", "Messages (Anthropic)"), "https://api.myapis.ai/v1", "/messages", "Bearer token"],
                      [text("Embeddings（OpenAI 兼容）", "Embeddings (OpenAI)", "Embeddings (OpenAI)"), "https://api.myapis.ai/v1", "/embeddings", "Bearer token"],
                      [text("Images（OpenAI 兼容）", "Images (OpenAI)", "Images (OpenAI)"), "https://api.myapis.ai/v1", "/images/generations", "Bearer token"],
                      [text("Audio TTS（OpenAI 兼容）", "Audio TTS (OpenAI)", "Audio TTS (OpenAI)"), "https://api.myapis.ai/v1", "/audio/speech", "Bearer token"],
                      [text("Audio STT（OpenAI 兼容）", "Audio STT (OpenAI)", "Audio STT (OpenAI)"), "https://api.myapis.ai/v1", "/audio/transcriptions", "Bearer token"],
                      [text("Video（任务提交+轮询）", "Video (submit+poll)", "Video (submit+poll)"), "https://api.myapis.ai/v1", "/video/generations", "Bearer token"],
                      [text("Realtime（WebSocket）", "Realtime (WebSocket)", "Realtime (WebSocket)"), "https://api.myapis.ai/v1", "/realtime", "Bearer token"],
                      [text("Models（模型列表）", "Models (list)", "Models (list)"), "https://api.myapis.ai/v1", "/models", "Bearer token"],
                    ].map(([protocol, baseUrl, path, auth], i) => (
                      <tr key={i}>
                        <td>{protocol}</td>
                        <td><code className="mono">{baseUrl}</code></td>
                        <td><code className="mono">{path}</code></td>
                        <td>{auth}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="docs-callout">
                <p>
                  {text(
                    "所有协议使用统一的 Bearer token 认证（API 密钥）。密钥在控制台的密钥管理中创建，具有授权模型范围限制。",
                    "All protocols use unified Bearer token authentication (API key). Keys are created in the console's key management with authorized model scope limits.",
                    "تستخدم جميع البروتوكولات مصادقة Bearer token الموحدة (مفتاح API). تُنشأ المفاتيح في إدارة المفاتيح بالوحدة التحكم مع حدود النماذج المصروح بها.",
                  )}
                </p>
              </div>
            </>
          )}
          {activeTab === "routing" && (
            <>
              <p className="docs-lead">
                {text(
                  "模型授权和路由选择在密钥创建时配置。每个密钥绑定服务分组和模型范围。",
                  "Model authorization and routing are configured at key creation. Each key binds to a service group and model scope.",
                  "يتم تكوين تفويض النموذج والتوجيه عند إنشاء المفتاح. يرتبط كل مفتاح بمجموعة خدمة ونطاق نموذج.",
                )}
              </p>
              <div className="docs-callout">
                <p>
                  {text(
                    "路由流程：密钥认证 → 用户额度检查 → 密钥额度检查 → 渠道选择（按分组+模型匹配）→ 预扣额度 → 上游调用 → 结算或退款。",
                    "Routing flow: key auth → user quota check → key quota check → channel selection (group + model match) → pre-charge → upstream call → settle or refund.",
                    "تدفق التوجيه: مصادقة المفتاح → فحص حصة المستخدم → فحص حصة المفتاح → اختيار القناة (مطابقة المجموعة + النموذج) → الخصم المسبق → استدعاء المنبع → التسوية أو الاسترداد.",
                  )}
                </p>
              </div>
              <h2>{text("密钥授权范围", "Key authorization scope", "نطاق تفويض المفتاح")}</h2>
              <table>
                <thead>
                  <tr>
                    <th>{text("字段", "Field", "الحقل")}</th>
                    <th>{text("说明", "Description", "الوصف")}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr><td><code>group</code></td><td>{text("密钥所属服务分组，决定可用的模型和倍率", "Service group, determines available models and multipliers", "مجموعة الخدمة، تحدد النماذج والمضاعفات المتاحة")}</td></tr>
                  <tr><td><code>model_limit</code></td><td>{text("授权模型列表，逗号分隔；为空表示使用分组内全部模型", "Authorized model list, comma-separated; empty = all models in group", "قائمة النماذج المصرّح بها، مفصولة بفواصل؛ فارغ = كل النماذج في المجموعة")}</td></tr>
                  <tr><td><code>allowed_ips</code></td><td>{text("IP 白名单，CIDR 格式；为空表示不限制", "IP allowlist, CIDR format; empty = unrestricted", "قائمة IP المسموحة، تنسيق CIDR؛ فارغ = غير مقيد")}</td></tr>
                  <tr><td><code>remain_quota</code></td><td>{text("密钥剩余额度（quota 单位）；0 表示无限额", "Remaining key quota (quota units); 0 = unlimited", "الحصة المتبقية للمفتاح (وحدات الحصة)؛ 0 = غير محدود")}</td></tr>
                </tbody>
              </table>
            </>
          )}
          {activeTab === "limits" && (
            <>
              <p className="docs-lead">
                {text(
                  "平台对账户和密钥设有配额与速率限制，防止滥用并保障公平使用。",
                  "The platform enforces quota and rate limits on accounts and keys to prevent abuse and ensure fair usage.",
                  "تفرض المنصة حدود الحصة والمعدل على الحسابات والمفاتيح لمنع الإساءة وضمان الاستخدام العادل.",
                )}
              </p>
              <div className="docs-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{text("限制类型", "Limit type", "نوع الحد")}</th>
                      <th>{text("默认值", "Default", "الافتراضي")}</th>
                      <th>{text("说明", "Description", "الوصف")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr><td>{text("用户速率限制", "User rate limit", "حد معدل المستخدم")}</td><td className="mono">100 req/min</td><td>{text("每用户每分钟最大请求数", "Max requests per user per minute", "الحد الأقصى للطلبات لكل مستخدم في الدقيقة")}</td></tr>
                    <tr><td>{text("密钥速率限制", "Key rate limit", "حد معدل المفتاح")}</td><td className="mono">50 req/min</td><td>{text("每密钥每分钟最大请求数", "Max requests per key per minute", "الحد الأقصى للطلبات لكل مفتاح في الدقيقة")}</td></tr>
                    <tr><td>{text("模型速率限制", "Model rate limit", "حد معدل النموذج")}</td><td className="mono">500 req/min</td><td>{text("每模型每分钟最大请求数", "Max requests per model per minute", "الحد الأقصى للطلبات لكل نموذج في الدقيقة")}</td></tr>
                    <tr><td>{text("单次预扣上限", "Per-request cap", "حد الخصم المسبق")}</td><td className="mono">2,000,000 quota</td><td>{text("单次请求预扣额度上限，防止恶意大请求", "Per-request pre-charge cap, prevents malicious large requests", "حد الخصم المسبق لكل طلب، يمنع الطلبات الكبيرة الخبيثة")}</td></tr>
                  </tbody>
                </table>
              </div>
              <div className="docs-callout">
                <p>{text("超出速率限制返回 429（rate_limit_exceeded）。超出预扣上限返回 403（insufficient_quota）。", "Exceeding rate limits returns 429 (rate_limit_exceeded). Exceeding pre-charge cap returns 403 (insufficient_quota).", "تجاوز حدود المعدل يُرجع 429. تجاوز حد الخصم المسبق يُرجع 403.")}</p>
              </div>
            </>
          )}
          {activeTab === "faq" && (
            <>
              <p className="docs-lead">
                {text(
                  "关于平台使用的常见问题与解答。",
                  "Frequently asked questions about the platform.",
                  "الأسئلة الشائعة حول استخدام المنصة.",
                )}
              </p>
              {[
                {
                  q: text("密钥明文能在哪里看到？", "Where can I see the plaintext API key?", "أين يمكنني رؤية مفتاح API الصريح؟"),
                  a: text("仅在创建或重置密钥时一次性显示。之后只显示掩码（sk-****）。请妥善保存。", "Only shown once at creation or regeneration. After that only the mask (sk-****) is displayed. Save it securely.", "يُعرض مرة واحدة فقط عند الإنشاء أو إعادة التعيين. بعد ذلك يُعرض القناع فقط (sk-****). احفظه بأمان."),
                },
                {
                  q: text("失败请求会扣费吗？", "Are failed requests charged?", "هل تُحاسب الطلبات الفاشلة؟"),
                  a: text("不会。失败请求不扣费。预扣额度在失败时全额退还。", "No. Failed requests are not charged. Pre-charged quota is fully refunded on failure.", "لا. لا تُحاسب الطلبات الفاشلة. تُسترد الحصة المخصومة مسبقًا بالكامل عند الفشل."),
                },
                {
                  q: text("如何切换模型？", "How do I switch models?", "كيف أبدل النماذج؟"),
                  a: text("在请求中将 model 字段改为目标模型标识即可。前提是密钥授权范围内包含该模型。", "Change the model field in your request to the target model ID. Requires the model to be in your key's authorized scope.", "غيّر حقل model في طلبك إلى معرف النموذج المستهدف. يتطلب أن يكون النموذج ضمن نطاق تفويض مفتاحك."),
                },
                {
                  q: text("充值后多久到账？", "How long does top-up take?", "كم يستغرق الشحن؟"),
                  a: text("充值成功后即时到账。可在余额页查看最新余额。", "Credited immediately upon successful payment. Check the balance page for the latest amount.", "يُضاف فور نجاح الدفع. راجع صفحة الرصيد للحصول على أحدث مبلغ."),
                },
              ].map(({ q, a }, i) => (
                <details key={i} className="docs-callout">
                  <summary style={{ cursor: "pointer", fontWeight: 600, fontSize: "var(--text-sm)" }}>{q}</summary>
                  <p style={{ marginTop: 8, fontSize: "var(--text-sm)", color: "var(--secondary)" }}>{a}</p>
                </details>
              ))}
            </>
          )}
          <footer className="docs-article-footer">
            {text(
              "MyAPIs开发者文档 · 产品演示",
              "MyAPIs developer documentation · Product preview",
              "وثائق مطوري MyAPIs · معاينة المنتج",
            )}
          </footer>
        </main>
      </div>
    </div>
  );
}
