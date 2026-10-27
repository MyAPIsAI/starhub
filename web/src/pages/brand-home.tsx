import { useState, type CSSProperties } from "react";
import { Link } from "react-router-dom";
import {
  ArrowRight,
  ArrowUpRight,
  CheckCheck,
  ChevronDown,
  Code2,
  Copy,
  Layers3,
  ShieldCheck,
  Zap,
} from "lucide-react";
import {
  ProductBrand,
  PublicHeader,
  Quickstart,
} from "../components/product-shell";
import { Modal } from "../components/ui/overlay";
import { DemoScenarios } from "../components/demo-scenarios";
import { BrandMark } from "../components/brand";
import { catalogModels } from "../config/models";
import { placeholders } from "../config/placeholders";
import { useModelPricing, quoteModelCharge } from "../lib/model-pricing";
import { serviceGroupLabel } from "../lib/service-group-label";
import { formatMoney } from "../lib/money";
import { usePreferences } from "../lib/preferences";
import "../components/home-redesign.css";
import AuthDialog from "./auth/auth-dialog";
import { HomePortal } from "../components/home-portal";

const copy = {
  zh: {
    login: "登录",
    console: "进入控制台",
    eyebrow: "连接模型，让创造发生",
    title: ["一把密钥，", "多个模型。"],
    intro:
      "在熟悉的工具中选择和切换模型，\nVeyra（平台）统一管理接入、权限、费用与异常。",
    start: "开始接入",
    guide: "体验 Demo（演示）",
    footnote: "按量计费 · 灵活切换 · 用量透明",
    visualLabel: "从一个想法，到无限可能",
    route: "统一模型网关",
    online: "连接就绪",
    selected: "当前模型",
    select: "点击切换模型，探索更多可能",
    providers: "你熟悉的模型，都在这里",
    productEyebrow: "更少繁琐，更多创造",
    productTitle: "从第一次调用，\n到下一次突破。",
    productBody: "接入、调试、观察、优化。\n在一个清晰的工作台里，完成每一步。",
    dashboard: "查看工作台",
    preview: "控制台预览 · 演示数据",
    overview: "工作台概览",
    date: "最近 7 天",
    expense: "累计消费",
    requests: "调用次数",
    success: "请求成功率",
    trend: "消费趋势",
    balance: "可用余额",
    wallet: "统一账户，清晰掌握每一笔用量",
    recent: "最近调用",
    model: "模型",
    service: "服务等级",
    status: "状态",
    amount: "费用",
    done: "成功",
    features: [
      "一次接入，自由选择",
      "为每个场景找到平衡",
      "每一笔用量，都有答案",
    ],
    featureBodies: [
      "统一的调用方式，让你在多个模型之间灵活切换，专注产品本身。",
      "一把密钥对应一个服务分组。每次在客户端选择指定模型或智能路由，无需重复配置密钥。",
      "从用量曲线到单次请求明细，费用与调用状态清晰可查。",
    ],
    modelsEyebrow: "为你的下一个作品，找到合适的模型",
    modelsTitle: "优秀模型，随时就位。",
    allModels: "探索全部模型",
    context: "上下文",
    input: "普通输入 / 百万词元",
    unavailable: "暂无可用分组",
    modelNote:
      "展示可用分组中的最低普通输入价，单位为美元 / 百万词元；来自管理员发布的演示配置，输出与缓存价格见模型详情。",
    integrateEyebrow: "简单开始，持续构建",
    integrateTitle: "下一次灵感，\n从第一行代码开始。",
    integrateBody:
      "创建或复用密钥，选择接入工具，再在客户端切换模型。\n调试台可选，费用与调用始终可查。",
    quickstart: "快速接入",
    copied: "已复制",
    copy: "复制示例",
    codeNote: "代码为接入示意，请在文档中确认服务地址与密钥。",
    faqTitle: "开始之前，你可能想了解",
    faqs: [
      [
        "如何选择适合的模型？",
        "你可以在模型广场按厂商筛选，比较上下文、能力与各服务等级价格，再到在线调试台验证实际效果。",
      ],
      [
        "可以随时切换模型吗？",
        "可以。使用兼容的接口和密钥权限，在请求中更换模型名称即可；不同模型支持的参数与能力请以文档为准。",
      ],
      [
        "如何查看费用与调用情况？",
        "控制台提供用量报表、逐条请求明细和账本流水。当前页面中的数值均为交互演示数据。",
      ],
    ],
    cta: "好想法，值得即刻开始。",
    ctaBody: "让 Veyra，成为你下一次创造的起点。",
    copyright: "Veyra · 让智能触手可及",
    demo: "交互演示版本 · 展示数据不代表实时服务承诺",
    docs: "开发文档",
    system: "设计系统",
    privacy: "隐私政策",
    terms: "服务条款",
    menu: "展开导航",
  },
  en: {
    login: "Log in",
    console: "Open console",
    eyebrow: "Connected intelligence. Room to create.",
    title: ["One key.", "Many models."],
    intro:
      "Switch models in the tools you already know.\nVeyra manages access, permissions, costs and troubleshooting.",
    start: "Start connecting",
    guide: "Explore the demo",
    footnote: "Pay as you go · Flexible models · Clear usage",
    visualLabel: "One idea. Endless possibilities.",
    route: "Unified model gateway",
    online: "Ready to connect",
    selected: "Selected model",
    select: "Choose a model to explore the possibilities",
    providers: "The models you know. Together at last.",
    productEyebrow: "LESS FRICTION. MORE CREATION.",
    productTitle: "From your first call\nto your next breakthrough.",
    productBody:
      "Connect, test, observe, and optimize.\nOne clear console for every step.",
    dashboard: "Explore the console",
    preview: "Console preview · Demo data",
    overview: "Console overview",
    date: "Last 7 days",
    expense: "Total spend",
    requests: "Requests",
    success: "Success rate",
    trend: "Spending overview",
    balance: "Available balance",
    wallet: "One account. A clear view of every call.",
    recent: "Recent requests",
    model: "Model",
    service: "Service tier",
    status: "Status",
    amount: "Cost",
    done: "Success",
    features: [
      "Connect once. Choose freely.",
      "Find your perfect balance.",
      "Every request, accounted for.",
    ],
    featureBodies: [
      "A consistent interface lets you switch between models and focus on what you are building.",
      "Choose service tiers around your needs for performance, reliability, and cost.",
      "Understand every request with clear usage charts, cost breakdowns, and request details.",
    ],
    modelsEyebrow: "THE RIGHT MODEL FOR YOUR NEXT IDEA",
    modelsTitle: "Exceptional models. Ready when you are.",
    allModels: "Explore all models",
    context: "Context",
    input: "Uncached input / 1M tokens",
    unavailable: "No available groups",
    modelNote:
      "Lowest available uncached input price in USD per million tokens, from the published demo pricing configuration. See model details for output and cache rates.",
    integrateEyebrow: "START SIMPLE. KEEP BUILDING.",
    integrateTitle: "Your next idea starts\nwith a single line.",
    integrateBody:
      "Create a key. Pick a model. Send a request.\nA familiar interface for a smoother workflow.",
    quickstart: "Quickstart guide",
    copied: "Copied",
    copy: "Copy example",
    codeNote:
      "Illustrative example. Check the docs for your endpoint and credentials.",
    faqTitle: "A few things before you begin",
    faqs: [
      [
        "How do I find the right model?",
        "Filter the model catalog by provider, compare capabilities, context, and tier pricing, then try your use case in the playground.",
      ],
      [
        "Can I switch models anytime?",
        "Yes. Change the model name in a compatible request, subject to your key permissions. Check the docs for model-specific capabilities.",
      ],
      [
        "Where can I view my usage and costs?",
        "The console includes usage reports, request details, and transaction history. All values shown here are demonstration data.",
      ],
    ],
    cta: "Your next great idea starts here.",
    ctaBody: "Build something worth connecting.",
    copyright: "Veyra · Intelligence, within reach.",
    demo: "Interactive demo · Sample data is not a live service guarantee",
    docs: "Documentation",
    system: "Design system",
    privacy: "Privacy Policy",
    terms: "Terms of Service",
    menu: "Open navigation",
  },
  ar: {
    login: "تسجيل الدخول",
    console: "لوحة التحكم",
    eyebrow: "ذكاء متصل ومساحة للإبداع",
    title: ["مفتاح واحد.", "نماذج عديدة."],
    intro:
      "بدّل النماذج في أدواتك المألوفة.\nتدير Veyra الاتصال والصلاحيات والتكلفة والأخطاء.",
    start: "ابدأ الاتصال",
    guide: "جرّب العرض",
    footnote: "الدفع حسب الاستخدام · مرونة · شفافية",
    visualLabel: "فكرة واحدة. إمكانيات بلا حدود.",
    route: "بوابة نماذج موحدة",
    online: "جاهز للاتصال",
    selected: "النموذج المختار",
    select: "اختر نموذجاً لاستكشاف الإمكانيات",
    providers: "النماذج التي تعرفها، في مكان واحد",
    productEyebrow: "بساطة أكثر وإبداع أكبر",
    productTitle: "من أول طلب\nإلى إنجازك القادم.",
    productBody: "اتصل واختبر وراقب وحسّن.\nلوحة تحكم واحدة لكل خطوة.",
    dashboard: "اكتشف لوحة التحكم",
    preview: "معاينة لوحة التحكم · بيانات تجريبية",
    overview: "نظرة عامة",
    date: "آخر 7 أيام",
    expense: "إجمالي الإنفاق",
    requests: "الطلبات",
    success: "نسبة النجاح",
    trend: "اتجاه الإنفاق",
    balance: "الرصيد المتاح",
    wallet: "حساب واحد ورؤية واضحة لكل طلب",
    recent: "الطلبات الأخيرة",
    model: "النموذج",
    service: "فئة الخدمة",
    status: "الحالة",
    amount: "التكلفة",
    done: "نجاح",
    features: [
      "اتصال واحد وخيارات عديدة",
      "توازن يناسب احتياجاتك",
      "كل طلب واضح بالتفصيل",
    ],
    featureBodies: [
      "واجهة موحدة للتبديل بين النماذج والتركيز على منتجك.",
      "اختر الفئة المناسبة للأداء والموثوقية والتكلفة.",
      "رسوم استخدام واضحة وتفاصيل كل طلب وتكاليفه.",
    ],
    modelsEyebrow: "النموذج المناسب لفكرتك القادمة",
    modelsTitle: "نماذج استثنائية جاهزة لك.",
    allModels: "جميع النماذج",
    context: "السياق",
    input: "إدخال غير مخزن / مليون رمز",
    unavailable: "لا توجد مجموعات متاحة",
    modelNote:
      "أقل سعر إدخال غير مخزن بين المجموعات المتاحة بالدولار لكل مليون رمز، من إعدادات التسعير التجريبية المنشورة. أسعار الإخراج والتخزين في تفاصيل النموذج.",
    integrateEyebrow: "ابدأ ببساطة واستمر بالبناء",
    integrateTitle: "فكرتك القادمة تبدأ\nبسطر واحد.",
    integrateBody:
      "أنشئ مفتاحاً واختر نموذجاً وأرسل طلباً.\nواجهة مألوفة لتجربة تطوير أسهل.",
    quickstart: "البدء السريع",
    copied: "تم النسخ",
    copy: "نسخ المثال",
    codeNote: "مثال توضيحي. راجع التوثيق لعنوان الخدمة والمفتاح.",
    faqTitle: "قبل أن تبدأ",
    faqs: [
      [
        "كيف أختار النموذج المناسب؟",
        "قارن القدرات والسياق والأسعار في كتالوج النماذج، ثم اختبره في مساحة التجربة.",
      ],
      [
        "هل يمكن تغيير النموذج؟",
        "نعم، غيّر اسم النموذج في الطلب المتوافق وفقاً لصلاحيات المفتاح وتحقق من التوثيق.",
      ],
      [
        "أين أجد الاستخدام والتكلفة؟",
        "تضم لوحة التحكم تقارير الاستخدام وتفاصيل الطلبات والمعاملات. القيم المعروضة تجريبية.",
      ],
    ],
    cta: "فكرتك القادمة تبدأ هنا.",
    ctaBody: "ابنِ شيئاً يستحق الاتصال.",
    copyright: "Veyra · الذكاء في متناولك",
    demo: "عرض تفاعلي · البيانات ليست ضماناً للخدمة",
    docs: "التوثيق",
    system: "نظام التصميم",
    privacy: "سياسة الخصوصية",
    terms: "شروط الخدمة",
    menu: "فتح القائمة",
  },
};
const providers = [
  { short: "✳", name: "OpenAI", color: "#222f2c" },
  { short: "✺", name: "Anthropic", color: "#bf7961" },
  { short: "✦", name: "Google", color: "#4386f5" },
  { short: "≈", name: "DeepSeek", color: "#5179f9" },
  { short: "◇", name: "Qwen", color: "#8362df" },
];

