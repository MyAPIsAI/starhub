import { message as translatedMessage } from "../i18n/messages";
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
        ...translatedMessage(
          "briefly_explain_how_to_get_started_with_myapi_c2d229c2d7",
        ),
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
            ...translatedMessage(
              "request_trace_unavailable_check_request_histo_4597e426d3",
            ),
          ),
        );
    } catch (err) {
      if (!unchanged()) return;
      if (!sent) {
        setPassword("");
        setPasswordOpen(true);
        setPasswordError(
          c(
            ...translatedMessage(
              "unable_to_unlock_this_key_verify_your_login_p_75e13e927e",
            ),
          ),
        );
      } else
        setError(
          err instanceof Error
            ? err.message
            : c(
                ...translatedMessage(
                  "request_outcome_unknown_check_history_before__594bad066c",
                ),
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
            {c(...translatedMessage("text_chat_playground_d82514594e"))}
          </strong>
          <p>
            {c(
              ...translatedMessage(
                "calls_the_gateway_with_your_selected_key_and__af174ebb92",
              ),
            )}
          </p>
        </div>
        <Link
          to={`/console/connect?key=${encodeURIComponent(keyId)}&model=${encodeURIComponent(model)}`}
        >
          {c(...translatedMessage("connections_83e40faf19"))}
          <ArrowRight size={14} />
        </Link>
      </div>
      <div className="verify-columns">
        <section className="verify-card">
          <div className="verify-card-heading">
            <h2>
              {c(...translatedMessage("request_configuration_32b0653457"))}
            </h2>
            {currentKey && <TierBadge id={currentKey.tier} />}
          </div>
          {(keysError || choices.isError) && (
            <p role="alert">
              {c(
                ...translatedMessage(
                  "unable_to_load_keys_or_models_retry_5f7d38abf6",
                ),
              )}{" "}
              <Button
                onClick={() => {
                  void choices.refetch();
                  void refetchKeys();
                }}
              >
                {c(...translatedMessage("retry_c16455021c"))}
              </Button>
            </p>
          )}
          {loading && (
            <p role="status">
              {c(...translatedMessage("loading_keys_a7ac88f41a"))}
            </p>
          )}
          {!loading && !validKeys.length && (
            <div className="verify-empty">
              <h3>{c(...translatedMessage("no_active_keys_e117479d2f"))}</h3>
              <p>
                {c(
                  ...translatedMessage(
                    "create_or_enable_an_unexpired_key_88660033b1",
                  ),
                )}
              </p>
              <Button onClick={onCreate}>
                {c(...translatedMessage("create_key_06433c30b0"))}
              </Button>
            </div>
          )}
          <Field
            id="verify-key"
            label={c(...translatedMessage("key_82ee8b617e"))}
          >
            <select
              id="verify-key"
              className="select-trigger"
              value={keyId}
              disabled={busy}
              onChange={(e) => setKeyId(e.target.value)}
            >
              {!currentKey && (
                <option value={keyId}>
                  {c(...translatedMessage("select_an_active_key_10677a92e9"))}
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
              label={c(...translatedMessage("protocol_35e80b8457"))}
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
              label={c(...translatedMessage("request_model_177665bff5"))}
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
                    {model} ·{" "}
                    {c(...translatedMessage("unavailable_018e9b5757"))}
                  </option>
                )}
                {options.map((option) => (
                  <option key={option.id} value={option.id}>
                    {option.id} ·{" "}
                    {option.kind === "smart"
                      ? c(...translatedMessage("smart_routing_34b9b06792"))
                      : c(...translatedMessage("specified_model_19254d43b3"))}
                  </option>
                ))}
              </select>
            </Field>
          </div>
          {selectedModel?.kind === "smart" && (
            <p className="verify-hint">
              {c(
                ...translatedMessage(
                  "cost_routing_selects_an_authorized_compatible_53bb805f6f",
                ),
              )}
            </p>
          )}
          <Field
            id="verify-prompt"
            label={c(...translatedMessage("message_1f77e94bb4"))}
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
            <summary>
              {c(...translatedMessage("parameters_e2a1266467"))}
            </summary>
            <Field
              id="verify-system"
              label={c(...translatedMessage("system_prompt_8eacc1e898"))}
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
                label={c(...translatedMessage("temperature_5cd567cbc8"))}
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
                label={c(...translatedMessage("max_output_tokens_c478d69b7b"))}
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
              {c(...translatedMessage("stream_response_da18895327"))}
            </label>
          </details>
          {!validParameters && (
            <p role="alert">
              {c(
                ...translatedMessage(
                  "temperature_must_be_0_2_max_output_must_be_an_0ffb28e13c",
                ),
              )}
            </p>
          )}
          <div className="verify-send">
            <span>
              {c(
                ...translatedMessage(
                  "uses_this_key_s_group_permissions_and_quota_68d373b116",
                ),
              )}
            </span>
            <Button
              variant="primary"
              disabled={busy || !canSend}
              onClick={send}
            >
              <Play size={14} />
              {busy
                ? c(...translatedMessage("running_df0e221215"))
                : c(...translatedMessage("send_request_97662035e5"))}
            </Button>
          </div>
        </section>
        <section className="verify-card verify-result">
          <div className="verify-card-heading">
            <h2>{c(...translatedMessage("result_8bf4743b85"))}</h2>
            <span className="verify-demo-label">
              {c(...translatedMessage("live_44ed0ed37e"))}
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
                ...translatedMessage(
                  "receiving_response_and_awaiting_settlement_1554b32117",
                ),
              )}
            </p>
          )}
          {output && (
            <pre
              className="verify-output"
              aria-label={c(...translatedMessage("response_5ded1f2cd9"))}
            >
              {output}
            </pre>
          )}
          {result && (
            <dl className="verify-metrics">
              {[
                [
                  c(...translatedMessage("actual_model_87fe7e61b2")),
                  result.actual_model,
                ],
                [
                  c(...translatedMessage("provider_efd2d07eac")),
                  result.vendor_display || result.vendor,
                ],
                [
                  c(...translatedMessage("fallback_5d371dc968")),
                  result.fallback
                    ? c(...translatedMessage("yes_3445b9ddec"))
                    : c(...translatedMessage("no_b02901acfa")),
                ],
                [
                  c(...translatedMessage("time_to_first_token_88b0726bc5")),
                  `${result.ttft_ms} ms`,
                ],
                [
                  c(...translatedMessage("total_time_c519957b3a")),
                  `${result.duration_ms} ms`,
                ],
                [
                  c(...translatedMessage("input_output_tokens_57071059f4")),
                  `${result.input_tokens} / ${result.output_tokens}`,
                ],
                [
                  c(...translatedMessage("charge_4440ece1ce")),
                  formatMoney(result.cost_nano, "detail"),
                ],
                [
                  c(...translatedMessage("status_dda39dcd63")),
                  result.status === 1
                    ? c(...translatedMessage("succeeded_1a524ac49d"))
                    : c(...translatedMessage("failed_b8a1e467e3")),
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
                ...translatedMessage(
                  "send_a_request_to_see_its_response_and_settle_3a5e895579",
                ),
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
              {c(...translatedMessage("view_request_history_5e59ba4557"))}
              <ArrowRight size={14} />
            </Link>
          )}
          <details className="verify-code">
            <summary>
              {c(...translatedMessage("request_preview_fb4f66d60f"))}
            </summary>
            <Segmented
              label={c(...translatedMessage("code_language_414510bdc4"))}
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
                ...translatedMessage(
                  "set_myapis_api_key_before_running_actual_usag_a999d73a42",
                ),
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
                ? c(...translatedMessage("copied_e6bf924fa2"))
                : c(...translatedMessage("copy_code_f09930b305"))}
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
        title={c(...translatedMessage("verify_login_password_c03eb83bf5"))}
        description={c(
          ...translatedMessage(
            "verify_before_using_this_key_verification_las_61a0f711ac",
          ),
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
            label={c(...translatedMessage("login_password_e69bc15f89"))}
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
            {c(...translatedMessage("verify_and_send_3d7c3706d3"))}
          </Button>
        </form>
      </Modal>
    </div>
  );
}
