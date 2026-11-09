import { message as translatedMessage } from "../../i18n/messages";
import { useState, useEffect, useRef } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  KeyRound,
  Terminal,
  Cpu,
  ShieldCheck,
  Coins,
  Sparkles,
  ArrowUpRight,
  BookOpen,
  ChevronRight,
  Search,
  ChevronDown,
} from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { CodeBlock, DocsCodeContext } from "./code-block";
import { usePreferences } from "../../lib/preferences";
import { placeholders } from "../../config/placeholders";
import { responsesGuide } from "./responses-guide";
import { geminiNativeGuide } from "./gemini-guide";
import { gatewayPolicyGuide } from "./gateway-policy-guide";
import { claudeCodeGuide } from "./claude-code-guide";

import { normalizeDocsBase } from "./examples";
import { ArticleOutline } from "./navigation";
import { documentationText } from "./content";
import {
  Quickstart,
  Supplement,
  extraGuides,
  type ExtraArticle,
} from "./completion";

type ArticleId =
  | ExtraArticle
  | "quickstart"
  | "auth"
  | "tiers"
  | "claudeCode"
  | "codex"
  | "cursor"
  | "errors"
  | "billing"
  | "protocols"
  | "models"
  | "chat-completions"
  | "responses"
  | "anthropic"
  | "gemini"
  | "routing"
  | "limits"
  | "faq";
const cursorSettings = `OpenAI API Key: <YOUR_MYAPIS_API_KEY>
Override OpenAI Base URL: ${placeholders.apiBase}
Custom Model: YOUR_AUTHORIZED_CHAT_MODEL`;
const geminiExample = `curl --fail --silent --show-error ${placeholders.apiBase}/chat/completions \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "YOUR_AUTHORIZED_GEMINI_MODEL",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 64,
    "stream": false
  }'`;
const anthropicExample = `curl --fail --silent --show-error ${placeholders.apiBase}/messages \\
  -H "x-api-key: $MYAPIS_API_KEY" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "YOUR_AUTHORIZED_CLAUDE_MODEL",
    "system": "Reply concisely.",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 64,
    "stream": false
  }'`;
const anthropicResponse = JSON.stringify(
  {
    id: "msg_example",
    type: "message",
    role: "assistant",
    model: "claude-sonnet-4-20250514",
    content: [{ type: "text", text: "Hello!" }],
    stop_reason: "end_turn",
  },
  null,
  2,
);
const chatExample = `curl --fail --silent --show-error ${placeholders.apiBase}/chat/completions \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "YOUR_AUTHORIZED_CHAT_MODEL",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 64,
    "stream": false
  }'`;
const chatResponse = JSON.stringify(
  {
    id: "chatcmpl-example",
    object: "chat.completion",
    created: 0,
    model: "gpt-4.1",
    choices: [
      {
        index: 0,
        message: { role: "assistant", content: "Hello!" },
        finish_reason: "stop",
      },
    ],
  },
  null,
  2,
);
const modelsExample = `curl --fail --silent --show-error ${placeholders.apiBase}/models \\
  -H "Authorization: Bearer $MYAPIS_API_KEY"`;
