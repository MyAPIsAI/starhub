import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  Copy,
  Play,
  Terminal,
  SlidersHorizontal,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Field, Input } from "../components/ui/primitives";
import { TierBadge } from "../components/tier-badge";
import type { PreviewRequest } from "../lib/product-preview";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useTokenChoices } from "../lib/api/token-choices";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { resolveVendor } from "../lib/api/usage";
import { findClientModel } from "../lib/client-config";
import {
  usePlaygroundGrant,
  unlockPlaygroundKey,
  sendPlaygroundRequest,
} from "../lib/api/playground";
import { readGeminiGatewayResponse } from "../lib/gemini-response";
import {
  buildCurlRequest,
  type DemoRequest,
  type DemoProtocol,
} from "../lib/request-contract";
import { copyText } from "./code-example";
import "../components/request-playground.css";

function readDraft(draftKey: string | null) {
  if (!draftKey) return {};
  try {
    return JSON.parse(sessionStorage.getItem(draftKey) || "{}");
  } catch {
    return {};
  }
}
export default function Playground({
  onCreate,
  initialModel,
  onRelayComplete,
}: {
  onCreate: () => void;
  initialModel?: string;
  onRelayComplete?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const identity = useSessionQueryIdentity();
  const scope = JSON.stringify(identity);
  const owner = identity[1];
  const draftKey =
    owner !== null && Number.isSafeInteger(owner) && owner > 0
      ? "veyra-request-draft-v4:" + owner
      : null;
  const [draft] = useState(() => readDraft(draftKey));
  const { keys, loading: keysLoading, error: keysError } = useBackendKeys();
  const choicesQuery = useTokenChoices();
  const [keyId, setKeyId] = useState(params.get("key") || draft.keyId || "");
  const [model, setModel] = useState(
    params.get("model") || draft.model || initialModel || "",
  );
  const [protocol, setProtocol] = useState<DemoProtocol>(
    (params.get("protocol") || draft.protocol || "OpenAI") as DemoProtocol,
  );
  const [prompt, setPrompt] = useState<string>(
    draft.prompt || "请把以下任务拆成三个步骤：为团队建立多模型接入流程。",
  );
  const [system, setSystem] = useState<string>(draft.system || "");
  const [temperature, setTemperature] = useState<number>(
    draft.temperature ?? 0.7,
  );
  const [maxTokens, setMaxTokens] = useState<number>(draft.maxTokens || 512);
  const [capability, setCapability] = useState<
    NonNullable<DemoRequest["capability"]>
  >(draft.capability || "text");
  const [chainId, setChainId] = useState<string>(draft.chainId || "");
  const [sessionId] = useState(
    () =>
      draft.sessionId ||
      `playground-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
  );
  const [busy, setBusy] = useState(false);
  const [password, setPassword] = useState("");
  const [unlocking, setUnlocking] = useState(false);
  const [unlockError, setUnlockError] = useState("");
  const [relayResult, setRelayResult] = useState<PreviewRequest | null>(null);
  const last = relayResult;
  const operation = useRef(0);
  const pending = useRef(false);
  const controller = useRef<AbortController>();
  const currentKey = keys.find((k) => k.id === keyId);
  const options: { id: string; name: string; kind: "specified" | "smart" }[] =
    [];
  if (currentKey && !choicesQuery.isError) {
    for (const choice of choicesQuery.data?.choices ?? []) {
      if (
        choice.group === currentKey.tier &&
        choice.protocols.includes(protocol) &&
        (currentKey.modelAccess === "all" ||
          currentKey.allowedModels?.includes(choice.model)) &&
        !options.some((option) => option.id === choice.model)
      )
        options.push({
          id: choice.model,
          name: choice.model,
          kind: "specified",
        });
    }
    if (currentKey.smartAccess && options.length)
      options.push({ id: "myapis-auto", name: "MyAPIs Auto", kind: "smart" });
  }
  const selectedModel = findClientModel(options, model);
  const grant = usePlaygroundGrant(
    currentKey ? Number(currentKey.id) : undefined,
    currentKey?.managementRevision,
  );
  const unlocked =
    currentKey?.active && !grant.isError && grant.data?.unlocked === true;
  const isCurrentSession = () =>
    scope === JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()));
  useEffect(() => {
    if (!keyId && keys.length)
      setKeyId((keys.find((key) => key.active) ?? keys[0]).id);
    if (!model && options.length) setModel(options[0].id);
  }, [keyId, model, keys, options]);
  useEffect(() => {
    operation.current++;
    setPassword("");
    setUnlockError("");
    setUnlocking(false);
  }, [keyId]);
  const request: DemoRequest = {
    model,
    messages: [
      ...(system ? [{ role: "system" as const, content: system }] : []),
      { role: "user", content: prompt },
    ],
    temperature,
    max_tokens: maxTokens,
    protocol,
    capability,
    ...(chainId ? { chainId } : {}),
    sessionId,
  };
  const code = buildCurlRequest(
    request,
    typeof window !== "undefined" ? `${window.location.origin}/v1` : undefined,
  );
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    setCopied(false);
  }, [code]);
  useEffect(() => {
    if (!draftKey || !isCurrentSession()) return;
    try {
      sessionStorage.setItem(
        draftKey,
        JSON.stringify({
          keyId,
          model,
          protocol,
          prompt,
          system,
          temperature,
          maxTokens,
          capability,
          chainId,
          sessionId,
        }),
      );
    } catch {}
  }, [
    keyId,
    model,
    protocol,
    prompt,
    system,
    temperature,
    maxTokens,
    capability,
    chainId,
    sessionId,
    draftKey,
  ]);
  useEffect(
    () => () => {
      operation.current++;
      controller.current?.abort();
    },
    [],
  );
  const unlock = async (event: React.FormEvent) => {
    event.preventDefault();
    if (
      !currentKey?.active ||
      !password ||
      unlocking ||
      pending.current ||
      !isCurrentSession()
    )
      return;
    const attempt = ++operation.current;
    const input = password;
    setPassword("");
    setUnlocking(true);
    setUnlockError("");
    try {
      await unlockPlaygroundKey(Number(currentKey.id), input, identity);
      const confirmed = (await grant.refetch()).data?.unlocked === true;
      if (attempt !== operation.current || !isCurrentSession()) return;
      if (!confirmed)
        setUnlockError(
          c(
            "验证失败，请检查当前账号密码、登录状态及密钥状态后重试。",
            "Verification failed. Check your account password, session and key status, then retry.",
            "فشل التحقق. تحقق من كلمة مرور الحساب والجلسة وحالة المفتاح ثم أعد المحاولة.",
          ),
        );
    } catch {
      if (attempt === operation.current && isCurrentSession())
        setUnlockError(
          c(
            "暂时无法解锁密钥，请重试。",
            "The key could not be unlocked. Please retry.",
            "تعذر فتح المفتاح. أعد المحاولة.",
          ),
        );
    } finally {
      if (attempt === operation.current && isCurrentSession())
        setUnlocking(false);
    }
  };
  const send = async () => {
    if (
      !currentKey?.active ||
      !prompt.trim() ||
      !model ||
      !unlocked ||
      pending.current ||
      !isCurrentSession()
    )
      return;
    pending.current = true;
    setBusy(true);
    setRelayResult(null);
    const snapshot = structuredClone(request);
    const credential = currentKey.id;
    const attempt = ++operation.current;
    const started = performance.now();
    const trace = `relay_${Date.now()}`;
    controller.current = new AbortController();
    const active = () => attempt === operation.current && isCurrentSession();
    try {
      const res = await sendPlaygroundRequest(
        Number(credential),
        snapshot,
        identity,
        controller.current.signal,
      );
      const data = await res.json();
      if (!active()) return;
      if (!res.ok) {
        const errMsg =
          data?.error?.message ??
          (data?.message === "playground_unlock_required"
            ? c(
                "密钥需要重新解锁，请验证当前账号密码。",
                "Unlock this key again with your current account password.",
                "افتح المفتاح مجددًا بكلمة مرور حسابك الحالية.",
              )
            : data?.message) ??
          `HTTP ${res.status}`;
        setRelayResult({
          source: "api",
          trace_id: trace,
          time: new Date().toISOString(),
          model: snapshot.model,
          vendor: resolveVendor(snapshot.model),
          status: "failed",
          cost_nano: "0",
          duration_ms: Math.round(performance.now() - started),
          key_id: credential,
          input_tokens: 0,
          output_tokens: 0,
          error_code: data?.error?.code ?? `http_${res.status}`,
          error_message: errMsg,
          requested_model: snapshot.model,
          response_text: "",
        } as PreviewRequest);
        return;
      }
      const gemini =
        snapshot.protocol === "Gemini"
          ? readGeminiGatewayResponse(data)
          : undefined;
      const content = gemini
        ? gemini.content
        : snapshot.protocol === "Anthropic"
          ? (data?.content ?? [])
              .filter((block: { type: string }) => block.type === "text")
              .map((block: { text: string }) => block.text)
              .join("")
          : (data?.choices?.[0]?.message?.content ?? "");
      const inputTokens =
        gemini?.inputTokens ??
        data?.usage?.prompt_tokens ??
        data?.usage?.input_tokens ??
        0;
      const outputTokens =
        gemini?.outputTokens ??
        data?.usage?.completion_tokens ??
        data?.usage?.output_tokens ??
        0;
      setRelayResult({
        source: "api",
        trace_id: trace,
        time: new Date().toISOString(),
        model: snapshot.model,
        vendor: resolveVendor(snapshot.model),
        status: gemini?.status ?? "success",
        cost_nano: "0",
        duration_ms: Math.round(performance.now() - started),
        key_id: credential,
        input_tokens: inputTokens,
        output_tokens: outputTokens,
        error_code: gemini?.errorCode ?? "",
        error_message: gemini?.errorCode ? `Gemini: ${gemini.errorCode}` : "",
        requested_model: snapshot.model,
        actual_model: gemini
          ? gemini.actualModel
          : (data?.model ?? snapshot.model),
        protocol: snapshot.protocol,
        response_text: content,
      } as PreviewRequest);
    } catch {
      if (!active()) return;
      // A real request may have executed; never retry it as a simulation.
      setRelayResult({
        source: "api",
        trace_id: trace,
        time: new Date().toISOString(),
        model: snapshot.model,
        requested_model: snapshot.model,
        status: "failed",
        cost_nano: "0",
        key_id: credential,
        vendor: resolveVendor(snapshot.model),
        duration_ms: Math.round(performance.now() - started),
        input_tokens: 0,
        output_tokens: 0,
        error_code: "network_error",
        error_message: c(
          "调用结果未知，请先查看调用日志再决定是否重试。",
          "Request outcome unknown. Check request logs before retrying.",
          "نتيجة الطلب غير معروفة. راجع السجلات قبل إعادة المحاولة.",
        ),
      } as PreviewRequest);
    } finally {
      pending.current = false;
      if (active()) {
        setBusy(false);
        void grant.refetch();
        onRelayComplete?.();
      }
    }
  };
  const error = last?.status === "failed";
  return (
    <div className="verify-workspace">
      <div className="verify-notice">
        <Terminal size={19} />
        <div>
          <strong>
            {c("真实接口调试", "API playground", "اختبار الواجهة")}
          </strong>
          <p>
            {c(
              "使用已创建的密钥发送真实网关请求，按实际用量计费。每把密钥验证一次账号密码，在当前登录会话内持续可用，刷新或切页无需重复验证；登出、会话过期、改密或密钥变更后需重新解锁。",
              "Send real gateway requests; usage charges apply. Unlock each key once per login session, including after refresh or navigation. Sign-out, session expiry, password changes or key changes require a new unlock.",
              "أرسل طلبات فعلية برسوم الاستخدام. افتح كل مفتاح مرة لكل جلسة دخول، حتى بعد التحديث والتنقل. يلزم فتحه مجددًا بعد الخروج أو انتهاء الجلسة أو تغيير كلمة المرور أو المفتاح.",
            )}
          </p>
        </div>
        <Link
          to={`/console/connect?key=${encodeURIComponent(keyId)}&model=${encodeURIComponent(model)}`}
        >
          {c("返回接入中心", "Back to connections", "العودة للاتصال")}
          <ArrowRight size={14} />
        </Link>
      </div>
      <div className="verify-columns">
        <section className="verify-card">
          <div className="verify-card-heading">
            <h2>{c("请求配置", "Request configuration", "إعداد الطلب")}</h2>
            {currentKey && <TierBadge id={currentKey.tier} />}
          </div>
          {keysLoading || keysError ? (
            <div className="verify-empty" role="status">
              <p>
                {keysError
                  ? c(
                      "无法读取密钥，请刷新页面重试。",
                      "Keys could not be loaded. Refresh to retry.",
                      "تعذر تحميل المفاتيح. حدّث الصفحة وأعد المحاولة.",
                    )
                  : c("正在读取密钥…", "Loading keys…", "جارٍ تحميل المفاتيح…")}
              </p>
            </div>
          ) : !keys.length ? (
            <div className="verify-empty">
              <h3>
                {c(
                  "先创建一把密钥",
                  "Create a key first",
                  "أنشئ مفتاحًا أولاً",
                )}
              </h3>
              <p>
                {c(
                  "可在接入中心选择工具，再用这里验证。",
                  "Choose your tool in the connection center, then validate here.",
                  "اختر أداتك في مركز الاتصال ثم تحقق هنا.",
                )}
              </p>
              <Button variant="primary" onClick={onCreate}>
                {c("创建密钥", "Create key", "إنشاء مفتاح")}
              </Button>
            </div>
          ) : (
            <>
              <Field id="verify-key" label={c("Key（密钥）", "Key", "المفتاح")}>
                <select
                  id="verify-key"
                  className="select-trigger"
                  value={keyId}
                  disabled={busy}
                  onChange={(e) => {
                    setKeyId(e.target.value);
                  }}
                >
                  {!currentKey && (
                    <option value={keyId}>
                      {c("请选择有效密钥", "Select a key", "اختر مفتاحًا")}
                    </option>
                  )}
                  {keys.map((k) => (
                    <option key={k.id} value={k.id}>
                      {k.name}
                      {k.active ? "" : c(" · 已停用", " · Disabled", " · معطل")}
                    </option>
                  ))}
                </select>
              </Field>
              {currentKey && grant.isPending && (
                <p role="status" className="verify-hint">
                  {c(
                    "正在确认解锁状态…",
                    "Checking key unlock…",
                    "جارٍ التحقق من فتح المفتاح…",
                  )}
                </p>
              )}
              {currentKey && !unlocked && !grant.isPending && (
                <form className="verify-unlock" onSubmit={unlock}>
                  <Field
                    id="verify-password"
                    label={c(
                      "当前账号密码",
                      "Current account password",
                      "كلمة مرور الحساب الحالية",
                    )}
                  >
                    <Input
                      id="verify-password"
                      type="password"
                      autoComplete="current-password"
                      value={password}
                      onChange={(event) => setPassword(event.target.value)}
                      disabled={busy || unlocking || !currentKey.active}
                      aria-invalid={Boolean(unlockError)}
                      aria-describedby={
                        unlockError ? "verify-unlock-error" : undefined
                      }
                      required
                    />
                  </Field>
                  <Button
                    type="submit"
                    disabled={
                      busy || unlocking || !password || !currentKey.active
                    }
                    aria-busy={unlocking}
                  >
                    {unlocking
                      ? c("正在验证…", "Verifying…", "جارٍ التحقق…")
                      : c("解锁密钥", "Unlock key", "فتح المفتاح")}
                  </Button>
                  {unlockError && (
                    <p
                      id="verify-unlock-error"
                      className="verify-unlock-error"
                      role="alert"
                    >
                      {unlockError}
                    </p>
                  )}
                </form>
              )}
              {choicesQuery.isError && (
                <p role="alert" className="verify-hint">
                  {c(
                    "无法读取可用模型，请刷新页面重试。",
                    "Available models could not be loaded. Refresh to retry.",
                    "تعذر تحميل النماذج المتاحة. حدّث الصفحة وأعد المحاولة.",
                  )}
                </p>
              )}
              <div className="verify-field-row">
                <Field
                  id="verify-protocol"
                  label={c("请求协议", "Protocol", "البروتوكول")}
                >
                  <select
                    id="verify-protocol"
                    className="select-trigger"
                    value={protocol}
                    onChange={(e) =>
                      setProtocol(e.target.value as DemoProtocol)
                    }
                  >
                    <option value="OpenAI">
                      {c(
                        "OpenAI（对话协议）",
                        "OpenAI (chat)",
                        "OpenAI (المحادثة)",
                      )}
                    </option>
                    <option value="Anthropic">
                      {c(
                        "Anthropic（消息协议）",
                        "Anthropic (messages)",
                        "Anthropic (الرسائل)",
                      )}
                    </option>
                    <option value="Gemini">
                      {c(
                        "Gemini（内容生成）",
                        "Gemini (content generation)",
                        "Gemini (إنشاء المحتوى)",
                      )}
                    </option>
                  </select>
                </Field>
                <Field
                  id="verify-model"
                  label={c("本次模型", "Request model", "نموذج الطلب")}
                >
                  <select
                    id="verify-model"
                    className="select-trigger"
                    value={selectedModel?.id ?? model}
                    onChange={(e) => setModel(e.target.value)}
                  >
                    {!selectedModel && (
                      <option value={model}>
                        {model} · {c("当前不可用", "Unavailable", "غير متاح")}
                      </option>
                    )}
                    {options.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.id} ·{" "}
                        {m.kind === "smart"
                          ? c("智能路由", "Smart routing", "توجيه ذكي")
                          : c("指定模型", "Specified model", "نموذج محدد")}
                      </option>
                    ))}
                  </select>
                </Field>
              </div>
              {selectedModel?.kind === "smart" && (
                <p className="verify-hint">
                  {c(
                    "智能路由按成本从已授权且兼容的候选中选择，按实际模型计费。分组倍率只计算一次。",
                    "Cost routing selects an authorized, compatible candidate and charges its rate once.",
                    "يختار التوجيه الأقل تكلفة من النماذج المتوافقة المسموحة ويحسب رسومه مرة واحدة.",
                  )}
                </p>
              )}
              <Field
                id="verify-prompt"
                label={c("请求内容", "Message", "محتوى الطلب")}
              >
                <textarea
                  id="verify-prompt"
                  className="input verify-prompt"
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  rows={7}
                />
              </Field>
              <div className="verify-presets">
                {[
                  [
                    c("任务拆解", "Plan a task", "تخطيط مهمة"),
                    "请把以下任务拆成三个步骤：为团队建立多模型接入流程。",
                  ],
                  [
                    c("费用计算", "Calculate cost", "حساب التكلفة"),
                    "请用代码表示：费用 = 输入词元 × 输入单价 + 输出词元 × 输出单价。",
                  ],
                  [
                    c("翻译示例", "Translation sample", "مثال ترجمة"),
                    "请将 Hello world 翻译为中文。",
                  ],
                ].map(([label, value]) => (
                  <button key={label} onClick={() => setPrompt(value)}>
                    {label}
                  </button>
                ))}
              </div>
              <details className="verify-advanced">
                <summary>
                  <SlidersHorizontal size={14} />
                  {c(
                    "请求参数与工具链",
                    "Parameters and tool chain",
                    "المعاملات وسلسلة الأدوات",
                  )}
                </summary>
                <div className="verify-field-row">
                  <Field
                    id="verify-temp"
                    label={c(
                      "Temperature（随机性）",
                      "Temperature",
                      "العشوائية",
                    )}
                  >
                    <Input
                      id="verify-temp"
                      type="number"
                      min="0"
                      max="2"
                      step="0.1"
                      value={temperature}
                      onChange={(e) => setTemperature(Number(e.target.value))}
                    />
                  </Field>
                  <Field
                    id="verify-max"
                    label={c(
                      "最大输出词元",
                      "Max output tokens",
                      "الحد الأقصى للرموز",
                    )}
                  >
                    <Input
                      id="verify-max"
                      type="number"
                      min="1"
                      max="16384"
                      value={maxTokens}
                      onChange={(e) => setMaxTokens(Number(e.target.value))}
                    />
                  </Field>
                </div>
                <Field
                  id="verify-system"
                  label={c("系统指令", "System instruction", "تعليمات النظام")}
                >
                  <textarea
                    id="verify-system"
                    className="input"
                    rows={2}
                    value={system}
                    onChange={(e) => setSystem(e.target.value)}
                  />
                </Field>
                <div className="verify-field-row">
                  <Field
                    id="verify-capability"
                    label={c(
                      "所需能力",
                      "Required capability",
                      "القدرة المطلوبة",
                    )}
                  >
                    <select
                      id="verify-capability"
                      className="select-trigger"
                      value={capability}
                      onChange={(e) =>
                        setCapability(e.target.value as typeof capability)
                      }
                    >
                      <option value="text">{c("文本", "Text", "نص")}</option>
                      <option value="code">{c("代码", "Code", "برمجة")}</option>
                      <option value="vision">
                        {c("视觉", "Vision", "رؤية")}
                      </option>
                      <option value="reasoning">
                        {c("推理", "Reasoning", "استدلال")}
                      </option>
                    </select>
                  </Field>
                  <Field
                    id="verify-chain"
                    label={c(
                      "工具链编号（选填）",
                      "Tool chain ID (optional)",
                      "معرّف سلسلة الأدوات",
                    )}
                  >
                    <Input
                      id="verify-chain"
                      value={chainId}
                      onChange={(e) => setChainId(e.target.value)}
                      placeholder={c(
                        "同编号保持实际模型",
                        "Pin actual model per ID",
                        "يثبت النموذج حسب المعرّف",
                      )}
                    />
                  </Field>
                </div>
                <Field
                  id="verify-custom-model"
                  label={c(
                    "自定义模型标识（验证错误用）",
                    "Custom model ID (error checks)",
                    "معرّف نموذج مخصص",
                  )}
                >
                  <Input
                    id="verify-custom-model"
                    value={model}
                    onChange={(e) => setModel(e.target.value)}
                  />
                </Field>
              </details>
              <div className="verify-send">
                <span>
                  {!unlocked
                    ? c(
                        "请先解锁密钥，再发送请求",
                        "Unlock the key before sending",
                        "افتح المفتاح قبل الإرسال",
                      )
                    : c(
                        "当前登录会话已解锁 · 实际调用会计费",
                        "Unlocked for this session · usage charges apply",
                        "مفتوح لهذه الجلسة · تُطبق رسوم الاستخدام",
                      )}
                </span>
                <Button
                  variant="primary"
                  disabled={
                    busy ||
                    !currentKey?.active ||
                    !prompt.trim() ||
                    !model ||
                    !unlocked
                  }
                  onClick={send}
                >
                  <Play size={14} />
                  {busy
                    ? c("正在发送…", "Sending…", "جارٍ الإرسال…")
                    : c("发送请求", "Send request", "إرسال الطلب")}
                </Button>
              </div>
            </>
          )}
        </section>
        <section className="verify-card verify-result">
          <div className="verify-card-heading">
            <h2>{c("验证结果", "Verification result", "نتيجة التحقق")}</h2>
            <span className="verify-demo-label">
              {c("接口调用", "API CALL", "استدعاء")}
            </span>
          </div>
          {last ? (
            <>
              <div
                role="status"
                className={`verify-outcome ${error ? "is-error" : ""}`}
              >
                {error ? <CircleAlert size={21} /> : <CheckCircle2 size={21} />}
                <div>
                  <strong>
                    {error
                      ? c(
                          "接口调用未成功",
                          "API call did not succeed",
                          "لم ينجح استدعاء الواجهة",
                        )
                      : c(
                          "接口调用成功 · 费用以调用日志为准",
                          "API call succeeded · see logs for charges",
                          "نجح الاستدعاء · راجع الرسوم في السجلات",
                        )}
                  </strong>
                  <p>
                    {error
                      ? last.error_message
                      : c(
                          "此处仅显示接口响应。实际结算状态和费用请查看调用日志。",
                          "This shows the API response only. Check request logs for settlement and charges.",
                          "تظهر هنا استجابة الواجهة فقط. راجع السجلات للتسوية والرسوم.",
                        )}
                  </p>
                  {error && <p>{last.recovery_action}</p>}
                </div>
              </div>
              <dl className="verify-metrics">
                <div>
                  <dt>{c("请求模型", "Requested model", "النموذج المطلوب")}</dt>
                  <dd>{last.requested_model || last.model}</dd>
                </div>
                <div>
                  <dt>{c("实际模型", "Actual model", "النموذج الفعلي")}</dt>
                  <dd>{last.actual_model || "—"}</dd>
                </div>
                <div>
                  <dt>
                    {c(
                      "输入 / 输出词元",
                      "Input / output tokens",
                      "رموز الإدخال / الإخراج",
                    )}
                  </dt>
                  <dd>
                    {last.input_tokens} / {last.output_tokens}
                  </dd>
                </div>
                <div>
                  <dt>{c("本次费用", "Charge", "الرسوم")}</dt>
                  <dd>
                    <Link to="/console/requests">
                      {c(
                        "查看调用日志",
                        "View request logs",
                        "عرض سجلات الطلبات",
                      )}
                    </Link>
                  </dd>
                </div>
              </dl>
              {last.response_text && (
                <pre className="verify-output">{last.response_text}</pre>
              )}
              {!error && !last.response_text && (
                <p className="verify-hint">
                  {c(
                    "接口未返回文本内容，请查看调用日志确认状态与用量。",
                    "The API returned no text. Check request logs for status and usage.",
                    "لم تُرجع الواجهة نصًا. راجع السجلات للحالة والاستخدام.",
                  )}
                </p>
              )}
              <div className="verify-result-actions">
                <Button asChild>
                  <Link to="/console/requests">
                    {c(
                      "查看调用日志",
                      "View request logs",
                      "عرض سجلات الطلبات",
                    )}
                    <ArrowRight size={14} />
                  </Link>
                </Button>
                {error && (
                  <>
                    <Link to="/console/billing/recharge">
                      {c("充值余额", "Add balance", "إضافة رصيد")}
                    </Link>
                    <Link to="/console/keys">
                      {c(
                        "检查密钥与额度",
                        "Check key and quota",
                        "فحص المفتاح والحصة",
                      )}
                    </Link>
                  </>
                )}
              </div>
              <small className="verify-trace" dir="ltr">
                {last.trace_id}
              </small>
            </>
          ) : (
            <div className="verify-empty">
              <Terminal size={32} />
              <h3>
                {c("等待第一次验证", "Ready for verification", "جاهز للتحقق")}
              </h3>
              <p>
                {c(
                  "选择模型并发送请求。你也可以先去客户端使用，再回来查看明细。",
                  "Select a model and send a request, or use your client first and return for records.",
                  "اختر نموذجًا وأرسل طلبًا أو استخدم العميل أولاً ثم راجع السجلات.",
                )}
              </p>
            </div>
          )}
          <details className="verify-code" open>
            <summary>
              {c(
                "导出当前请求",
                "Export current request",
                "تصدير الطلب الحالي",
              )}
            </summary>
            <p className="verify-hint">
              {c(
                "使用当前网关地址与环境变量 MYAPIS_API_KEY（平台密钥）；执行会产生实际调用费用。Gemini 使用原生 generateContent（内容生成）接口。",
                "Uses this gateway and MYAPIS_API_KEY. Running it incurs usage charges. Gemini uses native generateContent.",
                "يستخدم هذه البوابة ومتغير MYAPIS_API_KEY. التنفيذ يُحاسب حسب الاستخدام. يستخدم Gemini واجهة generateContent الأصلية.",
              )}
            </p>
            <Button
              size="small"
              onClick={async () => {
                if (
                  await copyText(
                    code,
                    c(
                      "配置已复制，尚未验证接通",
                      "Configuration copied, connection unverified",
                      "تم النسخ دون تأكيد الاتصال",
                    ),
                    t("copyFailed"),
                  )
                )
                  setCopied(true);
              }}
            >
              <Copy size={13} />
              {copied
                ? c("已复制配置", "Configuration copied", "تم نسخ الإعداد")
                : c("复制命令", "Copy command", "نسخ الأمر")}
            </Button>
            <pre dir="ltr" tabIndex={0}>
              <code>{code}</code>
            </pre>
          </details>
        </section>
      </div>
    </div>
  );
}
