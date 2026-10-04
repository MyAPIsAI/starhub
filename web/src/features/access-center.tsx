import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  ArrowRight,
  CheckCircle2,
  Copy,
  KeyRound,
  Link2,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { usePreferences } from "../lib/preferences";
import { useProductPreview } from "../lib/product-preview";
import { useModelPricing } from "../lib/model-pricing";
import { serviceGroupLabel } from "../lib/service-group-label";
import {
  buildClientConfiguration,
  clientProtocols,
  getClientModels,
  type ClientTarget,
  type ConnectionTool,
} from "../lib/client-config";
import { getKeyModelOptions } from "../lib/key-model-policy";
import { copyText } from "./code-example";
import "../components/access-center.css";

const DRAFT_KEY = "veyra-connect-draft-v1";
type Draft = {
  key: string;
  tool: ConnectionTool;
  target: ClientTarget;
  model: string;
};
function readDraft(): Partial<Draft> {
  try {
    const value = JSON.parse(sessionStorage.getItem(DRAFT_KEY) ?? "{}");
    return typeof value === "object" && value ? value : {};
  } catch {
    return {};
  }
}
const isTool = (v: unknown): v is ConnectionTool =>
  ["openai", "claude-code", "codex", "cc-switch"].includes(String(v));
const isTarget = (v: unknown): v is ClientTarget =>
  ["openai", "claude-code", "codex"].includes(String(v));

