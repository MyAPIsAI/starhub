import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ArrowRight, CircleAlert, Copy, Play, Terminal } from "lucide-react";
import { Button } from "../components/ui/button";
import { Field, Input, Segmented } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import { TierBadge } from "../components/tier-badge";
import type { PreviewRequest } from "../lib/product-preview";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useTokenChoices } from "../lib/api/token-choices";
import {
  currentKeyGrant,
  revealTokenSecret,
  verifyTokenPassword,
} from "../lib/api/tokens";
import { apiGet, ApiError } from "../lib/api/http";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { buildGatewayRequest } from "../lib/gateway-request";
import {
  playgroundModels,
  selectedPlaygroundModel,
  playgroundExamples,
  readPlaygroundResponse,
  type PlaygroundResult,
} from "../lib/playground";
import type { DemoRequest, DemoProtocol } from "../lib/request-contract";
import { formatMoney } from "../lib/money";
import { copyText } from "./code-example";
import "../components/request-playground.css";

const draftKey = "veyra-request-draft-v2";
function readDraft() {
  try {
    return JSON.parse(sessionStorage.getItem(draftKey) || "{}");
  } catch {
    return {};
  }
}

export default function Playground({
  onCreate,
  onRelayComplete,
}: {
  onRecord: (record: PreviewRequest) => void;
  onCreate: () => void;
  initialModel?: string;
  onRelayComplete?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const [draft] = useState(readDraft);
  const {
    keys,
    loading,
    error: keysError,
    refetch: refetchKeys,
  } = useBackendKeys();
  const choices = useTokenChoices();
  const scope = useSessionQueryIdentity();
  const scopeKey = JSON.stringify(scope);
  const [now, setNow] = useState(Date.now);
  const validKeys = keys.filter(
    (key) =>
      key.active && (!key.validUntil || Date.parse(key.validUntil) > now),
  );
  const [keyId, setKeyId] = useState<string>(
    params.get("key") || draft.keyId || "",
  );
  const [model, setModel] = useState<string>(
    params.get("model") || draft.model || "",
  );
  const [protocol, setProtocol] = useState<DemoProtocol>(
    (params.get("protocol") || draft.protocol || "OpenAI") as DemoProtocol,
  );
  const [prompt, setPrompt] = useState<string>(
    draft.prompt ||
      c(
        "请简要说明如何开始使用 MyAPIs。",
        "Briefly explain how to get started with MyAPIs.",
        "اشرح بإيجاز كيفية بدء استخدام MyAPIs.",
      ),
  );
  const [system, setSystem] = useState<string>(draft.system || "");
  const [temperature, setTemperature] = useState<number>(
    draft.temperature ?? 0.7,
  );
  const [maxTokens, setMaxTokens] = useState<number>(draft.maxTokens || 512);
  const [stream, setStream] = useState<boolean>(draft.stream ?? true);
  const [busy, setBusy] = useState(false);
  const [output, setOutput] = useState("");
  const [result, setResult] = useState<PlaygroundResult | null>(null);
  const [trace, setTrace] = useState("");
  const [error, setError] = useState("");
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const passwordInput = useRef<HTMLInputElement>(null);
  const pending = useRef(false);
  const controller = useRef<AbortController | null>(null);
  const alive = useRef(true);
  const [language, setLanguage] = useState("curl");
  const [copied, setCopied] = useState(false);
  const currentKey = validKeys.find((key) => key.id === keyId);
  const options = playgroundModels(
    currentKey,
    choices.isError ? undefined : choices.data,
    protocol,
  );
  const selectedModel = selectedPlaygroundModel(options, model);
  const request: DemoRequest = {
    model,
    messages: [
      ...(system ? [{ role: "system" as const, content: system }] : []),
      { role: "user", content: prompt },
    ],
    temperature,
    max_tokens: maxTokens,
    protocol,
    stream,
  };
  const examples = playgroundExamples(
    request,
    typeof window === "undefined"
      ? "https://api.myapis.example"
      : window.location.origin,
  );
  const code = examples[language as keyof typeof examples];
  const validParameters =
    Number.isFinite(temperature) &&
    temperature >= 0 &&
    temperature <= 2 &&
    Number.isSafeInteger(maxTokens) &&
    maxTokens >= 1 &&
    maxTokens <= 16384;
  const canSend =
    !!currentKey &&
    !!selectedModel &&
    !choices.isError &&
    !keysError &&
    !!prompt.trim() &&
    validParameters;
  useEffect(() => {
    alive.current = true;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      alive.current = false;
      clearInterval(timer);
      controller.current?.abort();
    };
  }, []);
  useEffect(() => {
    controller.current?.abort();
    setPassword("");
    setPasswordOpen(false);
    setOutput("");
    setResult(null);
    setTrace("");
    setError("");
  }, [scopeKey]);
  useEffect(() => {
    if (!keyId && validKeys.length) setKeyId(validKeys[0].id);
  }, [keyId, validKeys]);
  useEffect(() => {
    if (!model && options.length) setModel(options[0].id);
  }, [model, options]);
  useEffect(() => {
    setCopied(false);
  }, [code]);
  useEffect(() => {
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
          stream,
        }),
      );
    } catch {}
  }, [keyId, model, protocol, prompt, system, temperature, maxTokens, stream]);

  const execute = async (loginPassword?: string) => {
    if (!canSend || pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    setPasswordError("");
    const id = Number(keyId),
      snapshot = structuredClone(request),
      identity = scope;
    const abort = new AbortController();
    controller.current = abort;
    const unchanged = () =>
      alive.current &&
      JSON.stringify(sessionQueryIdentity(useSystemAccess.getState())) ===
        JSON.stringify(identity) &&
      !abort.signal.aborted;
    let sent = false;
    try {
      let grant = currentKeyGrant(id, identity);
      let secret: string;
      if (loginPassword !== undefined) {
        const unlocked = await verifyTokenPassword(id, loginPassword, identity);
        grant = unlocked.grant;
        secret = unlocked.key;
      } else {
        if (!grant) {
          setPasswordOpen(true);
          return;
        }
        const unlocked = await revealTokenSecret(id, grant, identity);
        secret = unlocked.key;
      }
      if (!unchanged()) return;
      setPassword("");
      setPasswordOpen(false);
      setOutput("");
      setResult(null);
      setTrace("");
      const call = buildGatewayRequest(snapshot, secret);
      const headers = {
        ...call.init.headers,
        "X-MyAPIs-Source": "playground",
        "X-StarHub-Portal": "workspace",
        "X-MyAPIs-Key-ID": String(id),
        "X-MyAPIs-Key-Grant": grant!,
      };
      secret = "";
      sent = true;
      const response = await fetch(call.endpoint, {
        ...call.init,
        headers,
        credentials: "same-origin",
        signal: abort.signal,
      });
      const requestTrace = response.headers.get("X-Request-ID") || "";
      if (unchanged()) setTrace(requestTrace);
      let responseError: unknown;
      try {
        await readPlaygroundResponse(response, protocol, (text) => {
          if (unchanged()) setOutput(text);
        });
      } catch (err) {
        responseError = err;
      }
      if (requestTrace && unchanged()) {
        // Stream completion can precede the atomic settlement by a few milliseconds.
        for (let attempt = 0; attempt < 20; attempt++) {
          try {
            const metadata = await apiGet<PlaygroundResult>(
              `/api/playground/results/${encodeURIComponent(requestTrace)}`,
              abort.signal,
            );
            if (
              unchanged() &&
              metadata.key_id === id &&
              metadata.trace_id === requestTrace
            )
              setResult(metadata);
            break;
          } catch (err) {
            if (
              responseError &&
              err instanceof ApiError &&
              err.status === 404 &&
              attempt === 19
            )
              break;
            if (
              !(err instanceof ApiError && err.status === 404) ||
              attempt === 19
            )
              throw err;
            await new Promise((resolve) => setTimeout(resolve, 100));
          }
        }
      }
      if (responseError) throw responseError;
      if (!requestTrace)
        throw new Error(
          c(
            "缺少调用标识，请查看调用明细。",
            "Request trace unavailable. Check request history.",
            "معرّف الطلب غير متاح. راجع السجل.",
          ),
        );
    } catch (err) {
      if (!unchanged()) return;
      if (!sent) {
        setPassword("");
        setPasswordOpen(true);
        setPasswordError(
          c(
            "无法解锁所选密钥，请验证登录密码。",
            "Unable to unlock this key. Verify your login password.",
            "تعذر فتح المفتاح. تحقق من كلمة مرور الدخول.",
          ),
        );
      } else
        setError(
          err instanceof Error
            ? err.message
            : c(
                "调用结果未知，请查看调用明细后再决定是否重试。",
                "Request outcome unknown. Check history before retrying.",
                "نتيجة الطلب غير معروفة. راجع السجل قبل إعادة المحاولة.",
              ),
        );
    } finally {
      pending.current = false;
      if (alive.current) setBusy(false);
      if (sent && unchanged()) onRelayComplete?.();
    }
  };
  const send = () => {
    if (!canSend || pending.current) return;
    if (!currentKeyGrant(Number(keyId), scope)) {
      setPasswordError("");
      setPasswordOpen(true);
    } else void execute();
  };
  return (
    <div className="verify-workspace">
      <div className="verify-notice">
        <Terminal size={19} />
        <div>
          <strong>
            {c("文本对话调试", "Text chat playground", "تجربة المحادثة النصية")}
          </strong>
          <p>
            {c(
              "通过所选 Key 直接调用网关，按实际用量计费；IP 白名单照常生效。密钥仅在页面内存中使用，对话草稿保存在当前标签页。",
              "Calls the gateway with your selected key and charges actual usage. IP restrictions apply. Credentials stay in page memory; drafts stay in this tab.",
              "يستدعي البوابة بالمفتاح المختار ويحاسب الاستخدام الفعلي. تنطبق قيود IP. تبقى بيانات المفتاح في ذاكرة الصفحة والمسودات في علامة التبويب.",
            )}
          </p>
        </div>
        <Link
          to={`/console/connect?key=${encodeURIComponent(keyId)}&model=${encodeURIComponent(model)}`}
        >
          {c("接入中心", "Connections", "الاتصال")}
          <ArrowRight size={14} />
        </Link>
      </div>
      <div className="verify-columns">
        <section className="verify-card">
          <div className="verify-card-heading">
            <h2>{c("请求配置", "Request configuration", "إعداد الطلب")}</h2>
            {currentKey && <TierBadge id={currentKey.tier} />}
          </div>
          {(keysError || choices.isError) && (
            <p role="alert">
              {c(
                "无法加载密钥或模型，请重试。",
                "Unable to load keys or models. Retry.",
                "تعذر تحميل المفاتيح أو النماذج. أعد المحاولة.",
              )}{" "}
              <Button
                onClick={() => {
                  void choices.refetch();
                  void refetchKeys();
                }}
              >
                {c("重试", "Retry", "إعادة المحاولة")}
              </Button>
            </p>
          )}
          {loading && (
            <p role="status">
              {c("正在加载密钥…", "Loading keys…", "جارٍ تحميل المفاتيح…")}
            </p>
          )}
          {!loading && !validKeys.length && (
            <div className="verify-empty">
              <h3>
                {c("暂无有效密钥", "No active keys", "لا توجد مفاتيح صالحة")}
              </h3>
              <p>
                {c(
                  "请创建或启用一把未过期的 Key。",
                  "Create or enable an unexpired key.",
                  "أنشئ مفتاحًا غير منتهي الصلاحية أو فعّله.",
                )}
              </p>
              <Button onClick={onCreate}>
                {c("创建密钥", "Create key", "إنشاء مفتاح")}
              </Button>
            </div>
          )}
          <Field id="verify-key" label={c("Key（密钥）", "Key", "المفتاح")}>
            <select
              id="verify-key"
              className="select-trigger"
              value={keyId}
              disabled={busy}
              onChange={(e) => setKeyId(e.target.value)}
            >
              {!currentKey && (
                <option value={keyId}>
                  {c(
                    "请选择有效密钥",
                    "Select an active key",
                    "اختر مفتاحًا صالحًا",
                  )}
                </option>
              )}
              {validKeys.map((key) => (
                <option value={key.id} key={key.id}>
                  {key.name}
                </option>
              ))}
            </select>
          </Field>
          <div className="verify-field-row">
            <Field
              id="verify-protocol"
              label={c("请求协议", "Protocol", "البروتوكول")}
            >
              <select
                id="verify-protocol"
                className="select-trigger"
                value={protocol}
                disabled={busy}
                onChange={(e) => setProtocol(e.target.value as DemoProtocol)}
              >
                <option>OpenAI</option>
                <option>Anthropic</option>
                <option>Gemini</option>
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
                disabled={busy}
                onChange={(e) => setModel(e.target.value)}
              >
                {!selectedModel && (
                  <option value={model}>
                    {model} · {c("当前不可用", "Unavailable", "غير متاح")}
                  </option>
                )}
                {options.map((option) => (
                  <option key={option.id} value={option.id}>
                    {option.id} ·{" "}
                    {option.kind === "smart"
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
                "智能入口按最低成本策略在授权文本候选中选择，按实际模型计费。",
                "Cost routing selects an authorized, compatible candidate and charges its actual model rate.",
                "يختار التوجيه الأقل تكلفة نموذجًا نصيًا متوافقًا ومصرحًا به ويحاسب سعره الفعلي.",
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
              rows={7}
              value={prompt}
              disabled={busy}
              onChange={(e) => setPrompt(e.target.value)}
            />
          </Field>
          <details className="verify-advanced">
            <summary>{c("参数设置", "Parameters", "المعلمات")}</summary>
            <Field
              id="verify-system"
              label={c("系统提示", "System prompt", "تعليمات النظام")}
            >
              <textarea
                id="verify-system"
                className="input"
                rows={2}
                value={system}
                disabled={busy}
                onChange={(e) => setSystem(e.target.value)}
              />
            </Field>
            <div className="verify-field-row">
              <Field
                id="verify-temperature"
                label={c("温度", "Temperature", "درجة الحرارة")}
              >
                <Input
                  id="verify-temperature"
                  type="number"
                  min="0"
                  max="2"
                  step="0.1"
                  value={temperature}
                  disabled={busy}
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
                  disabled={busy}
                  onChange={(e) => setMaxTokens(Number(e.target.value))}
                />
              </Field>
            </div>
            <label className="verify-stream">
              <input
                id="verify-stream"
                type="checkbox"
                checked={stream}
                disabled={busy}
                onChange={(e) => setStream(e.target.checked)}
              />
              {c("流式输出", "Stream response", "إخراج متدفق")}
            </label>
          </details>
          {!validParameters && (
            <p role="alert">
              {c(
                "温度须为 0–2，最大输出须为 1–16384 的整数。",
                "Temperature must be 0–2; max output must be an integer from 1–16384.",
                "درجة الحرارة من 0 إلى 2 وحد الإخراج عدد صحيح من 1 إلى 16384.",
              )}
            </p>
          )}
          <div className="verify-send">
            <span>
              {c(
                "按所选 Key 的分组、权限和额度调用",
                "Uses this key’s group, permissions and quota",
                "يستخدم مجموعة المفتاح وصلاحياته وحصته",
              )}
            </span>
            <Button
              variant="primary"
              disabled={busy || !canSend}
              onClick={send}
            >
              <Play size={14} />
              {busy
                ? c("正在调用…", "Running…", "جارٍ التنفيذ…")
                : c("发送请求", "Send request", "إرسال الطلب")}
            </Button>
          </div>
        </section>
        <section className="verify-card verify-result">
          <div className="verify-card-heading">
            <h2>{c("调用结果", "Result", "نتيجة الطلب")}</h2>
            <span className="verify-demo-label">
              {c("实际调用", "LIVE", "فعلي")}
            </span>
          </div>
          {error && (
            <div role="alert" className="verify-outcome is-error">
              <CircleAlert size={18} />
              <p>{error}</p>
            </div>
          )}
          {busy && (
            <p role="status">
              {c(
                "正在接收响应并等待结算…",
                "Receiving response and awaiting settlement…",
                "جارٍ تلقي الاستجابة وانتظار التسوية…",
              )}
            </p>
          )}
          {output && (
            <pre
              className="verify-output"
              aria-label={c("响应内容", "Response", "الاستجابة")}
            >
              {output}
            </pre>
          )}
          {result && (
            <dl className="verify-metrics">
              {[
                [
                  c("实际模型", "Actual model", "النموذج الفعلي"),
                  result.actual_model,
                ],
                [
                  c("厂商", "Provider", "المزود"),
                  result.vendor_display || result.vendor,
                ],
                [
                  c("自动切换", "Fallback", "تبديل تلقائي"),
                  result.fallback ? c("是", "Yes", "نعم") : c("否", "No", "لا"),
                ],
                [
                  c("首 Token 延迟", "Time to first token", "زمن أول رمز"),
                  `${result.ttft_ms} ms`,
                ],
                [
                  c("总耗时", "Total time", "المدة الكلية"),
                  `${result.duration_ms} ms`,
                ],
                [
                  c(
                    "输入 / 输出词元",
                    "Input / output tokens",
                    "رموز الإدخال / الإخراج",
                  ),
                  `${result.input_tokens} / ${result.output_tokens}`,
                ],
                [
                  c("本次费用", "Charge", "الرسوم"),
                  formatMoney(result.cost_nano, "detail"),
                ],
                [
                  c("状态", "Status", "الحالة"),
                  result.status === 1
                    ? c("成功", "Succeeded", "نجح")
                    : c("未成功", "Failed", "فشل"),
                ],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>
                    <bdi>{value}</bdi>
                  </dd>
                </div>
              ))}
            </dl>
          )}
          {!busy && !output && !error && (
            <p className="verify-hint">
              {c(
                "发送后在此查看响应与实际结算信息。",
                "Send a request to see its response and settled usage.",
                "أرسل طلبًا لعرض الاستجابة والاستخدام الفعلي.",
              )}
            </p>
          )}
          {trace && (
            <small className="verify-trace" dir="ltr">
              Trace ID: {trace}
            </small>
          )}
          {(result || error) && (
            <Link to="/console/requests">
              {c("查看调用明细", "View request history", "عرض سجل الطلبات")}
              <ArrowRight size={14} />
            </Link>
          )}
          <details className="verify-code">
            <summary>
              {c("请求预览", "Request preview", "معاينة الطلب")}
            </summary>
            <Segmented
              label={c("代码语言", "Code language", "لغة الشفرة")}
              value={language}
              onChange={setLanguage}
              options={[
                { value: "curl", label: "cURL" },
                { value: "python", label: "Python" },
                { value: "node", label: "Node.js" },
              ]}
            />
            <p className="verify-hint">
              {c(
                "设置环境变量 MYAPIS_API_KEY 后运行，实际调用按用量计费。",
                "Set MYAPIS_API_KEY before running. Actual usage is billed.",
                "عيّن MYAPIS_API_KEY قبل التشغيل. يُحاسب الاستخدام الفعلي.",
              )}
            </p>
            <Button
              size="small"
              onClick={async () => {
                if (await copyText(code, t("copied"), t("copyFailed")))
                  setCopied(true);
              }}
            >
              <Copy size={13} />
              {copied
                ? c("已复制", "Copied", "تم النسخ")
                : c("复制代码", "Copy code", "نسخ الشفرة")}
            </Button>
            <pre dir="ltr" tabIndex={0}>
              <code>{code}</code>
            </pre>
          </details>
        </section>
      </div>
      <Modal
        open={passwordOpen}
        onOpenChange={(open) => {
          if (!busy) {
            setPasswordOpen(open);
            setPassword("");
            setPasswordError("");
          }
        }}
        title={c(
          "验证登录密码",
          "Verify login password",
          "تحقق من كلمة مرور الدخول",
        )}
        description={c(
          "调用所选密钥前验证身份，验证有效期为 5 分钟。",
          "Verify before using this key. Verification lasts five minutes.",
          "تحقق قبل استخدام المفتاح. يسري التحقق لخمس دقائق.",
        )}
        initialFocus={passwordInput}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (password) void execute(password);
          }}
        >
          <Field
            id="playground-password"
            label={c("登录密码", "Login password", "كلمة مرور الدخول")}
            error={passwordError}
          >
            <Input
              ref={passwordInput}
              id="playground-password"
              type="password"
              autoComplete="current-password"
              value={password}
              disabled={busy}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <Button type="submit" variant="primary" disabled={busy || !password}>
            {c("验证并调用", "Verify and send", "تحقق وأرسل")}
          </Button>
        </form>
      </Modal>
    </div>
  );
}