export default function BrandHome() {
  const language = usePreferences((s) => s.language);
  const pricing = useModelPricing((s) => s.config);
  const c = copy[language];
  const [active, setActive] = useState(0);
  const [quickstart, setQuickstart] = useState(false);
  const [scenariosOpen, setScenariosOpen] = useState(false);
  const [codeTab, setCodeTab] = useState<"python" | "curl">("python");
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const selected = catalogModels[active];
  const code =
    codeTab === "python"
      ? `from openai import OpenAI\n\nclient = OpenAI(\n    api_key="YOUR_API_KEY",\n    base_url="YOUR_BASE_URL"\n)\n\nresponse = client.chat.completions.create(\n    model="${selected.id}",\n    messages=[{"role": "user", "content": "Hello!"}]\n)`
      : `curl "$YOUR_BASE_URL/chat/completions" \\\n  -H "Authorization: Bearer $YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "${selected.id}",\n    "messages": [{"role": "user", "content": "Hello!"}]\n  }'`;
  const copyCode = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopyFailed(false);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  };
  return (
    <div className="xy-home xy-home-portal">
      <PublicHeader />
      <main>
        <HomePortal language={language} onDemo={() => setScenariosOpen(true)}>
          <section className="xy-provider-section xy-width">
            <div className="xy-provider-heading">
              <span>{c.providers}</span>
              <small>{c.select}</small>
            </div>
            <div className="xy-providers">
              {providers.map((p, i) => (
                <button
                  key={p.name}
                  className={i === active ? "is-selected" : ""}
                  aria-pressed={active === i}
                  onClick={() => setActive(i)}
                >
                  <span className="xy-provider-icon" style={{ color: p.color }}>
                    {p.short}
                  </span>
                  <b>{p.name}</b>
                  <span className="provider-dot" />
                </button>
              ))}
            </div>
          </section>
          <section className="xy-workspace-section" id="why">
            <div className="xy-width">
              <div className="xy-section-heading">
                <div>
                  <span className="xy-section-kicker">{c.productEyebrow}</span>
                  <h2>{c.productTitle}</h2>
                </div>
                <div>
                  <p>{c.productBody}</p>
                  <Link className="xy-text-button" to="/console">
                    {c.dashboard}
                    <ArrowUpRight size={16} />
                  </Link>
                </div>
              </div>
              <div className="xy-dashboard-preview">
                <div className="xy-preview-side" aria-hidden="true">
                  <BrandMark />
                  {[Layers3, Zap, Code2, ShieldCheck].map((Icon, i) => (
                    <span className={i === 0 ? "active" : ""} key={i}>
                      <Icon size={17} />
                    </span>
                  ))}
                </div>
                <div className="xy-preview-main">
                  <div className="xy-preview-top">
                    <strong>{c.overview}</strong>
                    <span>
                      <i />
                      {c.preview}
                    </span>
                  </div>
                  <div className="xy-preview-metrics">
                    {[
                      [c.expense, "$38.2045"],
                      [c.requests, "24,892"],
                      [c.success, "99.82%"],
                    ].map(([label, value], i) => (
                      <div key={label}>
                        <span>
                          {label}
                          <ArrowUpRight size={13} />
                        </span>
                        <strong>{value}</strong>
                        <small>
                          {i === 0 ? c.date : i === 1 ? "5 " + c.model : c.done}
                        </small>
                      </div>
                    ))}
                  </div>
                  <div className="xy-preview-bottom">
                    <div className="xy-preview-chart">
                      <div>
                        <b>{c.trend}</b>
                        <span>
                          {c.date}
                          <ChevronDown size={12} />
                        </span>
                      </div>
                      <div className="xy-chart-container">
                        <div className="xy-axis">
                          <span>$12</span>
                          <span>$8</span>
                          <span>$4</span>
                          <span>$0</span>
                        </div>
                        <svg
                          viewBox="0 0 650 150"
                          preserveAspectRatio="none"
                          role="img"
                          aria-label={c.trend}
                        >
                          <defs>
                            <linearGradient
                              id="home-chart-fill"
                              x1="0"
                              x2="0"
                              y1="0"
                              y2="1"
                            >
                              <stop
                                offset="0%"
                                stopColor="#4778f7"
                                stopOpacity=".2"
                              />
                              <stop
                                offset="100%"
                                stopColor="#4778f7"
                                stopOpacity="0"
                              />
                            </linearGradient>
                          </defs>
                          {[15, 55, 95, 135].map((y) => (
                            <line
                              key={y}
                              x1="0"
                              x2="650"
                              y1={y}
                              y2={y}
                              stroke="var(--border)"
                              strokeDasharray="3 6"
                            />
                          ))}
                          <path
                            d="M0 125 C25 130 36 88 65 95 S98 70 130 84 S166 50 196 72 S229 103 264 85 S298 30 328 46 S365 19 393 40 S427 86 460 60 S495 14 526 28 S567 4 588 19 S623 5 650 10 L650 150 L0 150Z"
                            fill="url(#home-chart-fill)"
                          />
                          <path
                            d="M0 125 C25 130 36 88 65 95 S98 70 130 84 S166 50 196 72 S229 103 264 85 S298 30 328 46 S365 19 393 40 S427 86 460 60 S495 14 526 28 S567 4 588 19 S623 5 650 10"
                            fill="none"
                            stroke="#4778f7"
                            strokeWidth="2.5"
                          />
                        </svg>
                      </div>
                      <div className="xy-chart-days">
                        {[
                          "09.17",
                          "09.18",
                          "09.19",
                          "09.20",
                          "09.21",
                          "09.22",
                          "09.23",
                        ].map((day) => (
                          <span key={day}>{day}</span>
                        ))}
                      </div>
                    </div>
                    <div className="xy-preview-wallet">
                      <div>
                        {c.balance}
                        <ArrowUpRight size={15} />
                      </div>
                      <strong>$528.40</strong>
                      <p>{c.wallet}</p>
                      <Link to="/console/billing">
                        {c.console}
                        <ArrowRight size={15} />
                      </Link>
                      <BrandMark />
                    </div>
                  </div>
                </div>
              </div>
              <div className="xy-features">
                {[Layers3, Zap, ShieldCheck].map((Icon, i) => (
                  <article key={i}>
                    <span className="xy-feature-icon">
                      <Icon size={20} />
                    </span>
                    <h3>{c.features[i]}</h3>
                    <p>{c.featureBodies[i]}</p>
                  </article>
                ))}
              </div>
            </div>
          </section>
          <section className="xy-model-section xy-width" id="models">
            <div className="xy-section-heading">
              <div>
                <span className="xy-section-kicker">{c.modelsEyebrow}</span>
                <h2>{c.modelsTitle}</h2>
              </div>
              <Link className="xy-text-button" to="/models">
                {c.allModels}
                <ArrowUpRight size={16} />
              </Link>
            </div>
            <div className="xy-model-grid">
              {catalogModels.slice(0, 4).map((m, i) => {
                const quotes = placeholders.tiers
                  .flatMap((tier) => {
                    try {
                      return [
                        {
                          tier: tier.id,
                          ...quoteModelCharge(
                            {
                              modelId: m.id,
                              tierId: tier.id,
                              usage: {
                                input_tokens: 1000,
                                output_tokens: 300,
                                cache_read_tokens: 0,
                                cache_write_tokens: 0,
                                requests: 1,
                              },
                            },
                            pricing,
                          ),
                        },
                      ];
                    } catch {
                      return [];
                    }
                  })
                  .sort((a, b) =>
                    BigInt(a.amountNano) < BigInt(b.amountNano) ? -1 : 1,
                  );
                const quote = quotes[0];
                return (
                  <Link
                    className="xy-model-card"
                    key={m.id}
                    to={`/models/${m.id}`}
                  >
                    <div className="xy-model-top">
                      <span
                        className="xy-model-symbol"
                        style={
                          {
                            "--vendor-color": providers[i].color,
                          } as CSSProperties
                        }
                      >
                        {providers[i].short}
                      </span>
                      <ArrowUpRight size={18} />
                    </div>
                    <span className="xy-model-vendor">{m.vendor}</span>
                    <h3>{m.name}</h3>
                    <p>{m.description[language]}</p>
                    <div className="xy-model-spec">
                      <span>
                        {c.context}
                        <b>{m.contextDisplay}</b>
                      </span>
                      <span>
                        {quote
                          ? language === "zh"
                            ? "示例费用 / 次起"
                            : language === "ar"
                              ? "تقدير لكل طلب من"
                              : "Estimate / call from"
                          : c.unavailable}
                        <b>
                          {quote ? formatMoney(quote.amountNano, "call") : "—"}
                        </b>
                        {quote && (
                          <small className="xy-model-price-group">
                            {serviceGroupLabel(quote.tier, language, pricing)}
                          </small>
                        )}
                      </span>
                    </div>
                  </Link>
                );
              })}
            </div>
            <p className="xy-section-note">
              {language === "zh"
                ? "模拟价格。按当前时刻、1,000 输入 + 300 输出词元、无缓存估算，已包含对应分组及时段系数；实际费用取决于用量与模型规则。"
                : language === "ar"
                  ? "أسعار تجريبية: 1000 رمز إدخال و300 إخراج بلا تخزين، تشمل معامل المجموعة والفترة الحالية."
                  : "Sample prices: 1,000 input + 300 output tokens, no cache, current time and group multipliers included. Actual cost depends on usage and model rules."}
            </p>
          </section>
          <section className="xy-integration xy-width">
            <div>
              <span className="xy-section-kicker">{c.integrateEyebrow}</span>
              <h2>{c.integrateTitle}</h2>
              <p>{c.integrateBody}</p>
              <button
                className="xy-btn xy-btn-dark"
                onClick={() => setQuickstart(true)}
              >
                {c.quickstart}
                <ArrowUpRight size={16} />
              </button>
            </div>
            <div className="xy-code-card">
              <div className="xy-code-toolbar">
                <div role="tablist" aria-label={c.copy}>
                  {(["python", "curl"] as const).map((tab) => (
                    <button
                      role="tab"
                      aria-selected={codeTab === tab}
                      key={tab}
                      onClick={() => setCodeTab(tab)}
                    >
                      {tab === "python"
                        ? "Python" + (language === "zh" ? "（编程语言）" : "")
                        : "cURL" + (language === "zh" ? "（命令行）" : "")}
                    </button>
                  ))}
                </div>
                <button
                  onClick={copyCode}
                  aria-label={copied ? c.copied : c.copy}
                >
                  {copied ? <CheckCheck size={15} /> : <Copy size={15} />}
                </button>
              </div>
              <pre dir="ltr">
                <code>
                  {code.split("\n").map((line, i) => (
                    <span key={i}>
                      <em>{i + 1}</em>
                      <span
                        className={
                          line.includes('"')
                            ? "code-string"
                            : line.startsWith("from")
                              ? "code-import"
                              : ""
                        }
                      >
                        {line || " "}
                      </span>
                      {"\n"}
                    </span>
                  ))}
                </code>
              </pre>
              <div className="xy-code-note" role="status">
                <i />
                {copyFailed
                  ? language === "zh"
                    ? "复制失败，请选中代码手动复制。"
                    : language === "ar"
                      ? "تعذر النسخ. حدد الرمز وانسخه يدوياً."
                      : "Copy failed. Select the code and copy it manually."
                  : c.codeNote}
              </div>
            </div>
          </section>
          <section className="xy-faq xy-width">
            <h2>{c.faqTitle}</h2>
            <div>
              {c.faqs.map(([question, answer]) => (
                <details key={question}>
                  <summary>
                    {question}
                    <span>+</span>
                  </summary>
                  <p>{answer}</p>
                </details>
              ))}
            </div>
          </section>
          <section className="xy-final-cta xy-width">
            <div className="xy-cta-pixels" aria-hidden="true" />
            <span className="xy-cta-star">
              <BrandMark />
            </span>
            <h2>{c.cta}</h2>
            <p>{c.ctaBody}</p>
            <Link className="xy-btn xy-btn-blue" to="/console/connect">
              {c.start}
              <ArrowUpRight size={16} />
            </Link>
          </section>
        </HomePortal>
      </main>
      <footer className="xy-footer xy-width">
        <div>
          <ProductBrand compact />
          <p>{c.copyright}</p>
        </div>
        <nav>
          <Link to="/docs">{c.docs}</Link>
          <Link to="/legal/privacy">{c.privacy}</Link>
          <Link to="/legal/terms">{c.terms}</Link>
          <Link to="/design-system">{c.system}</Link>
        </nav>
        <small>{c.demo}</small>
      </footer>
      <Quickstart open={quickstart} onOpenChange={setQuickstart} />
      <AuthDialog />
      <Modal
        open={scenariosOpen}
        onOpenChange={setScenariosOpen}
        title={
          language === "zh"
            ? "选择演示场景"
            : language === "ar"
              ? "اختر سيناريو العرض"
              : "Choose a demo scenario"
        }
        description={
          language === "zh"
            ? "所有调用、价格与支付状态均为模拟。"
            : language === "ar"
              ? "جميع الطلبات والأسعار والمدفوعات محاكاة."
              : "All calls, prices and payment states are simulated."
        }
      >
        <DemoScenarios onSelect={() => setScenariosOpen(false)} />
      </Modal>
    </div>
  );
}