export default function AccessCenter() {
  const [params] = useSearchParams();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const keys = useProductPreview((s) => s.keys);
  const selectedKey = useProductPreview((s) => s.selectedKey);
  const requests = useProductPreview((s) => s.requests);
  const revealed = useProductPreview((s) => s.revealedKeyIds);
  const chooseKey = useProductPreview((s) => s.chooseKey);
  const unlock = useProductPreview((s) => s.verifyPasswordAndRevealKey);
  const pricing = useModelPricing((s) => s.config);
  const [draft, setDraft] = useState<Draft>(() => {
    const saved = readDraft();
    const model =
      params.get("model") ??
      (typeof saved.model === "string" ? saved.model : "");
    const matching = keys.find(
      (key) =>
        key.active &&
        (!model ||
          getKeyModelOptions(key).some((option) => option.id === model)),
    );
    const key =
      params.get("key") ??
      (params.has("model") ? matching?.id : (saved.key ?? selectedKey)) ??
      matching?.id ??
      "";
    const tool = isTool(saved.tool) ? saved.tool : "openai";
    const target =
      tool === "cc-switch"
        ? isTarget(saved.target)
          ? saved.target
          : "claude-code"
        : tool;
    const foundKey = keys.find((item) => item.id === key);
    return {
      key,
      tool,
      target,
      model:
        model ||
        (foundKey ? getClientModels(foundKey, target)[0]?.id : "") ||
        "",
    };
  });
  const [clock, setClock] = useState(Date.now());
  const [includeSecret, setIncludeSecret] = useState(false);
  const [password, setPassword] = useState("");
  const [unlockError, setUnlockError] = useState(false);
  const [unlocking, setUnlocking] = useState(false);
  const [copied, setCopied] = useState("");
  const key = keys.find((item) => item.id === draft.key);
  const protocol = clientProtocols[draft.target];
  const models = useMemo(
    () => (key ? getClientModels(key, draft.target, pricing) : []),
    [key, draft.target, pricing],
  );
  const selectedModel = models.find((model) => model.id === draft.model);
  const unlocked = Boolean(key && (revealed[key.id] ?? 0) > clock);
  const config =
    key?.active && selectedModel
      ? buildClientConfiguration({
          target: draft.target,
          model: selectedModel.id,
          secret: includeSecret && unlocked ? key.keySecret : undefined,
        })
      : null;
  const latestRequest = requests.find(
    (row) =>
      row.key_id === key?.id &&
      row.requested_model === draft.model &&
      (row.protocol ?? row.request?.protocol) === protocol,
  );
  const verified =
    latestRequest?.status === "success" ? latestRequest : undefined;
  const failed =
    latestRequest && latestRequest.status !== "success"
      ? latestRequest
      : undefined;
  useEffect(() => {
    try {
      sessionStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
    } catch {
      /* In-memory configuration remains usable. */
    }
  }, [draft]);
  useEffect(() => {
    const timer = window.setInterval(() => setClock(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const update = (changes: Partial<Draft>) => {
    setDraft((prev) => ({ ...prev, ...changes }));
    setCopied("");
  };
  const toolName = (target: ConnectionTool) =>
    ({
      openai: text(
        "通用 OpenAI（兼容工具）",
        "OpenAI-compatible tool",
        "أداة متوافقة مع OpenAI",
      ),
      "claude-code": text(
        "Claude Code（代码客户端）",
        "Claude Code",
        "Claude Code",
      ),
      codex: text("Codex（代码客户端）", "Codex", "Codex"),
      "cc-switch": text("CC Switch（配置管理工具）", "CC Switch", "CC Switch"),
    })[target];
  const validationUrl = `/console/playground?${new URLSearchParams({ key: draft.key, model: draft.model, protocol: protocol ?? "OpenAI" })}`;
  return (
    <div className="access-center">
      <header className="access-heading">
        <span className="access-demo-label">
          <Link2 size={14} />
          {text(
            "接入中心 · 本地交互演示",
            "Connection center · Local demo",
            "مركز الاتصال · عرض محلي",
          )}
        </span>
        <h1>
          {text(
            "把模型接入你熟悉的工具",
            "Connect models to the tools you use",
            "اربط النماذج بالأدوات التي تستخدمها",
          )}
        </h1>
        <p>
          {text(
            "复用一把 Key（密钥），获取配置，再在客户端切换指定模型与智能路由。所有地址、凭证和验证调用均为演示，不连接真实模型服务。",
            "Reuse one key, get configuration, then switch between specific models and smart routing in your client. Endpoints, credentials, and validation calls are simulated; no live model service is connected.",
            "استخدم مفتاحًا واحدًا ثم بدّل النماذج في عميلك. العناوين والمفاتيح وطلبات التحقق محاكاة بلا اتصال بخدمة فعلية.",
          )}
        </p>
      </header>
      <div className="access-steps">
        <span>
          <b>1</b>
          {text("选择密钥与工具", "Key & tool", "المفتاح والأداة")}
        </span>
        <span>
          <b>2</b>
          {text("获取配置", "Get configuration", "الحصول على الإعداد")}
        </span>
        <span>
          <b>3</b>
          {text(
            "客户端选模",
            "Choose a model in client",
            "اختيار النموذج في العميل",
          )}
        </span>
        <span>
          <b>4</b>
          {text(
            "查看费用与异常",
            "Inspect cost & errors",
            "مراجعة التكلفة والأخطاء",
          )}
        </span>
      </div>
      <div className="access-layout">
        <section className="access-panel">
          <h2>
            <KeyRound size={18} />
            {text("选择接入配置", "Connection settings", "إعدادات الاتصال")}
          </h2>
          <div className="access-field">
            <label htmlFor="connect-key">
              {text("Key（密钥）", "Key", "المفتاح")}
            </label>
            <select
              id="connect-key"
              value={draft.key}
              onChange={(e) => {
                const next = keys.find((item) => item.id === e.target.value);
                chooseKey(e.target.value);
                setIncludeSecret(false);
                setPassword("");
                update({
                  key: e.target.value,
                  model:
                    next &&
                    getClientModels(next, draft.target, pricing).some(
                      (m) => m.id === draft.model,
                    )
                      ? draft.model
                      : "",
                });
              }}
            >
              <option value="">
                {text(
                  "选择已有密钥",
                  "Select an existing key",
                  "اختر مفتاحًا موجودًا",
                )}
              </option>
              {keys.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name} · {serviceGroupLabel(item.tier, lang, pricing)}
                  {!item.active
                    ? ` · ${text("已停用", "Disabled", "معطل")}`
                    : ""}
                </option>
              ))}
            </select>
            <small>
              {text(
                "优先复用已有兼容密钥；切换模型无需新建密钥。",
                "Reuse a compatible key; switching models does not require a new key.",
                "استخدم مفتاحًا متوافقًا؛ تغيير النموذج لا يتطلب مفتاحًا جديدًا.",
              )}
            </small>
            <Link
              to={`/console/keys?create=1${draft.model ? `&model=${encodeURIComponent(draft.model)}` : ""}`}
            >
              {text("创建新密钥", "Create a key", "إنشاء مفتاح")}
            </Link>
          </div>
          {key && (
            <div className="access-notice">
              <span>
                {serviceGroupLabel(key.tier, lang, pricing)} ·{" "}
                {key.quotaLimit_usd == null
                  ? text(
                      "未设置密钥消费上限",
                      "No key spending cap",
                      "لا حد إنفاق للمفتاح",
                    )
                  : `$${key.quotaLimit_usd} / ${key.quotaPeriod === "daily" ? text("日", "day", "يوم") : key.quotaPeriod === "monthly" ? text("月", "month", "شهر") : text("累计", "lifetime", "الإجمالي")}`}
                <br />
                {text(
                  "可用模型取决于分组、白名单及工具协议。",
                  "Available models depend on group, allowlist, and tool protocol.",
                  "تتحدد النماذج حسب المجموعة والصلاحيات وبروتوكول الأداة.",
                )}
              </span>
            </div>
          )}
          <div className="access-field">
            <label htmlFor="connect-tool">
              {text("接入工具", "Connection tool", "أداة الاتصال")}
            </label>
            <select
              id="connect-tool"
              value={draft.tool}
              onChange={(e) => {
                const tool = e.target.value as ConnectionTool;
                const target = tool === "cc-switch" ? "claude-code" : tool;
                update({ tool, target });
              }}
            >
              {(
                [
                  "openai",
                  "claude-code",
                  "codex",
                  "cc-switch",
                ] as ConnectionTool[]
              ).map((tool) => (
                <option key={tool} value={tool}>
                  {toolName(tool)}
                </option>
              ))}
            </select>
          </div>
          {draft.tool === "cc-switch" && (
            <div className="access-field">
              <label htmlFor="connect-target">
                {text(
                  "CC Switch（配置管理工具）的目标客户端",
                  "Target client configured by CC Switch",
                  "العميل المستهدف عبر CC Switch",
                )}
              </label>
              <select
                id="connect-target"
                value={draft.target}
                onChange={(e) =>
                  update({ target: e.target.value as ClientTarget })
                }
              >
                {(["claude-code", "codex", "openai"] as ClientTarget[]).map(
                  (target) => (
                    <option key={target} value={target}>
                      {toolName(target)}
                    </option>
                  ),
                )}
              </select>
              <small>
                {text(
                  "它管理目标工具配置，不改变目标工具的协议要求。此演示未确认版本化导入协议，使用手动填写。",
                  "It manages target-tool configuration; it does not change protocol requirements. Use manual entry because no versioned import contract has been verified.",
                  "يدير إعداد العميل ولا يغير متطلبات البروتوكول. الإدخال يدوي لأن صيغة الاستيراد لم تتحقق.",
                )}
              </small>
            </div>
          )}
          {draft.target !== "codex" && (
            <div className="access-field">
              <label htmlFor="connect-model">
                {text(
                  "客户端初始模型",
                  "Initial client model",
                  "النموذج الأولي في العميل",
                )}
              </label>
              <select
                id="connect-model"
                value={selectedModel?.id ?? ""}
                onChange={(e) => update({ model: e.target.value })}
                disabled={!key?.active}
              >
                <option value="">
                  {text(
                    "请选择适配模型",
                    "Choose a compatible model",
                    "اختر نموذجًا متوافقًا",
                  )}
                </option>
                {models.map((model) => (
                  <option value={model.id} key={model.id}>
                    {model.kind === "smart" ? "Veyra Auto" : model.name} ·{" "}
                    {model.kind === "smart"
                      ? text("智能路由", "Smart routing", "توجيه ذكي")
                      : text("指定模型", "Specific model", "نموذج محدد")}
                  </option>
                ))}
              </select>
              <small>
                {text(
                  "这是配置默认值，不会改变密钥授权；客户端可继续切换列表中的其他模型。",
                  "This is a default configuration value, not a key restriction. Your client can switch to other models listed here.",
                  "هذه قيمة افتراضية للإعداد ولا تغير صلاحيات المفتاح. يمكن التبديل إلى نماذج أخرى في القائمة.",
                )}
              </small>
            </div>
          )}
          {draft.model && !selectedModel && draft.target !== "codex" && (
            <div role="alert" className="access-notice access-error">
              {text(
                `模型 ${draft.model} 未获当前密钥授权、已不可用或不兼容此工具。请明确选择其他模型或密钥；不会自动替换。`,
                `Model ${draft.model} is unauthorized, unavailable, or incompatible. Explicitly choose another model or key; it will not be substituted.`,
                `النموذج ${draft.model} غير مصرح أو غير متاح أو غير متوافق. اختر نموذجًا أو مفتاحًا آخر صراحةً.`,
              )}
            </div>
          )}
          {key && !key.active && (
            <div role="alert" className="access-notice access-error">
              {text(
                "此密钥已停用。请到密钥管理启用，或选择其他密钥。",
                "This key is disabled. Enable it in key management or select another key.",
                "هذا المفتاح معطل. فعّله من إدارة المفاتيح أو اختر غيره.",
              )}{" "}
              <Link to="/console/keys">
                {text("管理密钥", "Manage keys", "إدارة المفاتيح")}
              </Link>
            </div>
          )}
        </section>
        <section className="access-panel">
          <h2>
            <Terminal size={18} />
            {text("手动配置", "Manual configuration", "الإعداد اليدوي")}
          </h2>
          {draft.target === "codex" ? (
            <div className="access-empty">
              <h2>
                {text(
                  "当前没有已确认适配的模型",
                  "No verified compatible models yet",
                  "لا توجد نماذج متوافقة مؤكدة",
                )}
              </h2>
              <p>
                {text(
                  "Codex（代码客户端）要求 Responses（响应接口）协议。现目录仅声明聊天、消息和 Gemini（谷歌模型）协议，不能据此认定兼容。",
                  "Codex requires the Responses protocol. The current catalog declares chat, messages, and Gemini protocols only; compatibility cannot be assumed.",
                  "يتطلب Codex بروتوكول Responses. يعلن الدليل الحالي بروتوكولات أخرى فقط ولا يمكن افتراض التوافق.",
                )}
              </p>
              <p>
                {text(
                  "正式适配后需在用户配置中设置模型、服务商、基础地址、环境变量密钥和 wire_api（接口协议）= responses；本演示不生成伪可用配置。",
                  "A supported provider would need model, provider, base URL, environment key, and wire_api = responses in user configuration. This demo does not generate an unverified usable configuration.",
                  "يتطلب المزود المتوافق النموذج والعنوان ومفتاح البيئة وبروتوكول responses. لا ينشئ العرض إعدادًا غير متحقق.",
                )}
              </p>
              <a
                href="https://developers.openai.com/codex/config-reference/"
                target="_blank"
                rel="noreferrer"
              >
                {text(
                  "查看官方配置说明",
                  "Official configuration reference",
                  "مرجع الإعداد الرسمي",
                )}
              </a>
              <Button
                variant="secondary"
                onClick={() =>
                  update({
                    target: "openai",
                    tool: draft.tool === "cc-switch" ? "cc-switch" : "openai",
                  })
                }
              >
                {text(
                  "改用通用兼容工具",
                  "Use a compatible generic tool",
                  "استخدام أداة عامة متوافقة",
                )}
              </Button>
            </div>
          ) : !config ? (
            <div className="access-empty">
              <Link2 size={32} />
              <h2>
                {text(
                  "先选择密钥与适配模型",
                  "Select a key and compatible model",
                  "اختر مفتاحًا ونموذجًا متوافقًا",
                )}
              </h2>
              <p>
                {text(
                  "配置会使用所选工具的协议及地址。找不到密钥时可以创建；已有选择保留在本机，刷新后可继续。",
                  "Configuration uses the selected tool’s protocol and endpoint. Create a key if needed. Your selections are saved locally so you can continue after refresh.",
                  "يستخدم الإعداد بروتوكول الأداة وعنوانها. تُحفظ اختياراتك محليًا للاستمرار بعد التحديث.",
                )}
              </p>
            </div>
          ) : (
            <>
              <div className="access-notice">
                {text(
                  "示例域名不会接入真实服务。下方配置用于演示接入关系；真实工具连通性尚未验证。",
                  "The example domain does not reach a live service. Configuration demonstrates the integration; real client connectivity is not verified.",
                  "النطاق مثال ولا يصل إلى خدمة فعلية. الإعداد يوضح التكامل ولم يتحقق الاتصال الفعلي.",
                )}
              </div>
              <dl className="access-config-fields">
                <div>
                  <dt>{text("目标工具", "Target tool", "الأداة المستهدفة")}</dt>
                  <dd>{toolName(draft.target)}</dd>
                </div>
                <div>
                  <dt>{text("基础地址", "Base URL", "العنوان الأساسي")}</dt>
                  <dd>
                    <code>{config.baseUrl}</code>
                  </dd>
                </div>
                <div>
                  <dt>{text("协议", "Protocol", "البروتوكول")}</dt>
                  <dd>
                    {config.protocol}
                    {lang === "zh"
                      ? config.protocol === "Anthropic"
                        ? "（消息接口）"
                        : "（聊天补全接口）"
                      : ""}
                  </dd>
                </div>
                <div>
                  <dt>{text("模型标识", "Model ID", "معرف النموذج")}</dt>
                  <dd>
                    <code>{config.model}</code>
                  </dd>
                </div>
                <div>
                  <dt>
                    {text("凭证来源", "Credential source", "مصدر المفتاح")}
                  </dt>
                  <dd>
                    {includeSecret && unlocked
                      ? text(
                          "已解锁的模拟密钥",
                          "Revealed demo credential",
                          "مفتاح تجريبي مكشوف",
                        )
                      : text(
                          "环境变量 VEYRA_API_KEY（密钥占位）",
                          "Environment variable VEYRA_API_KEY",
                          "متغير البيئة VEYRA_API_KEY",
                        )}
                  </dd>
                </div>
              </dl>
              {key && !unlocked && (
                <details className="access-details">
                  <summary>
                    {text(
                      "需要完整模拟密钥？解锁后加入配置",
                      "Need the demo credential? Unlock to include it",
                      "تحتاج المفتاح التجريبي؟ افتحه لإضافته",
                    )}
                  </summary>
                  <div className="access-field">
                    <label htmlFor="connect-password">
                      {text(
                        "当前账号密码",
                        "Current account password",
                        "كلمة مرور الحساب الحالية",
                      )}
                    </label>
                    <Input
                      id="connect-password"
                      type="password"
                      value={password}
                      onChange={(e) => {
                        setPassword(e.target.value);
                        setUnlockError(false);
                      }}
                      autoComplete="current-password"
                    />
                    <small>
                      {text(
                        "使用当前演示账号密码；若已修改，请输入新密码。不要输入真实服务口令。",
                        "Use the current password of your demo account, including any changes. Do not use real credentials.",
                        "استخدم كلمة مرور حساب العرض الحالية بعد أي تعديل. لا تستخدم بيانات حقيقية.",
                      )}
                    </small>
                    {unlockError && (
                      <small role="alert">
                        {text(
                          "当前密码验证失败；请重试，或返回登录页通过手机号找回。",
                          "Password verification failed. Retry or recover through phone login.",
                          "فشل التحقق. استخدم كلمة المرور التجريبية.",
                        )}
                      </small>
                    )}
                    <Button
                      size="small"
                      variant="secondary"
                      disabled={unlocking}
                      onClick={async () => {
                        setUnlocking(true);
                        const ok = await unlock(key.id, password);
                        setUnlocking(false);
                        setUnlockError(!ok);
                        if (ok) {
                          setPassword("");
                          setIncludeSecret(true);
                          setClock(Date.now());
                        }
                      }}
                    >
                      {text(
                        "解锁模拟密钥",
                        "Reveal demo key",
                        "إظهار المفتاح التجريبي",
                      )}
                    </Button>
                  </div>
                </details>
              )}
              {unlocked && (
                <label className="access-check">
                  <input
                    type="checkbox"
                    checked={includeSecret}
                    onChange={(e) => setIncludeSecret(e.target.checked)}
                  />
                  <span>
                    {text(
                      "在导出配置中包含已解锁的完整模拟密钥",
                      "Include the revealed demo credential in exported configuration",
                      "تضمين المفتاح التجريبي المكشوف في الإعداد",
                    )}
                  </span>
                </label>
              )}
              <pre className="access-code">
                <code>{config.snippet}</code>
              </pre>
              <p className="access-muted">
                {text(
                  "未解锁时仅引用环境变量；请先赋予它完整模拟密钥。通用工具的模型环境变量只是配置提示，应在工具的模型输入框中填写上方标识。",
                  "When locked, the snippet references an environment variable; set it to the complete demo credential first. Generic model environment variables are guidance; enter the model ID in your tool’s model field.",
                  "عند القفل يُستخدم متغير بيئة؛ عيّنه إلى المفتاح الكامل أولًا. أدخل معرف النموذج في حقل النموذج بالأداة.",
                )}
              </p>
              <div className="access-actions">
                <Button
                  variant="secondary"
                  onClick={async () => {
                    if (
                      await copyText(
                        config.snippet,
                        text(
                          "配置已复制",
                          "Configuration copied",
                          "نُسخ الإعداد",
                        ),
                        text(
                          "复制失败，请手动选择代码复制",
                          "Copy failed; select and copy the code manually",
                          "فشل النسخ؛ انسخ النص يدويًا",
                        ),
                      )
                    )
                      setCopied(config.snippet);
                  }}
                >
                  {copied === config.snippet ? (
                    <CheckCircle2 size={15} />
                  ) : (
                    <Copy size={15} />
                  )}
                  {copied === config.snippet
                    ? text("配置已复制", "Configuration copied", "نُسخ الإعداد")
                    : text("复制配置", "Copy configuration", "نسخ الإعداد")}
                </Button>
                <Button asChild variant="primary">
                  <Link to={validationUrl}>
                    {text(
                      "可选：验证配置",
                      "Optional: validate configuration",
                      "اختياري: التحقق من الإعداد",
                    )}
                    <ArrowRight size={15} />
                  </Link>
                </Button>
              </div>
              {copied === config.snippet && (
                <p className="access-muted" role="status">
                  {text(
                    "仅完成复制。尚未因此产生请求，也不代表客户端已接通。",
                    "Copied only. No request was generated, and this does not confirm client connectivity.",
                    "تم النسخ فقط. لم ينشأ طلب ولا يعني نجاح اتصال العميل.",
                  )}
                </p>
              )}
              <details className="access-details" open>
                <summary>
                  {text(
                    "按工具手动填写",
                    "Manual setup steps",
                    "خطوات الإعداد اليدوي",
                  )}
                </summary>
                <ol>
                  <li>
                    {draft.tool === "cc-switch"
                      ? text(
                          "在配置管理工具中选择上方目标客户端，新建自定义服务商。",
                          "Choose the target client in CC Switch, then add a custom provider.",
                          "اختر العميل المستهدف في CC Switch ثم أضف مزودًا مخصصًا.",
                        )
                      : text(
                          "在目标工具中新增自定义服务商；命令行用户可参考上方环境变量。",
                          "Add a custom provider in the target tool; CLI users can use the environment variables above.",
                          "أضف مزودًا مخصصًا في الأداة؛ يمكن استخدام متغيرات البيئة لسطر الأوامر.",
                        )}
                  </li>
                  <li>
                    {text(
                      "填写基础地址、完整模拟密钥和模型标识。不要填写掩码或把环境变量占位当作密钥原文。",
                      "Enter the base URL, full demo credential, and model ID. Do not enter a masked key or a literal environment placeholder.",
                      "أدخل العنوان والمفتاح الكامل ومعرف النموذج. لا تستخدم مفتاحًا مخفيًا أو نص متغير البيئة.",
                    )}
                  </li>
                  <li>
                    {draft.target === "claude-code"
                      ? text(
                          "将默认、主模型及子代理映射为所选模型；切换时同步更新映射，避免工具隐式请求未授权模型。",
                          "Map default model aliases and subagents to the selected model. Update mappings when switching to avoid implicit unauthorized requests.",
                          "اربط الأسماء الافتراضية والوكلاء الفرعيين بالنموذج المختار وحدثها عند التبديل.",
                        )
                      : text(
                          "在工具的模型选择器中输入目录中的模型标识；同一密钥可切换已授权的指定模型与智能路由。",
                          "Enter a catalog model ID in the client selector. The same key can switch among authorized specific models and smart routing.",
                          "أدخل معرف النموذج في العميل. يمكن للمفتاح نفسه التبديل بين النماذج المصرح بها والتوجيه الذكي.",
                        )}
                  </li>
                  <li>
                    {text(
                      "回到平台做本地模拟验证；真实客户端需要另行接入真实网关才能完成连通性验证。",
                      "Use local demo validation here. Real client connectivity requires a separately implemented real gateway.",
                      "استخدم التحقق التجريبي المحلي هنا. يتطلب الاتصال الفعلي بوابة حقيقية منفصلة.",
                    )}
                  </li>
                </ol>
                {draft.target === "claude-code" && (
                  <a
                    href="https://code.claude.com/docs/en/llm-gateway"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      "官方网关配置说明",
                      "Official gateway configuration",
                      "دليل البوابة الرسمي",
                    )}
                  </a>
                )}
              </details>
            </>
          )}
        </section>
      </div>
      <section className="access-panel access-status">
        <ShieldCheck size={23} />
        <div>
          <strong>
            {verified
              ? text(
                  "已有对应的模拟成功请求",
                  "A matching simulated request succeeded",
                  "نجح طلب تجريبي مطابق",
                )
              : failed
                ? text(
                    "最近一次模拟验证失败",
                    "Latest simulated validation failed",
                    "فشل التحقق التجريبي الأخير",
                  )
                : text(
                    "等待对应请求验证",
                    "Awaiting a matching request",
                    "بانتظار طلب مطابق",
                  )}
          </strong>
          <p>
            {verified
              ? text(
                  `请求 ${verified.trace_id} 已在本地完成模拟验证。真实客户端连通性未验证。`,
                  `Request ${verified.trace_id} passed local simulation. Real client connectivity is unverified.`,
                  `نجح الطلب ${verified.trace_id} في المحاكاة المحلية. الاتصال الفعلي غير متحقق.`,
                )
              : text(
                  "复制配置、保存设置和打开工具均不会被标记为接入成功。只有同一密钥、模型和协议的成功记录才计为模拟验证通过。",
                  "Copying, saving, and opening a tool never count as connected. Only a successful record for this key, model, and protocol counts as simulated validation.",
                  "النسخ والحفظ وفتح الأداة لا تعني الاتصال. يلزم سجل ناجح لهذا المفتاح والنموذج والبروتوكول للتحقق التجريبي.",
                )}
          </p>
          {failed && !verified && (
            <p>
              {text(
                "最近一次模拟验证失败且未计费；请进入明细查看原因，配置选择已保留。",
                "The latest simulation failed without charge. Inspect the details; your configuration is retained.",
                "فشلت المحاكاة الأخيرة بلا رسوم. راجع التفاصيل؛ إعدادك محفوظ.",
              )}
            </p>
          )}
          <div className="access-actions">
            <Button asChild variant="secondary" size="small">
              <Link
                to={`/console/requests${verified || failed ? `?trace=${encodeURIComponent((verified ?? failed)!.trace_id)}` : ""}`}
              >
                {text("查看调用明细", "View requests", "عرض الطلبات")}
              </Link>
            </Button>
            <Button asChild variant="ghost" size="small">
              <Link to="/console/billing">
                {text("查看费用", "View billing", "عرض الفوترة")}
              </Link>
            </Button>
          </div>
        </div>
      </section>
    </div>
  );
}