const modelsResponse = JSON.stringify(
  {
    object: "list",
    data: [{ id: "gpt-4.1", object: "model", created: 0, owned_by: "starhub" }],
  },
  null,
  2,
);
export default function DocsPage() {
  const { t } = useTranslation();
  const lang = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const [searchParams, setSearchParams] = useSearchParams();
  const copy = documentationText(text);
  const [baseInput, setBaseInput] = useState<string>(placeholders.apiBase);
  const normalizedBase = normalizeDocsBase(baseInput);
  const baseValid = Boolean(normalizedBase);
  const apiBase = normalizedBase ?? placeholders.apiBase;
  const extra = extraGuides(copy, apiBase);
  const responses = responsesGuide(text, apiBase);
  const nativeGemini = geminiNativeGuide(text, apiBase);
  const policy = gatewayPolicyGuide(text);
  const claudeCode = claudeCodeGuide(text, apiBase);
  const articles: { id: ArticleId; title: string; Icon: typeof Terminal }[] = [
    { id: "quickstart", title: t("docsQuickstart"), Icon: Terminal },
    { id: "auth", title: t("docsAuth"), Icon: KeyRound },
    { id: "tiers", title: t("docsTiers"), Icon: Cpu },
    {
      id: "protocols",
      title: text(...translatedMessage("protocols_0fc0348ab3")),
      Icon: ArrowUpRight,
    },
    {
      id: "chat-completions",
      title: text(...translatedMessage("chat_completions_9c944fa26f")),
      Icon: Terminal,
    },
    {
      id: "responses",
      title: copy.responsesTitle,
      Icon: Terminal,
    },
    {
      id: "anthropic",
      title: text(...translatedMessage("anthropic_messages_df399e36c0")),
      Icon: Terminal,
    },
    {
      id: "gemini",
      title: text(
        ...translatedMessage("gemini_native_and_compatible_apis_b9e6d691c6"),
      ),
      Icon: Terminal,
    },
    {
      id: "models",
      title: text(...translatedMessage("model_list_aba1fb7a46")),
      Icon: Cpu,
    },
    ...Object.entries(extra).map(([id, guide]) => ({
      id: id as ExtraArticle,
      title: guide.title,
      Icon: Terminal,
    })),
    { id: "claudeCode", title: t("docsClaudeCode"), Icon: Sparkles },
    { id: "codex", title: copy.codexTitle, Icon: Terminal },
    {
      id: "cursor",
      title: text(...translatedMessage("cursor_setup_1976afd7a2")),
      Icon: Terminal,
    },
    { id: "errors", title: t("docsErrors"), Icon: ShieldCheck },
    { id: "billing", title: t("docsBilling"), Icon: Coins },
    {
      id: "routing",
      title: text(...translatedMessage("routing_authz_14e8ee8325")),
      Icon: ArrowUpRight,
    },
    {
      id: "limits",
      title: text(...translatedMessage("limits_rate_a19fcb7918")),
      Icon: BookOpen,
    },
    {
      id: "faq",
      title: text(...translatedMessage("faq_cb96028c75")),
      Icon: ChevronRight,
    },
  ];
  const groups: { title: string; ids: ArticleId[] }[] = [
    { title: copy.groupStart, ids: ["quickstart", "auth", "protocols"] },
    {
      title: copy.groupAPI,
      ids: ["chat-completions", "responses", "anthropic", "gemini", "models"],
    },
    { title: copy.groupMedia, ids: ["embeddings", "images", "audio", "video"] },
    {
      title: copy.groupRules,
      ids: ["tiers", "routing", "billing", "errors", "limits"],
    },
    {
      title: copy.groupTools,
      ids: ["claudeCode", "codex", "geminiCli", "cursor"],
    },
    { title: copy.groupReference, ids: ["faq"] },
  ];
  const orderedArticles = groups.flatMap((group) =>
    group.ids.map((id) => articles.find((article) => article.id === id)!),
  );
  const [menuOpen, setMenuOpen] = useState(false);
  const articleRef = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const focusArticle = useRef(false);
  const validIds = articles.map((article) => article.id);
  const urlArticle = searchParams.get("article");
  const activeTab: ArticleId =
    urlArticle && validIds.includes(urlArticle as ArticleId)
      ? (urlArticle as ArticleId)
      : "quickstart";
  const setActiveTab = (id: ArticleId, replace = false) => {
    const next = new URLSearchParams(searchParams);
    if (id === "quickstart") next.delete("article");
    else next.set("article", id);
    setSearchParams(next, { replace });
  };
  const selectArticle = (id: ArticleId) => {
    setMenuOpen(false);
    focusArticle.current = true;
    setActiveTab(id);
    if (id === activeTab)
      requestAnimationFrame(() => {
        const heading = articleRef.current?.querySelector<HTMLElement>("h1");
        heading?.focus({ preventScroll: true });
        heading?.scrollIntoView({ block: "start" });
        focusArticle.current = false;
      });
  };
  useEffect(() => {
    if (!focusArticle.current) return;
    const heading = articleRef.current?.querySelector<HTMLElement>("h1");
    heading?.focus({ preventScroll: true });
    heading?.scrollIntoView({ block: "start" });
    focusArticle.current = false;
  }, [activeTab]);
  const articleIndex = orderedArticles.findIndex(
    (article) => article.id === activeTab,
  );
  const previous = orderedArticles[articleIndex - 1];
  const next = orderedArticles[articleIndex + 1];
  const groupTitle = groups.find((group) =>
    group.ids.includes(activeTab),
  )!.title;
  const [searchQuery, setSearchQuery] = useState("");
  const current = articles.find((article) => article.id === activeTab)!;
  const cursorLead = text(
    ...translatedMessage(
      "use_your_gateway_key_in_cursor_with_a_chat_co_6640c0670e",
    ),
  );
  const cursorSteps = [
    text(
      ...translatedMessage(
        "open_cursor_settings_models_under_api_keys_en_ba349d0d2b",
      ),
    ),
    text(
      ...translatedMessage(
        "enable_override_openai_base_url_and_enter_kee_9f654a48b7",
        { p0: apiBase },
      ),
    ),
    text(
      ...translatedMessage(
        "use_add_custom_model_to_add_and_enable_the_ex_927b694c4e",
      ),
    ),
  ];
  const cursorBoundary = text(
    ...translatedMessage(
      "this_guide_covers_compatible_chat_configurati_21a810ac32",
    ),
  );
  const cursorTroubleshooting = text(
    ...translatedMessage(
      "if_connection_fails_first_test_the_same_key_b_f2376057ab",
    ),
  );
  const geminiLead = text(
    ...translatedMessage(
      "call_gemini_text_models_through_the_gateway_s_5ea3824806",
    ),
  );
  const geminiBoundary = text(
    ...translatedMessage(
      "the_compatible_chat_endpoint_uses_messages_an_e6131c093f",
    ),
  );
  const geminiResponseNote = text(
    ...translatedMessage(
      "with_stream_false_read_text_from_chat_complet_58ae9e529a",
    ),
  );
  const anthropicLead = text(
    ...translatedMessage(
      "post_v1_messages_uses_the_anthropic_message_f_1cb834ff3f",
    ),
  );
  const anthropicAuth = text(
    ...translatedMessage(
      "the_example_uses_x_api_key_and_includes_anthr_2bd0d163b3",
    ),
  );
  const anthropicResponseNote = text(
    ...translatedMessage(
      "in_a_non_streaming_response_read_text_from_th_6a5af5f740",
    ),
  );
  const anthropicStreaming = text(
    ...translatedMessage(
      "with_stream_true_handle_sse_events_read_delta_78681acfd2",
    ),
  );
  const chatLead = text(
    ...translatedMessage(
      "post_v1_chat_completions_accepts_model_and_me_ccca4cddeb",
    ),
  );
  const chatResponseNote = text(
    ...translatedMessage(
      "stream_false_returns_a_complete_json_response_32da3b21bd",
    ),
  );
  const chatStreaming = text(
    ...translatedMessage(
      "for_streaming_set_stream_true_and_read_increm_7d2dda1561",
    ),
  );
  const modelsLead = text(
    ...translatedMessage(
      "get_v1_models_lists_enabled_model_ids_in_the__1849640cee",
    ),
  );
  const modelsScope = text(
    ...translatedMessage(
      "the_list_is_scoped_to_the_service_group_witho_262bb7a361",
    ),
  );
  const modelsResponseNote = text(
    ...translatedMessage(
      "the_json_below_illustrates_the_response_forma_95ec33c72c",
    ),
  );

  // Article search: full-text match against title, keywords, and article body.
  const articleKeywords: Partial<Record<ArticleId, string[]>> = {
    quickstart: [
      "curl",
      "python",
      "node",
      "api key",
      "base url",
      "getting started",
      "快速开始",
      "入门",
      "创建密钥",
    ],
    auth: [
      "authentication",
      "authorization",
      "bearer",
      "token",
      "api key",
      "认证",
      "授权",
      "密钥",
      "bearer",
    ],
    tiers: [
      "service tier",
      "group",
      "ultra",
      "pro",
      "standard",
      "lite",
      "分组",
      "等级",
      "服务",
    ],
    claudeCode: ["claude", "anthropic", "工具集成", "claude code", "编程工具"],
    codex: ["codex", "config.toml", "model_provider", "wire_api"],
    cursor: ["cursor setup", "Cursor 接入指南", "إعداد Cursor"],
    errors: [
      "error",
      "400",
      "401",
      "403",
      "429",
      "502",
      "503",
      "错误",
      "错误码",
      "故障排除",
    ],
    billing: [
      "billing",
      "quota",
      "cost",
      "charge",
      "balance",
      "top-up",
      "计费",
      "费用",
      "余额",
      "充值",
    ],
    protocols: [
      "protocol",
      "endpoint",
      "api",
      "curl",
      "url",
      "path",
      "协议",
      "端点",
      "路径",
      "接口",
    ],
    models: ["model list", "/v1/models", "模型列表", "قائمة النماذج"],
    "chat-completions": [
      "chat completions",
      "/v1/chat/completions",
      "聊天补全",
      "إكمال المحادثات",
    ],
    responses: ["responses", "/v1/responses", "响应接口"],
    anthropic: [
      "anthropic messages",
      "/v1/messages",
      "消息协议",
      "رسائل Anthropic",
    ],
    gemini: [
      "gemini compatibility",
      "Gemini 兼容接入",
      "الوصول المتوافق إلى Gemini",
    ],
    routing: [
      "routing",
      "authorization",
      "group",
      "model",
      "allowlist",
      "auto",
      "路由",
      "授权",
      "分组",
      "白名单",
    ],
    limits: [
      "limit",
      "rate",
      "quota",
      "concurrency",
      "throttle",
      "配额",
      "限流",
      "并发",
      "额度",
    ],
    faq: ["faq", "question", "answer", "常见问题", "问题", "解答"],
  };
  // Searchable body text for each article, built from the article content so
  // the search covers the full text, not just preset keywords.
  const articleBody: Partial<Record<ArticleId, string>> = {
    quickstart:
      "first request endpoint key curl python streaming demo domain environment variable chat request terminal connect application consistent interface",
    auth:
      policy.quota +
      " keys access boundaries usage quotas store securely project restrict access IP allowlist source control environment variables review request logs",
    tiers:
      "service level workload Ultra 1.00 99.5% 800ms Pro 0.80 99.0% 1500ms Standard 0.60 97.5% 3000ms Lite 0.40 price multiplier target latency demo configuration",
    claudeCode: Object.values(claudeCode).join(" "),
    codex: [responses.codex, responses.config, responses.boundary].join(" "),
    cursor: [
      cursorLead,
      ...cursorSteps,
      cursorSettings,
      cursorBoundary,
      cursorTroubleshooting,
    ].join(" "),
    errors:
      "401 invalid_api_key token_disabled token_expired 403 insufficient_quota token_quota_exhausted auto_group_empty group_not_available 400 invalid_request_body invalid_json invalid_multipart model_required 429 rate_limit_exceeded 502 upstream_unavailable retry_budget_exhausted upstream_result_uncertain 503 no_available_channel 500 pre_consume_failed channel_selection_failed Authorization Bearer JSON multipart",
    billing:
      policy.wallet +
      " balance precision usage totals request logs export details account balance 2 4 6 9 display reconciliation",
    protocols:
      "Chat Completions OpenAI Messages Anthropic Embeddings Images generations Audio TTS speech STT transcriptions translations Video submit poll Realtime WebSocket Models list Bearer token base URL path auth",
    models: [
      modelsLead,
      modelsScope,
      modelsResponseNote,
      modelsExample,
      modelsResponse,
    ].join(" "),
    "chat-completions": [
      chatLead,
      chatResponseNote,
      chatStreaming,
      chatExample,
      chatResponse,
    ].join(" "),
    responses: Object.entries(responses)
      .filter(([key]) => key !== "codex" && key !== "config")
      .map(([, value]) => value)
      .join(" "),
    anthropic: [
      anthropicLead,
      anthropicAuth,
      anthropicResponseNote,
      anthropicStreaming,
      anthropicExample,
      anthropicResponse,
    ].join(" "),
    gemini: [
      geminiLead,
      geminiBoundary,
      geminiResponseNote,
      geminiExample,
      ...Object.values(nativeGemini),
    ].join(" "),
    routing:
      policy.tokenFields.flat().join(" ") +
      " routing flow key auth channel selection pre-charge settle refund",
    limits: [
      policy.defaults,
      policy.quota,
      policy.rateHelp,
      ...policy.rateRows.flat(),
    ].join(" "),
    faq:
      policy.wallet +
      " plaintext API key creation regeneration mask sk-**** failed requests charged pre-charged quota refunded switch models model field",
  };
  const filteredArticles = (() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return articles;
    return articles.filter(({ id, title }) => {
      const keywords = articleKeywords[id] || [];
      const guide = extra[id as ExtraArticle];
      const additions =
        id === "quickstart"
          ? [
              copy.startLead,
              copy.registerHelp,
              copy.fundHelp,
              copy.fundBoundary,
              copy.keyHelp,
              copy.setupHelp,
              copy.modelHelp,
              copy.sdkHelp,
              copy.successHelp,
              copy.troubleshoot,
            ].join(" ")
          : id === "auth"
            ? copy.authHelp
            : id === "tiers" || id === "routing"
              ? copy.routingHelp
              : id === "billing"
                ? copy.billingHelp
                : id === "limits"
                  ? copy.limitsHelp
                  : copy.diagnosticsHelp;
      const body = [
        articleBody[id] || "",
        guide ? guide.lead + guide.examples.join(" ") : additions,
      ].join(" ");
      return (
        title.toLowerCase().includes(q) ||
        body.toLowerCase().includes(q) ||
        keywords.some((kw) => kw.toLowerCase().includes(q))
      );
    });
  })();

  useEffect(() => {
    if (
      searchQuery.trim() &&
      filteredArticles.length === 1 &&
      filteredArticles[0].id !== activeTab
    ) {
      setActiveTab(filteredArticles[0].id, true);
    }
  }, [searchQuery, lang]);
  const connectionSettings = (
    <div className="docs-endpoint">
      <label className="docs-field">
        {copy.baseLabel}
        <input
          dir="ltr"
          type="url"
          value={baseInput}
          onChange={(event) => setBaseInput(event.target.value)}
          aria-invalid={!baseValid}
          aria-describedby="docs-base-help"
          spellCheck={false}
        />
      </label>
      <details className="docs-field-help">
        <summary>{copy.readMore}</summary>
        <p id="docs-base-help">{copy.baseHelp}</p>
      </details>
      {!baseValid && <p role="alert">{copy.baseInvalid}</p>}
    </div>
  );

  return (
    <DocsCodeContext.Provider value={{ apiBase, baseValid }}>
      <div className="brand-home public-refined docs-refined">
        <PublicHeader />
        <div className="docs-layout">
          <aside
            className="docs-sidebar"
            aria-label={t("docsStationTitle")}
            onKeyDown={(event) => {
              if (event.key === "Escape" && menuOpen) {
                setMenuOpen(false);
                menuButton.current?.focus();
              }
            }}
          >
            <button
              ref={menuButton}
              type="button"
              className="docs-menu-toggle"
              aria-expanded={menuOpen}
              aria-controls="docs-directory"
              onClick={() => setMenuOpen(!menuOpen)}
            >
              <BookOpen size={17} />
              <span>{copy.directory}</span>
              <ChevronDown size={16} />
            </button>
            <nav
              id="docs-directory"
              aria-label={copy.directory}
              className={menuOpen ? "is-open" : ""}
            >
              <div className="docs-search-box">
                <Search size={15} />
                <input
                  type="search"
                  aria-label={text(
                    ...translatedMessage("search_docs_75d562b78e"),
                  )}
                  placeholder={text(
                    ...translatedMessage("search_docs_75d562b78e"),
                  )}
                  value={searchQuery}
                  onChange={(event) => setSearchQuery(event.target.value)}
                />
              </div>
              {groups.map((group) => {
                const matches = group.ids
                  .map((id) =>
                    filteredArticles.find((article) => article.id === id),
                  )
                  .filter((article) => article !== undefined);
                return matches.length ? (
                  <section className="docs-nav-group" key={group.title}>
                    <h2>{group.title}</h2>
                    {matches.map((article) => (
                      <button
                        key={article.id}
                        type="button"
                        className={activeTab === article.id ? "is-active" : ""}
                        onClick={() => selectArticle(article.id)}
                        aria-current={
                          activeTab === article.id ? "page" : undefined
                        }
                      >
                        {article.title}
                      </button>
                    ))}
                  </section>
                ) : null;
              })}
              {filteredArticles.length === 0 && (
                <p className="docs-empty-search" role="status">
                  {text(...translatedMessage("no_matching_docs_08bdf22fce"))}
                </p>
              )}
            </nav>
          </aside>
          <main ref={articleRef} className="docs-article">
            <div className="docs-breadcrumb">
              <span>{groupTitle}</span>
            </div>
            <h1 tabIndex={-1} id="docs-article-title">
              {current.title}
            </h1>
            {activeTab === "quickstart" ? (
              <Quickstart
                copy={copy}
                base={apiBase}
                CodeBlock={CodeBlock}
                connectionSettings={connectionSettings}
              />
            ) : (
              <div className="docs-article-settings">
                <details className="docs-base">
                  <summary>
                    {copy.connection}
                    <ChevronDown size={14} />
                  </summary>
                  {connectionSettings}
                </details>
                <details className="docs-prerequisites">
                  <summary>
                    {copy.beforeRunning}
                    <ChevronDown size={14} />
                  </summary>
                  <p>{copy.prerequisites}</p>
                  <Link className="docs-action" to="/docs">
                    {copy.quickLink}
                  </Link>
                </details>
              </div>
            )}
            {activeTab === "cursor" && (
              <>
                <p className="docs-lead">{cursorLead}</p>
                <ol className="docs-steps">
                  {cursorSteps.map((step) => (
                    <li key={step}>{step}</li>
                  ))}
                </ol>
                <CodeBlock
                  title="Cursor Settings → Models"
                  code={cursorSettings}
                />
                <div className="docs-callout">
                  <p>{cursorBoundary}</p>
                </div>
                <p>{cursorTroubleshooting}</p>
                <p>
                  <Link to="/docs?article=chat-completions">
                    {text(
                      ...translatedMessage(
                        "read_the_chat_completions_request_example_b538a08eac",
                      ),
                    )}
                  </Link>
                </p>
                <p>
                  <a
                    href="https://cursor.com/help/models-and-usage/api-keys"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "cursor_api_key_documentation_d0ffdba360",
                      ),
                    )}
                  </a>
                  {" · "}
                  <a
                    href="https://developers.cloudflare.com/ai-gateway/integrations/coding-agents/cursor/"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "custom_base_url_and_mode_compatibility_d4fb718236",
                      ),
                    )}
                  </a>
                </p>
              </>
            )}
            {activeTab === "gemini" && (
              <>
                <p className="docs-lead">{nativeGemini.lead}</p>
                <CodeBlock title="Gemini REST" code={nativeGemini.example} />
                <p>{nativeGemini.response}</p>
                <div className="docs-callout">
                  <p>{nativeGemini.streaming}</p>
                </div>
                <div className="docs-callout">
                  <p>{nativeGemini.boundary}</p>
                </div>
                <p>
                  <a
                    href="https://ai.google.dev/api/generate-content"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "official_gemini_api_reference_8920aaaa6f",
                      ),
                    )}
                  </a>
                </p>
                <h2>
                  {text(...translatedMessage("compatible_chat_api_48aea263f2"))}
                </h2>
                <p className="docs-lead">{geminiLead}</p>
                <CodeBlock
                  title="POST /v1/chat/completions"
                  code={geminiExample}
                />
                <div className="docs-callout">
                  <p>{geminiBoundary}</p>
                </div>
                <p>{geminiResponseNote}</p>
                <Link to="/docs?article=chat-completions">
                  {text(
                    ...translatedMessage(
                      "read_the_chat_completions_format_82f179dd35",
                    ),
                  )}
                </Link>
              </>
            )}
            {activeTab === "responses" && (
              <>
                <p className="docs-lead">{responses.lead}</p>
                <CodeBlock
                  title="POST /v1/responses"
                  code={responses.request}
                />
                <p>{responses.response}</p>
                <div className="docs-callout">
                  <p>{responses.streaming}</p>
                </div>
                <p>{responses.retrieval}</p>
                <CodeBlock
                  title="GET /v1/responses/{id}"
                  code={responses.retrieve}
                />
                <p>{responses.indexStatus}</p>
                <div className="docs-callout">
                  <p>{responses.boundary}</p>
                </div>
                <p>
                  <Link to="/docs?article=codex">{copy.codexTitle}</Link>
                </p>
                <p>
                  <a
                    href="https://developers.openai.com/api/docs/guides/streaming-responses"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "official_responses_event_guide_f9a76027f7",
                      ),
                    )}
                  </a>
                  {" · "}
                  <a
                    href="https://developers.openai.com/api/docs/guides/conversation-state"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "official_response_storage_guide_eebe4d6f0e",
                      ),
                    )}
                  </a>
                </p>
              </>
            )}
            {activeTab === "codex" && (
              <>
                <p className="docs-lead">{responses.codex}</p>
                <h2>{copy.codexConfigTitle}</h2>
                <CodeBlock title="config.toml" code={responses.config} />
                <div className="docs-callout">
                  <p>{responses.boundary}</p>
                </div>
                <p>
                  <Link to="/docs?article=responses">
                    {copy.responsesTitle}
                  </Link>
                  {" · "}
                  <a
                    href="https://learn.chatgpt.com/docs/config-file/config-advanced"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "official_codex_configuration_5bd50a465b",
                      ),
                    )}
                  </a>
                </p>
              </>
            )}
            {activeTab === "anthropic" && (
              <>
                <p className="docs-lead">{anthropicLead}</p>
                <CodeBlock title="POST /v1/messages" code={anthropicExample} />
                <div className="docs-callout">
                  <p>{anthropicAuth}</p>
                </div>
                <p>{anthropicResponseNote}</p>
                <CodeBlock
                  title={text(
                    ...translatedMessage("example_response_f13c4b99fb"),
                  )}
                  code={anthropicResponse}
                />
                <div className="docs-callout">
                  <p>{anthropicStreaming}</p>
                </div>
              </>
            )}
            {activeTab === "chat-completions" && (
              <>
                <p className="docs-lead">{chatLead}</p>
                <CodeBlock
                  title="POST /v1/chat/completions"
                  code={chatExample}
                />
                <p>{chatResponseNote}</p>
                <CodeBlock
                  title={text(
                    ...translatedMessage("example_response_f13c4b99fb"),
                  )}
                  code={chatResponse}
                />
                <div className="docs-callout">
                  <p>{chatStreaming}</p>
                </div>
              </>
            )}
            {activeTab === "models" && (
              <>
                <p className="docs-lead">{modelsLead}</p>
                <CodeBlock title="GET /v1/models" code={modelsExample} />
                <div className="docs-callout">
                  <p>{modelsScope}</p>
                </div>
                <p>{modelsResponseNote}</p>
                <CodeBlock
                  title={text(
                    ...translatedMessage("example_response_f13c4b99fb"),
                  )}
                  code={modelsResponse}
                />
              </>
            )}
            {activeTab === "auth" && (
              <>
                <p className="docs-lead">
                  {text(
                    ...translatedMessage(
                      "manage_keys_access_boundaries_and_usage_quota_21621afa4a",
                    ),
                  )}
                </p>
                <div className="docs-info-grid">
                  {[
                    [
                      text(
                        ...translatedMessage("store_keys_securely_959acec50b"),
                      ),
                      text(
                        ...translatedMessage(
                          "save_your_key_after_creation_and_load_it_thro_ceb9215891",
                        ),
                      ),
                    ],
                    [
                      text(
                        ...translatedMessage("set_project_quotas_82abd38a33"),
                      ),
                      policy.quota,
                    ],
                    [
                      text(...translatedMessage("restrict_access_414740ab02")),
                      text(
                        ...translatedMessage(
                          "use_an_address_allowlist_to_restrict_key_acce_d02976b2ab",
                        ),
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
                <p className="docs-lead">{claudeCode.lead}</p>
                <div className="docs-callout">
                  <BookOpen size={18} />
                  <span>{claudeCode.endpoint}</span>
                </div>
                <h2>
                  {text(
                    ...translatedMessage("environment_variables_eb5150f9ad"),
                  )}
                </h2>
                <CodeBlock
                  title={text(
                    ...translatedMessage("terminal_configuration_acba8f956e"),
                  )}
                  code={claudeCode.bash}
                />
                <CodeBlock title="PowerShell" code={claudeCode.powershell} />
                <CodeBlock
                  title="POST /v1/messages · cURL"
                  code={claudeCode.request}
                />
                <div className="docs-callout">
                  <p>{claudeCode.boundary}</p>
                </div>
                <p>
                  <a
                    href="https://code.claude.com/docs/en/llm-gateway-connect"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "official_claude_code_gateway_setup_b60bb42aa0",
                      ),
                    )}
                  </a>
                  {" · "}
                  <a
                    href="https://code.claude.com/docs/en/llm-gateway-protocol"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {text(
                      ...translatedMessage(
                        "official_protocol_and_capability_requirements_0a05baad4b",
                      ),
                    )}
                  </a>
                </p>
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
                    ...translatedMessage(
                      "choose_a_service_level_for_your_workload_the__01401586fb",
                    ),
                  )}
                </p>
                <div className="docs-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>{t("tier")}</th>
                        <th>
                          {text(
                            ...translatedMessage("price_multiplier_d2bec078c8"),
                          )}
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
                    ...translatedMessage(
                      "use_the_response_status_to_identify_issues_an_bc7cc2b25e",
                    ),
                  )}
                </p>
                <div className="docs-errors">
                  {[
                    [
                      "401",
                      "invalid_api_key",
                      text(
                        ...translatedMessage(
                          "the_api_key_is_invalid_or_not_found_check_the_5236ec3673",
                        ),
                      ),
                    ],
                    [
                      "401",
                      "token_disabled",
                      text(
                        ...translatedMessage(
                          "the_api_key_has_been_disabled_enable_it_in_ke_41ab60eedc",
                        ),
                      ),
                    ],
                    [
                      "401",
                      "token_expired",
                      text(
                        ...translatedMessage(
                          "the_api_key_has_expired_create_a_new_key_or_e_1ec2e03b6e",
                        ),
                      ),
                    ],
                    [
                      "403",
                      "insufficient_quota",
                      text(
                        ...translatedMessage(
                          "the_key_or_account_quota_is_insufficient_for__6a5f339535",
                        ),
                      ),
                    ],
                    [
                      "403",
                      "token_quota_exhausted",
                      text(
                        ...translatedMessage(
                          "the_key_quota_is_exhausted_check_remain_quota_39c382c34e",
                        ),
                      ),
                    ],
                    [
                      "403",
                      "auto_group_empty",
                      text(
                        ...translatedMessage(
                          "the_key_uses_auto_but_auto_groups_is_empty_co_1c488008d5",
                        ),
                      ),
                    ],
                    [
                      "403",
                      "group_not_available",
                      text(
                        ...translatedMessage(
                          "the_current_service_group_has_no_configured_b_89293aafe4",
                        ),
                      ),
                    ],
                    [
                      "400",
                      "invalid_request_body",
                      text(
                        ...translatedMessage(
                          "the_request_body_is_invalid_check_the_json_or_83bc9022c1",
                        ),
                      ),
                    ],
                    [
                      "400",
                      "invalid_json",
                      text(
                        ...translatedMessage(
                          "json_parsing_failed_ensure_the_body_is_valid__2c861b4696",
                        ),
                      ),
                    ],
                    [
                      "400",
                      "invalid_multipart",
                      text(
                        ...translatedMessage(
                          "multipart_form_parsing_failed_check_the_file__f8ef35006e",
                        ),
                      ),
                    ],
                    [
                      "400",
                      "model_required",
                      text(
                        ...translatedMessage(
                          "the_model_field_is_missing_specify_the_model__248e1eb623",
                        ),
                      ),
                    ],
                    [
                      "429",
                      "rate_limit_exceeded",
                      text(
                        ...translatedMessage(
                          "rate_limit_exceeded_reduce_concurrency_or_wai_1891ae67c5",
                        ),
                      ),
                    ],
                    [
                      "502",
                      "upstream_unavailable",
                      text(
                        ...translatedMessage(
                          "the_upstream_is_unavailable_other_channels_ar_7ea8b4054e",
                        ),
                      ),
                    ],
                    [
                      "502",
                      "retry_budget_exhausted",
                      text(
                        ...translatedMessage(
                          "the_retry_budget_is_exhausted_no_further_chan_77249b0e61",
                        ),
                      ),
                    ],
                    [
                      "502",
                      "upstream_result_uncertain",
                      text(
                        ...translatedMessage(
                          "the_upstream_result_is_uncertain_the_request__118e9e4c0c",
                        ),
                      ),
                    ],
                    [
                      "503",
                      "no_available_channel",
                      text(
                        ...translatedMessage(
                          "no_available_channel_serves_this_model_the_ch_9370f84378",
                        ),
                      ),
                    ],
                    [
                      "500",
                      "pre_consume_failed",
                      text(
                        ...translatedMessage(
                          "an_internal_error_occurred_during_the_quota_p_3b80658ca0",
                        ),
                      ),
                    ],
                    [
                      "500",
                      "channel_selection_failed",
                      text(
                        ...translatedMessage(
                          "an_internal_error_occurred_during_channel_sel_19ca0578b0",
                        ),
                      ),
                    ],
                  ].map(([status, code, description]) => (
                    <section key={code}>
                      <span>{status}</span>
                      <div>
                        <code>
                          {code.replaceAll(placeholders.apiBase, apiBase)}
                        </code>
                        <p>{description}</p>
                      </div>
                    </section>
                  ))}
                </div>
              </>
            )}
            {activeTab === "billing" && (
              <>
                <div className="docs-callout">
                  <p>{policy.wallet}</p>
                </div>
                <p className="docs-lead">
                  {text(
                    ...translatedMessage(
                      "balances_reports_and_request_details_use_diff_6be2daf57e",
                    ),
                  )}
                </p>
                <div className="docs-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>{text(...translatedMessage("view_06693661f2"))}</th>
                        <th>
                          {text(...translatedMessage("precision_3fd46f2944"))}
                        </th>
                        <th>
                          {text(...translatedMessage("example_8a74298562"))}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {[
                        [
                          text(
                            ...translatedMessage("account_balance_78cbcf6a48"),
                          ),
                          "2",
                          "$128.40",
                        ],
                        [
                          text(...translatedMessage("usage_totals_8b39152d46")),
                          "4",
                          "$14.2850",
                        ],
                        [
                          text(...translatedMessage("request_logs_392b15c1b3")),
                          "6",
                          "$0.001850",
                        ],
                        [
                          text(
                            ...translatedMessage("export_details_6898ccc4bd"),
                          ),
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
                    ...translatedMessage(
                      "the_platform_supports_multiple_api_protocols__1b397f2df1",
                    ),
                  )}
                </p>
                <div className="docs-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>
                          {text(...translatedMessage("protocol_ae04bb9db6"))}
                        </th>
                        <th>Base URL</th>
                        <th>{text(...translatedMessage("path_98e6113c71"))}</th>
                        <th>{text(...translatedMessage("auth_017799a122"))}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {[
                        [
                          text(
                            ...translatedMessage(
                              "chat_completions_openai_567028d138",
                            ),
                          ),
                          apiBase,
                          "/chat/completions",
                          "Bearer token",
                        ],
                        ["Responses", apiBase, "/responses", "Bearer token"],
                        [
                          "Gemini",
                          apiBase.replace(/\/v1\/?$/, ""),
                          "/v1beta/models/{model}:generateContent",
                          "x-goog-api-key / Bearer",
                        ],
                        [
                          text(
                            ...translatedMessage(
                              "messages_anthropic_6a87169a69",
                            ),
                          ),
                          apiBase,
                          "/messages",
                          "Bearer token",
                        ],
                        [
                          text(
                            ...translatedMessage(
                              "embeddings_openai_f32853996d",
                            ),
                          ),
                          apiBase,
                          "/embeddings",
                          "Bearer token",
                        ],
                        [
                          text(
                            ...translatedMessage("images_openai_51308b0b32"),
                          ),
                          apiBase,
                          "/images/generations",
                          "Bearer token",
                        ],
                        [
                          text(
                            ...translatedMessage("audio_tts_openai_f9f0acdd13"),
                          ),
                          apiBase,
                          "/audio/speech",
                          "Bearer token",
                        ],
                        [
                          text(
                            ...translatedMessage("audio_stt_openai_72b9b864b3"),
                          ),
                          apiBase,
                          "/audio/transcriptions",
                          "Bearer token",
                        ],
                        [
                          text(
                            ...translatedMessage(
                              "video_submit_poll_4c4b235424",
                            ),
                          ),
                          apiBase,
                          "/video/generations",
                          "Bearer token",
                        ],
                        [
                          text(
                            ...translatedMessage(
                              "realtime_websocket_ba741dc0ea",
                            ),
                          ),
                          apiBase,
                          "/realtime",
                          "Bearer token",
                        ],
                        [
                          text(...translatedMessage("models_list_be2128ce36")),
                          apiBase,
                          "/models",
                          "Bearer token",
                        ],
                      ].map(([protocol, baseUrl, path, auth], i) => (
                        <tr key={i}>
                          <td>{protocol}</td>
                          <td>
                            <code className="mono">{baseUrl}</code>
                          </td>
                          <td>
                            <code className="mono">{path}</code>
                          </td>
                          <td>{auth}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="docs-callout">
                  <p>
                    {text(
                      ...translatedMessage(
                        "all_protocols_use_unified_bearer_token_authen_8903eb6464",
                      ),
                    )}
                  </p>
                </div>
              </>
            )}
            {activeTab === "routing" && (
              <>
                <p className="docs-lead">
                  {text(
                    ...translatedMessage(
                      "model_authorization_and_routing_are_configure_f239e63197",
                    ),
                  )}
                </p>
                <div className="docs-callout">
                  <p>
                    {text(
                      ...translatedMessage(
                        "routing_flow_key_auth_user_quota_check_key_qu_396587f924",
                      ),
                    )}
                  </p>
                </div>
                <h2>
                  {text(
                    ...translatedMessage("key_authorization_scope_ef9b796bcf"),
                  )}
                </h2>
                <div
                  className="docs-table-wrap"
                  role="region"
                  tabIndex={0}
                  aria-label={text(
                    ...translatedMessage("key_authorization_fields_80dc9eccd3"),
                  )}
                >
                  <table>
                    <thead>
                      <tr>
                        <th>
                          {text(...translatedMessage("field_ee0fa11849"))}
                        </th>
                        <th>
                          {text(...translatedMessage("description_c6bae5d4e7"))}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {policy.tokenFields.map(([field, description]) => (
                        <tr key={field}>
                          <td>
                            <code>{field}</code>
                          </td>
                          <td>{description}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            )}
            {activeTab === "limits" && (
              <>
                <div className="docs-callout">
                  <p>{policy.defaults}</p>
                </div>
                <p className="docs-lead">
                  {text(
                    ...translatedMessage(
                      "the_platform_enforces_quota_and_rate_limits_o_e2dab9cbbb",
                    ),
                  )}
                </p>
                <div
                  className="docs-table-wrap"
                  role="region"
                  tabIndex={0}
                  aria-label={text(
                    ...translatedMessage("rate_limit_defaults_2f0b6de9d8"),
                  )}
                >
                  <table>
                    <thead>
                      <tr>
                        <th>
                          {text(...translatedMessage("limit_type_fec3a2027a"))}
                        </th>
                        <th>
                          {text(...translatedMessage("default_46411cfc2d"))}
                        </th>
                        <th>
                          {text(...translatedMessage("description_c6bae5d4e7"))}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {policy.rateRows.map(([label, value, description]) => (
                        <tr key={label}>
                          <td>{label}</td>
                          <td className="mono">{value}</td>
                          <td>{description}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="docs-callout">
                  <div style={{ display: "grid", gap: 12, minWidth: 0 }}>
                    <p>{policy.rateHelp}</p>
                    <p>{policy.quota}</p>
                  </div>
                </div>
              </>
            )}
            {activeTab === "faq" && (
              <>
                <p className="docs-lead">
                  {text(
                    ...translatedMessage(
                      "frequently_asked_questions_about_the_platform_14863842fc",
                    ),
                  )}
                </p>
                {[
                  {
                    q: text(
                      ...translatedMessage(
                        "where_can_i_see_the_plaintext_api_key_db78e800b0",
                      ),
                    ),
                    a: text(
                      ...translatedMessage(
                        "only_shown_once_at_creation_or_regeneration_a_67cc5b9508",
                      ),
                    ),
                  },
                  {
                    q: text(
                      ...translatedMessage(
                        "are_failed_requests_charged_124da96d16",
                      ),
                    ),
                    a: text(
                      ...translatedMessage(
                        "requests_rejected_before_admission_are_not_ch_716ebfd82e",
                      ),
                    ),
                  },
                  {
                    q: text(
                      ...translatedMessage("how_do_i_switch_models_3cea5fe46d"),
                    ),
                    a: text(
                      ...translatedMessage(
                        "for_openai_anthropic_change_the_body_model_fi_35e3c4bb3f",
                      ),
                    ),
                  },
                  {
                    q: text(
                      ...translatedMessage(
                        "how_long_does_top_up_take_3035ae673e",
                      ),
                    ),
                    a: policy.wallet,
                  },
                ].map(({ q, a }, i) => (
                  <details key={i} className="docs-callout">
                    <summary
                      style={{
                        cursor: "pointer",
                        fontWeight: 600,
                        fontSize: "var(--text-sm)",
                      }}
                    >
                      {q}
                    </summary>
                    <p
                      style={{
                        marginTop: 8,
                        fontSize: "var(--text-sm)",
                        color: "var(--secondary)",
                      }}
                    >
                      {a}
                    </p>
                  </details>
                ))}
              </>
            )}
            <Supplement
              article={activeTab}
              copy={copy}
              base={apiBase}
              CodeBlock={CodeBlock}
            />
            <nav className="docs-pagination" aria-label={t("navDocs")}>
              {previous ? (
                <button
                  type="button"
                  onClick={() => selectArticle(previous.id)}
                >
                  <small>{copy.previous}</small>
                  <span>{previous.title}</span>
                </button>
              ) : (
                <span />
              )}
              {next && (
                <button type="button" onClick={() => selectArticle(next.id)}>
                  <small>{copy.next}</small>
                  <span>{next.title}</span>
                </button>
              )}
            </nav>
            <footer className="docs-article-footer">
              {text(
                ...translatedMessage(
                  "myapis_developer_documentation_product_previe_4ed234572b",
                ),
              )}
            </footer>
          </main>
          <ArticleOutline
            article={activeTab}
            language={lang}
            articleRef={articleRef}
            title={copy.onPage}
          />
        </div>
      </div>
    </DocsCodeContext.Provider>
  );
}
