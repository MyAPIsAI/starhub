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
import { useTokenChoices } from "../lib/api/token-choices";
import { useSessionQueryIdentity } from "../lib/system-access";
import type { PreviewKey } from "../lib/product-preview";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useBackendRequests } from "../lib/api/usage";

import {
  buildClientConfiguration,
  clientProtocols,
  findClientModel,
  validDemoSecret,
  type ClientTarget,
  type ConnectionTool,
} from "../lib/client-config";

import { canonicalModelId } from "../lib/smart-model";
import { copyText } from "./code-example";
import "../components/access-center.css";

const DRAFT_KEY = "veyra-connect-draft-v1";
type Draft = {
  key: string;
  tool: ConnectionTool;
  target: ClientTarget;
  model: string;
};
function readDraft(storageKey: string): Partial<Draft> {
  try {
    const value = JSON.parse(sessionStorage.getItem(storageKey) ?? "{}");
    return typeof value === "object" && value ? value : {};
  } catch {
    return {};
  }
}
const isTool = (v: unknown): v is ConnectionTool =>
  ["openai", "claude-code", "codex", "cc-switch"].includes(String(v));
const isTarget = (v: unknown): v is ClientTarget =>
  ["openai", "claude-code", "codex"].includes(String(v));

export default function AccessCenter({
  plaintextKeys,
  revealedKeys,
  onReveal,
}: {
  plaintextKeys?: Record<string, string>;
  revealedKeys?: Record<string, number>;
  onReveal?: (keyId: string, password: string) => Promise<boolean>;
}) {
  const [params] = useSearchParams();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const { keys } = useBackendKeys(plaintextKeys);
  const [selectedKey, chooseKey] = useState("");
  const scope = useSessionQueryIdentity(),
    storageKey = DRAFT_KEY + JSON.stringify(scope),
    choicesQuery = useTokenChoices();
  const choices = choicesQuery.isError ? undefined : choicesQuery.data;
  const nativeModels = (
    key: PreviewKey,
    target?: ClientTarget,
  ): { id: string; name: string; kind: "specified" | "smart" }[] => {
    const protocol =
      target === undefined
        ? undefined
        : target === "codex"
          ? "OpenAI"
          : clientProtocols[target];
    const seen = new Set<string>();
    const result: { id: string; name: string; kind: "specified" | "smart" }[] =
      (choices?.choices ?? [])
        .filter(
          (c) =>
            c.group === key.tier &&
            (!protocol || c.protocols.includes(protocol)) &&
            (key.modelAccess === "all" || key.allowedModels?.includes(c.model)),
        )
        .filter((c) => {
          if (seen.has(c.model)) return false;
          seen.add(c.model);
          return true;
        })
        .map((c) => ({
          id: c.model,
          name: c.model,
          kind: "specified" as const,
        }));
    if (key.smartAccess && result.length)
      result.push({ id: "myapis-auto", name: "MyAPIs Auto", kind: "smart" });
    return result;
  };
  const requests = useBackendRequests(100).rows;
  const revealed = revealedKeys ?? {};
  const unlock = onReveal ?? (async () => false);
  const [draft, setDraft] = useState<Draft>(() => {
    const saved = readDraft(storageKey);
    const model =
      params.get("model") ??
      (typeof saved.model === "string" ? saved.model : "");
    const matching = keys.find(
      (key) =>
        key.active && (!model || findClientModel(nativeModels(key), model)),
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
        model || (foundKey ? nativeModels(foundKey, target)[0]?.id : "") || "",
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
    () => (key ? nativeModels(key, draft.target) : []),
    [key, draft.target, choices],
  );
  const selectedModel = findClientModel(models, draft.model);
  const unlocked = Boolean(
    key && validDemoSecret(key.keySecret) && (revealed[key.id] ?? 0) > clock,
  );
  const config =
    key?.active && selectedModel
      ? buildClientConfiguration({
          target: draft.target,
          model: selectedModel.id,
          apiBase:
            typeof window !== "undefined"
              ? `${window.location.origin}/v1`
              : undefined,
          secret: includeSecret && unlocked ? key.keySecret : undefined,
        })
      : null;
  const latestRequest =
    key?.active && selectedModel && protocol
      ? requests.find(
          (row) =>
            row.key_id === key?.id &&
            canonicalModelId(row.requested_model) ===
              canonicalModelId(draft.model) &&
            row.protocol === protocol,
        )
      : undefined;
  const verified =
    latestRequest?.status === "success" ? latestRequest : undefined;
  const failed =
    latestRequest && latestRequest.status !== "success"
      ? latestRequest
      : undefined;
  useEffect(() => {
    try {
      sessionStorage.setItem(storageKey, JSON.stringify(draft));
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
          {text("接入中心", "Connection center", "مركز الاتصال")}
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
            "复用一把 Key（密钥），获取当前网关配置，再在客户端切换指定模型与智能路由。使用完整密钥的验证请求会实际转发并按用量计费。",
            "Reuse one key, get this gateway’s configuration, then choose specific models or smart routing in your client. Verification with a full key sends a real request and incurs usage charges.",
            "استخدم مفتاحًا واحدًا لإعداد هذه البوابة واختيار النماذج في عميلك. التحقق بمفتاح كامل يرسل طلبًا فعليًا ويُحاسب حسب الاستخدام.",
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
                    findClientModel(
                      nativeModels(next, draft.target),
                      draft.model,
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
                  {item.name} · {item.tier}
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
                {key.tier} ·{" "}
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
                    {model.kind === "smart" ? "MyAPIs Auto" : model.name} ·{" "}
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
                  "下方使用当前网关地址；请确认客户端能访问此地址。页面内验证不能代替目标客户端的实际连通性测试。",
                  "Configuration uses this gateway’s address. Confirm your client can reach it; in-page verification does not prove connectivity from that client.",
                  "يستخدم الإعداد عنوان هذه البوابة. تأكد أن عميلك يستطيع الوصول إليه؛ تحقق الصفحة لا يثبت اتصال العميل.",
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
                          "已解锁的完整密钥",
                          "Revealed full credential",
                          "مفتاح كامل مكشوف",
                        )
                      : text(
                          "环境变量 MYAPIS_API_KEY（密钥占位）",
                          "Environment variable MYAPIS_API_KEY",
                          "متغير البيئة MYAPIS_API_KEY",
                        )}
                  </dd>
                </div>
              </dl>
              {key && !unlocked && (
                <details className="access-details">
                  <summary>
                    {text(
                      "需要完整密钥？解锁后加入配置",
                      "Need the full credential? Unlock to include it",
                      "تحتاج المفتاح الكامل؟ افتحه لإضافته",
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
                        "使用当前平台账号密码。仅本次创建或重置后仍在内存中的密钥可解锁；旧密钥请使用已保存的原文，或返回密钥页重置。",
                        "Use your current account password. Only a key created or reset in this session can be revealed; use your saved original key or reset it on the keys page.",
                        "استخدم كلمة مرور حسابك الحالية. يمكن كشف مفتاح أُنشئ أو أُعيد تعيينه في هذه الجلسة فقط؛ استخدم نسختك المحفوظة أو أعد تعيينه.",
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
                      disabled={unlocking || !validDemoSecret(key.keySecret)}
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
                        "解锁完整密钥",
                        "Reveal full key",
                        "إظهار المفتاح الكامل",
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
                      "在导出配置中包含已解锁的完整密钥",
                      "Include the revealed full credential in exported configuration",
                      "تضمين المفتاح الكامل المكشوف في الإعداد",
                    )}
                  </span>
                </label>
              )}
              <pre className="access-code">
                <code>{config.snippet}</code>
              </pre>
              <p className="access-muted">
                {text(
                  "未解锁时仅引用环境变量；请先赋予它完整密钥。通用工具的模型环境变量只是配置提示，应在工具的模型输入框中填写上方标识。",
                  "When locked, the snippet references an environment variable; set it to the full credential first. Generic model environment variables are guidance; enter the model ID in your tool’s model field.",
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
                      "填写基础地址、完整密钥和模型标识。不要填写掩码或把环境变量占位当作密钥原文。",
                      "Enter the base URL, full credential, and model ID. Do not enter a masked key or a literal environment placeholder.",
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
                      "回到平台使用完整密钥验证网关，再在目标客户端发起请求并核对调用日志。",
                      "Verify the gateway here with a full key, then send a request from your target client and check the request logs.",
                      "تحقق من البوابة هنا بمفتاح كامل، ثم أرسل طلبًا من عميلك وراجع سجل الطلبات.",
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
                  "已有同协议成功调用",
                  "A matching API request succeeded",
                  "نجح طلب واجهة مطابق",
                )
              : failed
                ? text(
                    "最近一次对应调用未成功",
                    "Latest matching request did not succeed",
                    "لم ينجح آخر طلب مطابق",
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
                  `最近 100 条日志中，请求 ${verified.trace_id} 已通过同密钥、模型和协议检查；此记录不证明所选客户端已接通。`,
                  `Within the latest 100 logs, request ${verified.trace_id} matches this key, model and protocol. It does not prove connectivity of the selected client.`,
                  `ضمن آخر 100 سجل، يطابق الطلب ${verified.trace_id} المفتاح والنموذج والبروتوكول. لا يثبت اتصال العميل المحدد.`,
                )
              : text(
                  "仅检查最近 100 条日志。复制配置、保存设置和打开工具不算调用成功；缺少协议信息的历史日志不会被推断为验证通过。",
                  "Checks only the latest 100 logs. Copying or saving does not count as success. Historical logs without protocol metadata cannot verify this configuration.",
                  "يفحص آخر 100 سجل فقط. النسخ والحفظ لا يعنيان النجاح، ولا تؤكد السجلات القديمة دون بروتوكول هذا الإعداد.",
                )}
          </p>
          {failed && !verified && (
            <p>
              {text(
                "最近一次对应调用失败或取消；请在明细中查看原因与实际费用，配置选择已保留。",
                "The latest matching call failed or was cancelled. Inspect its details and actual charge; your configuration is retained.",
                "فشل آخر طلب مطابق أو أُلغي. راجع التفاصيل والرسوم الفعلية؛ إعدادك محفوظ.",
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
