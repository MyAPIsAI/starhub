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
import { useProductPreview, type PreviewRequest } from "../lib/product-preview";
import { useBackendKeys } from "../lib/api/key-adapter";
import { resolveVendor } from "../lib/api/usage";
import { getKeyModelOptions } from "../lib/key-model-policy";
import { findClientModel, validDemoSecret } from "../lib/client-config";
import { buildGatewayRequest } from "../lib/gateway-request";
import { readGeminiGatewayResponse } from "../lib/gemini-response";
import { useModelPricing } from "../lib/model-pricing";
import {
  buildCurlRequest,
  type DemoRequest,
  type DemoProtocol,
} from "../lib/request-contract";
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
  onRecord,
  onCreate,
  plaintextKeys,
  onRelayComplete,
}: {
  onRecord: (r: PreviewRequest) => void;
  onCreate: () => void;
  initialModel?: string;
  plaintextKeys?: Record<string, string>;
  onRelayComplete?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const [draft] = useState(readDraft);
  const { keys } = useBackendKeys(plaintextKeys);
  const selected = useProductPreview((s) => s.selectedKey);
  const choose = useProductPreview((s) => s.chooseKey);
  const recordCall = useProductPreview((s) => s.recordCall);
  const outage = useProductPreview((s) => s.demoMode.upstreamOutage);
  const setOutage = useProductPreview((s) => s.demoSetUpstreamOutage);
  useModelPricing((s) => s.config);
  const [keyId, setKeyId] = useState(
    params.get("key") || draft.keyId || selected,
  );
  const [model, setModel] = useState(
    params.get("model") || draft.model || "gpt-4.1",
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
  const [busy, setBusy] = useState(false);
  const [lastId, setLastId] = useState<string | null>(draft.lastId || null);
  const requests = useProductPreview((s) => s.requests);
  // Real relay result (when the key has a plaintext secret and the call
  // reaches the server). Falls back to demo-store requests when null.
  const [relayResult, setRelayResult] = useState<PreviewRequest | null>(null);
  const last = relayResult ?? requests.find((r) => r.trace_id === lastId);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const currentKey = keys.find((k) => k.id === keyId);
  const options = currentKey ? getKeyModelOptions(currentKey, protocol) : [];
  const selectedModel = findClientModel(options, model);
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
          lastId,
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
    lastId,
  ]);
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );
  const send = async () => {
    if (!currentKey || !prompt.trim() || busy) return;
    setBusy(true);
    setRelayResult(null);
    setLastId(null);
    const snapshot = structuredClone(request);
    const credential = currentKey.id;
    // If the key has a plaintext secret (not masked), try a real relay
    // call. Otherwise, fall back to the demo simulation.
    const secret = currentKey.keySecret;
    const isMasked = !validDemoSecret(secret);
    if (isMasked) {
      // No plaintext key available — simulate as before.
      timer.current = setTimeout(() => {
        const row = recordCall(credential, outage, snapshot.model, snapshot);
        if (row) setLastId(row.trace_id);
        setBusy(false);
      }, 420);
      return;
    }
    try {
      const call = buildGatewayRequest(snapshot, secret);
      const res = await fetch(call.endpoint, call.init);
      const data = await res.json();
      if (!res.ok) {
        const errMsg = data?.error?.message ?? `HTTP ${res.status}`;
        setRelayResult({
          source: "api",
          trace_id: `relay_${Date.now()}`,
          time: new Date().toISOString(),
          model: snapshot.model,
          vendor: resolveVendor(snapshot.model),
          status: "failed",
          cost_nano: "0",
          duration_ms: 0,
          key_id: credential,
          input_tokens: 0,
          output_tokens: 0,
          error_code: `http_${res.status}`,
          error_message: errMsg,
          requested_model: snapshot.model,
          response_text: "",
        } as any);
        setLastId(`relay_${Date.now()}`);
        setBusy(false);
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
        gemini?.inputTokens ?? data?.usage?.prompt_tokens ?? data?.usage?.input_tokens ?? 0;
      const outputTokens =
        gemini?.outputTokens ?? data?.usage?.completion_tokens ?? data?.usage?.output_tokens ?? 0;
      setRelayResult({
        source: "api",
        trace_id: `relay_${Date.now()}`,
        time: new Date().toISOString(),
        model: snapshot.model,
        vendor: resolveVendor(snapshot.model),
        status: gemini?.status ?? "success",
        cost_nano: "0",
        duration_ms: 0,
        key_id: credential,
        input_tokens: inputTokens,
        output_tokens: outputTokens,
        error_code: gemini?.errorCode ?? "",
        error_message: gemini?.errorCode ? `Gemini: ${gemini.errorCode}` : "",
        requested_model: snapshot.model,
        actual_model: gemini ? gemini.actualModel : data?.model ?? snapshot.model,
        protocol: snapshot.protocol,
        response_text: content,
      } as any);
      setLastId(`relay_${Date.now()}`);
    } catch {
      // A real request may have executed; never retry it as a simulation.
      setRelayResult({
        source: "api",
        trace_id: `relay_${Date.now()}`,
        time: new Date().toISOString(),
        model: snapshot.model,
        requested_model: snapshot.model,
        status: "failed",
        cost_nano: "0",
        key_id: credential,
        vendor: resolveVendor(snapshot.model),
        duration_ms: 0,
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
      onRelayComplete?.();
    }
    setBusy(false);
  };
  const error = last?.status === "failed";
  return (
    <div className="verify-workspace">
      <div className="verify-notice">
        <Terminal size={19} />
        <div>
          <strong>
            {c(
              "本地验证 · 可选步骤",
              "Local verification · Optional",
              "تحقق محلي · اختياري",
            )}
          </strong>
          <p>
            {c(
              "完整密钥会发起实际网关请求并按用量计费；缺少密钥明文时仅模拟。结果不能代替目标客户端的连通性测试。输入会保留在当前标签页草稿中。",
              "A full key sends a real gateway request with usage charges; without it, only simulation runs. Results do not prove connectivity from your target client. Drafts stay in this tab.",
              "المفتاح الكامل يرسل طلبًا فعليًا مدفوعًا حسب الاستخدام؛ بدونه تُستخدم المحاكاة فقط. النتيجة لا تثبت اتصال عميلك.",
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
          {!keys.length ? (
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
                  onChange={(e) => {
                    setKeyId(e.target.value);
                    choose(e.target.value);
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
                    <option value="OpenAI">OpenAI（对话协议）</option>
                    <option value="Anthropic">Anthropic（消息协议）</option>
                    <option value="Gemini">{c("Gemini（内容生成）", "Gemini (content generation)", "Gemini (إنشاء المحتوى)")}</option>
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
                  {!validDemoSecret(currentKey?.keySecret)
                    ? c(
                        "密钥明文不可用 · 将模拟请求",
                        "Key secret unavailable · will simulate",
                        "سر المفتاح غير متاح · محاكاة",
                      )
                    : c(
                        "将通过真实接口调用",
                        "Sends a real API request",
                        "يرسل طلبًا حقيقيًا",
                      )}
                </span>
                <Button
                  variant="primary"
                  disabled={
                    busy ||
                    !currentKey ||
                    !currentKey.active ||
                    !prompt.trim()
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
              {relayResult
                ? c("接口调用", "API CALL", "استدعاء")
                : c("模拟", "SIMULATION", "محاكاة")}
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
                    {relayResult
                      ? error
                        ? c(
                            "接口调用未成功",
                            "API call did not succeed",
                            "لم ينجح استدعاء الواجهة",
                          )
                        : c(
                            "接口调用成功 · 费用以调用日志为准",
                            "API call succeeded · see logs for charges",
                            "نجح الاستدعاء · راجع الرسوم في السجلات",
                          )
                      : error
                        ? c(
                            "请求被拒绝 · 未计费",
                            "Request rejected · Not charged",
                            "تم رفض الطلب · بلا رسوم",
                          )
                        : c(
                            "模拟验证通过 · 已计费",
                            "Simulation verified · Charged",
                            "نجحت المحاكاة · احتسبت الرسوم",
                          )}
                  </strong>
                  <p>
                    {relayResult
                      ? error
                        ? last.error_message
                        : c(
                            "此处仅显示接口响应。实际结算状态和费用请查看调用日志。",
                            "This shows the API response only. Check request logs for settlement and charges.",
                            "تظهر هنا استجابة الواجهة فقط. راجع السجلات للتسوية والرسوم.",
                          )
                      : error
                        ? last.error_message
                        : last.cost_nano === "0"
                          ? c(
                              "已结算零费用，未扣减余额。",
                              "Settled at zero cost; balance unchanged.",
                              "تمت التسوية دون تكلفة أو خصم.",
                            )
                          : c(
                              "已生成一笔调用记录和一笔扣费流水。",
                              "One request and one charge were recorded.",
                              "سُجل طلب واحد وقيد مالي واحد.",
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
                    {relayResult ? (
                      <Link to="/console/requests">
                        {c(
                          "查看调用日志",
                          "View request logs",
                          "عرض سجلات الطلبات",
                        )}
                      </Link>
                    ) : (
                      formatMoney(last.cost_nano, "call")
                    )}
                  </dd>
                </div>
              </dl>
              {last.response_text && (
                <pre className="verify-output">{last.response_text}</pre>
              )}
              {!error && !last.response_text && (
                <p className="verify-hint">
                  {c(
                    "刷新后仅保留调用元数据；请求内容与模拟输出不写入账务记录。",
                    "Only metadata survives refresh. Messages and outputs are excluded from billing storage.",
                    "بعد التحديث تبقى البيانات الوصفية فقط.",
                  )}
                </p>
              )}
              <div className="verify-result-actions">
                {relayResult ? (
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
                ) : (
                  <Button onClick={() => onRecord(last)}>
                    {c("查看本次明细", "View request details", "تفاصيل الطلب")}
                    <ArrowRight size={14} />
                  </Button>
                )}
                {error && (
                  <>
                    <Link to="/console/billing/recharge">
                      {c("补充演示余额", "Add demo balance", "إضافة رصيد")}
                    </Link>
                    <Link to="/console/keys">
                      {c(
                        "检查密钥与额度",
                        "Check key and quota",
                        "فحص المفتاح والحصة",
                      )}
                    </Link>
                    {outage && (
                      <Button onClick={() => setOutage(false)}>
                        {c(
                          "恢复模拟上游",
                          "Restore simulated upstream",
                          "استعادة الخدمة التجريبية",
                        )}
                      </Button>
                    )}
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
