import { message as translatedMessage } from "../i18n/messages";
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
          ? "Responses"
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
      result.unshift({ id: "myapis-auto", name: "MyAPIs Auto", kind: "smart" });
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
          responses: draft.target === "codex",
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
      openai: text(...translatedMessage("openai_compatible_tool_547ca52a15")),
      "claude-code": text(...translatedMessage("claude_code_baa49ea41b")),
      codex: text(...translatedMessage("codex_69989ef135")),
      "cc-switch": text(...translatedMessage("cc_switch_e72a1849da")),
    })[target];
  const validationUrl =
    draft.target === "codex"
      ? "/docs?article=responses"
      : `/console/playground?${new URLSearchParams({ key: draft.key, model: draft.model, protocol: protocol ?? "OpenAI" })}`;
  return (
    <div className="access-center">
      <header className="access-heading">
        <span className="access-demo-label">
          <Link2 size={14} />
          {text(...translatedMessage("connection_center_70bc5215a7"))}
        </span>
        <h1>
          {text(
            ...translatedMessage(
              "connect_models_to_the_tools_you_use_5693dbc10d",
            ),
          )}
        </h1>
        <p>
          {text(
            ...translatedMessage(
              "reuse_one_key_get_this_gateway_s_configuratio_af3dda0070",
            ),
          )}
        </p>
      </header>
      <div className="access-steps">
        <span>
          <b>1</b>
          {text(...translatedMessage("key_tool_209a4c657f"))}
        </span>
        <span>
          <b>2</b>
          {text(...translatedMessage("get_configuration_eb9b95097e"))}
        </span>
        <span>
          <b>3</b>
          {text(...translatedMessage("choose_a_model_in_client_fea7bbbaa0"))}
        </span>
        <span>
          <b>4</b>
          {text(...translatedMessage("inspect_cost_errors_84f8cff030"))}
        </span>
      </div>
      <div className="access-layout">
        <section className="access-panel">
          <h2>
            <KeyRound size={18} />
            {text(...translatedMessage("connection_settings_1ec40031ef"))}
          </h2>
          <div className="access-field">
            <label htmlFor="connect-key">
              {text(...translatedMessage("key_82ee8b617e"))}
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
                  ...translatedMessage("select_an_existing_key_a90e03eb3a"),
                )}
              </option>
              {keys.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name} · {item.tier}
                  {!item.active
                    ? ` · ${text(...translatedMessage("disabled_2556c8592f"))}`
                    : ""}
                </option>
              ))}
            </select>
            <small>
              {text(
                ...translatedMessage(
                  "reuse_a_compatible_key_switching_models_does__87d7023648",
                ),
              )}
            </small>
            <Link
              to={`/console/keys?create=1${draft.model ? `&model=${encodeURIComponent(draft.model)}` : ""}`}
            >
              {text(...translatedMessage("create_a_key_55d880e8e0"))}
            </Link>
          </div>
          {key && (
            <div className="access-notice">
              <span>
                {key.tier} ·{" "}
                {key.quotaLimit_usd == null
                  ? text(...translatedMessage("no_key_spending_cap_ab6eae7526"))
                  : `$${key.quotaLimit_usd} / ${key.quotaPeriod === "daily" ? text(...translatedMessage("day_2ab66518e8")) : key.quotaPeriod === "monthly" ? text(...translatedMessage("month_cd5ee0cc02")) : text(...translatedMessage("lifetime_36cf860fd2"))}`}
                <br />
                {text(
                  ...translatedMessage(
                    "available_models_depend_on_group_allowlist_an_09513b6eb5",
                  ),
                )}
              </span>
            </div>
          )}
          <div className="access-field">
            <label htmlFor="connect-tool">
              {text(...translatedMessage("connection_tool_a863d0a499"))}
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
                  ...translatedMessage(
                    "target_client_configured_by_cc_switch_52ce88017a",
                  ),
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
                  ...translatedMessage(
                    "it_manages_target_tool_configuration_it_does__04b4673187",
                  ),
                )}
              </small>
            </div>
          )}
          {
            <div className="access-field">
              <label htmlFor="connect-model">
                {text(...translatedMessage("initial_client_model_162ac4c3b6"))}
              </label>
              <select
                id="connect-model"
                value={selectedModel?.id ?? ""}
                onChange={(e) => update({ model: e.target.value })}
                disabled={!key?.active}
              >
                <option value="">
                  {text(
                    ...translatedMessage(
                      "choose_a_compatible_model_c61c5c3541",
                    ),
                  )}
                </option>
                {models.map((model) => (
                  <option value={model.id} key={model.id}>
                    {model.kind === "smart" ? "MyAPIs Auto" : model.name} ·{" "}
                    {model.kind === "smart"
                      ? text(...translatedMessage("smart_routing_34b9b06792"))
                      : text(...translatedMessage("specific_model_020bc6e3c7"))}
                  </option>
                ))}
              </select>
              <small>
                {text(
                  ...translatedMessage(
                    "this_is_a_default_configuration_value_not_a_k_6e52408eb1",
                  ),
                )}
              </small>
            </div>
          }
          {draft.model && !selectedModel && (
            <div role="alert" className="access-notice access-error">
              {text(
                ...translatedMessage(
                  "model_is_unauthorized_unavailable_or_incompat_98f1e54c37",
                  { p0: draft.model },
                ),
              )}
            </div>
          )}
          {key && !key.active && (
            <div role="alert" className="access-notice access-error">
              {text(
                ...translatedMessage(
                  "this_key_is_disabled_enable_it_in_key_managem_6e8bcf262d",
                ),
              )}{" "}
              <Link to="/console/keys">
                {text(...translatedMessage("manage_keys_2620f30141"))}
              </Link>
            </div>
          )}
        </section>
        <section className="access-panel">
          <h2>
            <Terminal size={18} />
            {text(...translatedMessage("manual_configuration_80244174e3"))}
          </h2>
          {draft.target === "codex" && !config ? (
            <div className="access-empty">
              <h2>
                {text(
                  ...translatedMessage(
                    "no_verified_compatible_models_yet_c268245cf0",
                  ),
                )}
              </h2>
              <p>
                {text(
                  ...translatedMessage(
                    "select_a_key_and_a_server_model_explicitly_de_1440a6a99c",
                  ),
                )}
              </p>
              <p>
                {text(
                  ...translatedMessage(
                    "a_supported_provider_would_need_model_provide_9a0cafcb22",
                  ),
                )}
              </p>
              <a
                href="https://developers.openai.com/codex/config-reference/"
                target="_blank"
                rel="noreferrer"
              >
                {text(
                  ...translatedMessage(
                    "official_configuration_reference_5e6cd63e28",
                  ),
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
                  ...translatedMessage(
                    "use_a_compatible_generic_tool_7d0692f71f",
                  ),
                )}
              </Button>
            </div>
          ) : !config ? (
            <div className="access-empty">
              <Link2 size={32} />
              <h2>
                {text(
                  ...translatedMessage(
                    "select_a_key_and_compatible_model_e22a723243",
                  ),
                )}
              </h2>
              <p>
                {text(
                  ...translatedMessage(
                    "configuration_uses_the_selected_tool_s_protoc_e91850a05f",
                  ),
                )}
              </p>
            </div>
          ) : (
            <>
              <div className="access-notice">
                {text(
                  ...translatedMessage(
                    "configuration_uses_this_gateway_s_address_con_d49a50204d",
                  ),
                )}
              </div>
              <dl className="access-config-fields">
                <div>
                  <dt>
                    {text(...translatedMessage("target_tool_92187a1bcb"))}
                  </dt>
                  <dd>{toolName(draft.target)}</dd>
                </div>
                <div>
                  <dt>{text(...translatedMessage("base_url_855160cdcc"))}</dt>
                  <dd>
                    <code>{config.baseUrl}</code>
                  </dd>
                </div>
                <div>
                  <dt>{text(...translatedMessage("protocol_ae04bb9db6"))}</dt>
                  <dd>
                    {config.protocol}
                    {lang === "zh"
                      ? config.protocol === "Anthropic"
                        ? text(
                            ...translatedMessage("ui__messages_api__cf7483e1"),
                          )
                        : config.protocol === "Responses"
                          ? text(
                              ...translatedMessage(
                                "ui__responses_api__8336b418",
                              ),
                            )
                          : text(
                              ...translatedMessage(
                                "ui__chat_completions_api__950bac8e",
                              ),
                            )
                      : ""}
                  </dd>
                </div>
                <div>
                  <dt>{text(...translatedMessage("model_id_dac89463ba"))}</dt>
                  <dd>
                    <code>{config.model}</code>
                  </dd>
                </div>
                <div>
                  <dt>
                    {text(...translatedMessage("credential_source_f0b7f25b79"))}
                  </dt>
                  <dd>
                    {includeSecret && unlocked
                      ? text(
                          ...translatedMessage(
                            "revealed_full_credential_18f93a8ed7",
                          ),
                        )
                      : text(
                          ...translatedMessage(
                            "environment_variable_myapis_api_key_35b44e4175",
                          ),
                        )}
                  </dd>
                </div>
              </dl>
              {key && !unlocked && (
                <details className="access-details">
                  <summary>
                    {text(
                      ...translatedMessage(
                        "need_the_full_credential_unlock_to_include_it_319dbca188",
                      ),
                    )}
                  </summary>
                  <div className="access-field">
                    <label htmlFor="connect-password">
                      {text(
                        ...translatedMessage(
                          "current_account_password_76ce47a071",
                        ),
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
                        ...translatedMessage(
                          "use_your_current_account_password_only_a_key__3517567824",
                        ),
                      )}
                    </small>
                    {unlockError && (
                      <small role="alert">
                        {text(
                          ...translatedMessage(
                            "password_verification_failed_retry_or_recover_cf7434368c",
                          ),
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
                      {text(...translatedMessage("reveal_full_key_3a12611d0d"))}
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
                      ...translatedMessage(
                        "include_the_revealed_full_credential_in_expor_39ef3b91f7",
                      ),
                    )}
                  </span>
                </label>
              )}
              <pre className="access-code">
                <code>{config.snippet}</code>
              </pre>
              <p className="access-muted">
                {text(
                  ...translatedMessage(
                    "when_locked_the_snippet_references_an_environ_3b07446035",
                  ),
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
                          ...translatedMessage(
                            "configuration_copied_65263caf0a",
                          ),
                        ),
                        text(
                          ...translatedMessage(
                            "copy_failed_select_and_copy_the_code_manually_af553f4035",
                          ),
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
                    ? text(
                        ...translatedMessage("configuration_copied_65263caf0a"),
                      )
                    : text(
                        ...translatedMessage("copy_configuration_2a9aea3409"),
                      )}
                </Button>
                <Button asChild variant="primary">
                  <Link to={validationUrl}>
                    {text(
                      ...translatedMessage(
                        "optional_validate_configuration_58f9dbac6c",
                      ),
                    )}
                    <ArrowRight size={15} />
                  </Link>
                </Button>
              </div>
              {copied === config.snippet && (
                <p className="access-muted" role="status">
                  {text(
                    ...translatedMessage(
                      "copied_only_no_request_was_generated_and_this_cff9d34e1a",
                    ),
                  )}
                </p>
              )}
              <details className="access-details" open>
                <summary>
                  {text(...translatedMessage("manual_setup_steps_919e1c5b4e"))}
                </summary>
                <ol>
                  <li>
                    {draft.tool === "cc-switch"
                      ? text(
                          ...translatedMessage(
                            "choose_the_target_client_in_cc_switch_then_ad_b38613bba3",
                          ),
                        )
                      : text(
                          ...translatedMessage(
                            "add_a_custom_provider_in_the_target_tool_cli__ef5141fb69",
                          ),
                        )}
                  </li>
                  <li>
                    {text(
                      ...translatedMessage(
                        "enter_the_base_url_full_credential_and_model__eedfc3d520",
                      ),
                    )}
                  </li>
                  <li>
                    {draft.target === "claude-code"
                      ? text(
                          ...translatedMessage(
                            "map_default_model_aliases_and_subagents_to_th_d5418b79d6",
                          ),
                        )
                      : text(
                          ...translatedMessage(
                            "enter_a_catalog_model_id_in_the_client_select_0d7f12dd5f",
                          ),
                        )}
                  </li>
                  <li>
                    {text(
                      ...translatedMessage(
                        "verify_the_gateway_here_with_a_full_key_then__bf62804e2f",
                      ),
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
                      ...translatedMessage(
                        "official_gateway_configuration_9de0e224f4",
                      ),
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
                  ...translatedMessage(
                    "a_matching_api_request_succeeded_270d3e6a0d",
                  ),
                )
              : failed
                ? text(
                    ...translatedMessage(
                      "latest_matching_request_did_not_succeed_0e661e7c64",
                    ),
                  )
                : text(
                    ...translatedMessage(
                      "awaiting_a_matching_request_6ab1ee625a",
                    ),
                  )}
          </strong>
          <p>
            {verified
              ? text(
                  ...translatedMessage(
                    "within_the_latest_100_logs_request_matches_th_66394ac14b",
                    { p0: verified.trace_id },
                  ),
                )
              : text(
                  ...translatedMessage(
                    "checks_only_the_latest_100_logs_copying_or_sa_7b8caca29b",
                  ),
                )}
          </p>
          {failed && !verified && (
            <p>
              {text(
                ...translatedMessage(
                  "the_latest_matching_call_failed_or_was_cancel_41aea8bdf9",
                ),
              )}
            </p>
          )}
          <div className="access-actions">
            <Button asChild variant="secondary" size="small">
              <Link
                to={`/console/requests${verified || failed ? `?trace=${encodeURIComponent((verified ?? failed)!.trace_id)}` : ""}`}
              >
                {text(...translatedMessage("view_requests_896eb3e854"))}
              </Link>
            </Button>
            <Button asChild variant="ghost" size="small">
              <Link to="/console/billing">
                {text(...translatedMessage("view_billing_415517ee1e"))}
              </Link>
            </Button>
          </div>
        </div>
      </section>
    </div>
  );
}
