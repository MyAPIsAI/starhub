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
import { placeholders } from "../../config/placeholders";
import { responsesGuide } from "./responses-guide";
import { geminiNativeGuide } from "./gemini-guide";

type ArticleId =
  "quickstart" | "auth" | "tiers" | "claudeCode" | "cursor" | "errors" | "billing" | "protocols" | "models" | "chat-completions" | "responses" | "anthropic" | "gemini" | "routing" | "limits" | "faq";
const cursorSettings = `OpenAI API Key: <YOUR_MYAPIS_API_KEY>
Override OpenAI Base URL: ${placeholders.apiBase}
Custom Model: gpt-4.1`;
const geminiExample = `curl --fail --silent --show-error ${placeholders.apiBase}/chat/completions \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 64,
    "stream": false
  }'`;
const anthropicExample = `curl --fail --silent --show-error ${placeholders.apiBase}/messages \\
  -H "x-api-key: $MYAPIS_API_KEY" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-sonnet-4-20250514",
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
    "model": "gpt-4.1",
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
    choices: [{ index: 0, message: { role: "assistant", content: "Hello!" }, finish_reason: "stop" }],
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
  const responses = responsesGuide(text);
  const nativeGemini = geminiNativeGuide(text);
  const articles: { id: ArticleId; title: string; Icon: typeof Terminal }[] = [
    { id: "quickstart", title: t("docsQuickstart"), Icon: Terminal },
    { id: "auth", title: t("docsAuth"), Icon: KeyRound },
    { id: "tiers", title: t("docsTiers"), Icon: Cpu },
    { id: "protocols", title: text("协议", "Protocols", "البروتوكولات"), Icon: ArrowUpRight },
    { id: "chat-completions", title: text("Chat Completions（聊天补全）", "Chat Completions", "إكمال المحادثات"), Icon: Terminal },
    { id: "responses", title: text("Responses（响应接口）与 Codex", "Responses and Codex", "Responses وCodex"), Icon: Terminal },
    { id: "anthropic", title: text("Anthropic Messages（消息）", "Anthropic Messages", "رسائل Anthropic"), Icon: Terminal },
    { id: "gemini", title: text("Gemini（原生与兼容）", "Gemini native and compatible APIs", "Gemini الأصلي والمتوافق"), Icon: Terminal },
    { id: "models", title: text("模型列表", "Model list", "قائمة النماذج"), Icon: Cpu },
    { id: "claudeCode", title: t("docsClaudeCode"), Icon: Sparkles },
    { id: "cursor", title: text("Cursor 接入指南", "Cursor setup", "إعداد Cursor"), Icon: Terminal },
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
  const cursorLead = text(
    "在 Cursor 中使用自己的网关密钥接入兼容 Chat Completions 的聊天模型。请先创建具有目标模型权限的密钥，并确认该模型在所选服务分组中有可用渠道。",
    "Use your gateway key in Cursor with a Chat Completions-compatible chat model. First create a key authorized for the target model and confirm an available channel in the selected service group.",
    "استخدم مفتاح البوابة في Cursor مع نموذج محادثة متوافق مع Chat Completions. أنشئ أولًا مفتاحًا مصرحًا له بالنموذج المطلوب وتأكد من وجود قناة متاحة في مجموعة الخدمة المختارة.",
  );
  const cursorSteps = [
    text(
      "打开 Cursor Settings → Models，在 API Keys 中将自己的网关密钥填入 OpenAI API Key 并保存；下方占位符不能用于认证。",
      "Open Cursor Settings → Models. Under API Keys, enter your gateway key in OpenAI API Key and save it; the placeholder below is not a credential.",
      "افتح Cursor Settings → Models. ضمن API Keys أدخل مفتاح البوابة في OpenAI API Key واحفظه؛ العنصر النائب أدناه ليس مفتاحًا صالحًا.",
    ),
    text(
      `启用 Override OpenAI Base URL，填写 ${placeholders.apiBase}。保留末尾 /v1，不要追加 /chat/completions。若没有该选项，请核对 Cursor 版本、套餐及团队的自带密钥设置。`,
      `Enable Override OpenAI Base URL and enter ${placeholders.apiBase}. Keep the trailing /v1; do not append /chat/completions. If the option is missing, check your Cursor version, plan, and team settings for custom keys.`,
      `فعّل Override OpenAI Base URL وأدخل ${placeholders.apiBase}. احتفظ باللاحقة /v1 ولا تضف /chat/completions. إن غاب الخيار فتحقق من إصدار Cursor والخطة وإعدادات الفريق للمفاتيح الخاصة.`,
    ),
    text(
      "使用 Add Custom Model 添加并启用已授权的准确模型标识（下方 gpt-4.1 仅为示例），然后在聊天模型选择器中选中它，发送一条简短消息。",
      "Use Add Custom Model to add and enable the exact authorized model ID (gpt-4.1 below is an example), then select it in the chat model picker and send a short message.",
      "استخدم Add Custom Model لإضافة معرّف النموذج المصرح به بدقة وتفعيله (gpt-4.1 أدناه مثال)، ثم اختره من قائمة نماذج المحادثة وأرسل رسالة قصيرة.",
    ),
  ];
  const cursorBoundary = text(
    "该指南覆盖兼容聊天配置；网关已有前台 Responses（响应接口），其支持范围见对应指南，Cursor Agent 尚待真实客户端验收。Tab 补全使用 Cursor 自带模型。Cursor 请求经其服务器转发，网关地址须可从公网访问。",
    "This guide covers compatible chat configuration. The gateway also supports foreground Responses; see its guide for supported features. Cursor Agent still requires real-client verification. Tab completion uses Cursor's own models. Cursor routes requests through its servers, so the gateway must be publicly reachable.",
    "يغطي الدليل إعداد المحادثة المتوافقة. تدعم البوابة أيضًا Responses الأمامية؛ راجع دليلها للقدرات المتاحة. لا يزال Cursor Agent بحاجة إلى التحقق بالعميل الفعلي. يستخدم إكمال Tab نماذج Cursor الخاصة. تمر الطلبات عبر خوادم Cursor، ولذلك يجب أن تكون البوابة متاحة عبر الإنترنت.",
  );
  const cursorTroubleshooting = text(
    "连接失败时先用 Chat Completions 指南的 cURL 验证同一密钥、地址和模型：401 检查密钥，403 检查授权或额度，503 检查渠道。cURL 成功但 Cursor 失败时，再核对自带密钥开关、自定义地址和所选模式。",
    "If connection fails, first test the same key, base URL, and model with the Chat Completions guide's cURL: check the key for 401, authorization or quota for 403, and channels for 503. If cURL succeeds but Cursor fails, check the custom-key toggle, base URL override, and selected mode.",
    "إذا فشل الاتصال فاختبر أولًا المفتاح والعنوان والنموذج نفسيهما باستخدام cURL في دليل Chat Completions: تحقق من المفتاح عند 401، والتفويض أو الحصة عند 403، والقنوات عند 503. إذا نجح cURL وفشل Cursor فتحقق من تفعيل المفتاح الخاص وتجاوز العنوان والوضع المختار.",
  );
  const geminiLead = text(
    "调用 Gemini 文本模型时，可使用网关的 POST /v1/chat/completions 兼容入口。示例使用 Bearer 密钥、gemini-2.5-pro 模型和 messages；实际调用仍要求密钥授权及匹配的可用渠道。",
    "Call Gemini text models through the gateway's compatible POST /v1/chat/completions endpoint. The example uses a Bearer key, gemini-2.5-pro, and messages; calls still require key authorization and a matching available channel.",
    "استدعِ نماذج Gemini النصية عبر نقطة POST /v1/chat/completions المتوافقة في البوابة. يستخدم المثال مفتاح Bearer ونموذج gemini-2.5-pro وحقل messages؛ وتظل المكالمات بحاجة إلى تفويض المفتاح وقناة متاحة مطابقة.",
  );
  const geminiBoundary = text(
    "兼容聊天入口继续使用 messages 与 Bearer 密钥；原生 contents 和 x-goog-api-key 用于上方 /v1beta/models 路径。请选择与请求格式匹配的入口。",
    "The compatible chat endpoint uses messages and a Bearer key. Native contents and x-goog-api-key use the /v1beta/models path above. Choose the endpoint that matches your request format.",
    "تستخدم نقطة المحادثة المتوافقة messages ومفتاح Bearer. يستخدم contents الأصلي وx-goog-api-key مسار /v1beta/models أعلاه. اختر النقطة المطابقة لصيغة طلبك.",
  );
  const geminiResponseNote = text(
    "stream: false 按 Chat Completions 的 choices[].message.content 读取文本，而非 Gemini 原生 candidates 结构。设置 stream: true 时同样按 Chat Completions 的 SSE 格式读取增量内容。",
    "With stream: false, read text from Chat Completions choices[].message.content, not the native Gemini candidates structure. With stream: true, read incremental content using the Chat Completions SSE format as well.",
    "مع stream: false اقرأ النص من choices[].message.content في Chat Completions وليس من بنية candidates الأصلية لـ Gemini. ومع stream: true اقرأ المحتوى التدريجي بصيغة SSE الخاصة بـ Chat Completions أيضًا.",
  );
  const anthropicLead = text(
    "POST /v1/messages 使用 Anthropic 消息格式。示例提供 model、messages、max_tokens 和 stream: false；system 放在顶层，用户消息放在 messages 数组中。请选择密钥已授权且支持该协议的模型。",
    "POST /v1/messages uses the Anthropic message format. The example provides model, messages, max_tokens, and stream: false, with a top-level system field and user messages in the messages array. Choose an authorized model that supports this protocol.",
    "يستخدم POST /v1/messages صيغة رسائل Anthropic. يوفر المثال model وmessages وmax_tokens وstream: false، مع حقل system في المستوى الأعلى ورسائل المستخدم في مصفوفة messages. اختر نموذجًا مصرحًا به يدعم هذا البروتوكول.",
  );
  const anthropicAuth = text(
    "示例使用 x-api-key，并携带 anthropic-version。网关也接受 Authorization: Bearer 认证，选择一种认证方式即可；密钥的模型范围、分组及额度限制仍然生效。",
    "The example uses x-api-key and includes anthropic-version. The gateway also accepts Authorization: Bearer; use one authentication style. Key model scope, service group, and quota limits still apply.",
    "يستخدم المثال x-api-key ويتضمن anthropic-version. تقبل البوابة أيضًا Authorization: Bearer؛ استخدم طريقة مصادقة واحدة. تظل قيود النماذج ومجموعة الخدمة والحصة سارية.",
  );
  const anthropicResponseNote = text(
    "非流式响应的文本位于 content 数组中 type 为 text 的块的 text 字段，stop_reason 表示结束原因。下方仅为格式示例；这不是 Chat Completions 的 choices 结构。",
    "In a non-streaming response, read text from the text field of content blocks whose type is text. stop_reason describes why generation ended. The example below illustrates the format; it is not the Chat Completions choices structure.",
    "في الاستجابة غير المتدفقة اقرأ النص من حقل text في كتل content التي يكون نوعها text. يوضح stop_reason سبب انتهاء التوليد. المثال أدناه يوضح الصيغة؛ وهي تختلف عن بنية choices في Chat Completions.",
  );
  const anthropicStreaming = text(
    "设置 stream: true 后按 SSE 事件处理：在 content_block_delta 中读取 delta.text，以 message_stop 结束。不要按 Chat Completions 的 choices[].delta.content 或 [DONE] 标记解析该协议。",
    "With stream: true, handle SSE events: read delta.text from content_block_delta and finish at message_stop. Do not parse this protocol using Chat Completions choices[].delta.content or the [DONE] marker.",
    "مع stream: true عالج أحداث SSE: اقرأ delta.text من content_block_delta وانتهِ عند message_stop. لا تستخدم choices[].delta.content أو علامة [DONE] الخاصة بـ Chat Completions لتحليل هذا البروتوكول.",
  );
  const chatLead = text(
    "POST /v1/chat/completions 接收 model 与 messages。下方使用已授权的文本模型、用户消息和 max_tokens 输出上限，发送一个非流式请求。",
    "POST /v1/chat/completions accepts model and messages. The example sends a non-streaming request using an authorized text model, a user message, and a max_tokens output limit.",
    "يستقبل POST /v1/chat/completions حقلي model وmessages. يرسل المثال طلبًا غير متدفق باستخدام نموذج نصي مصرح به ورسالة مستخدم وحد إخراج max_tokens.",
  );
  const chatResponseNote = text(
    "stream: false 返回完整 JSON，文本结果位于 choices[].message.content，finish_reason 表示结束原因。下方仅为响应格式示例，实际内容由模型生成。",
    "stream: false returns a complete JSON response. Read text from choices[].message.content and the completion reason from finish_reason. The response below is illustrative; actual content is generated by the model.",
    "تعيد stream: false استجابة JSON كاملة. اقرأ النص من choices[].message.content وسبب الانتهاء من finish_reason. الاستجابة أدناه توضيحية؛ يولد النموذج المحتوى الفعلي.",
  );
  const chatStreaming = text(
    "需要流式输出时设置 stream: true，按 text/event-stream 的 data: 事件读取增量内容 choices[].delta.content，遇到 data: [DONE] 结束。不要将整段事件流作为单个 JSON 解析；模型协议、能力及密钥授权仍须匹配。",
    "For streaming, set stream: true and read incremental content from choices[].delta.content in text/event-stream data: events until data: [DONE]. Do not parse the entire event stream as one JSON value. The model protocol, capabilities, and key authorization must still match.",
    "للبث اضبط stream: true واقرأ المحتوى التدريجي من choices[].delta.content في أحداث data: من نوع text/event-stream حتى data: [DONE]. لا تحلل تدفق الأحداث كله كقيمة JSON واحدة. يجب أن يتوافق بروتوكول النموذج وقدراته وتفويض المفتاح.",
  );
  const modelsLead = text(
    "GET /v1/models 返回当前密钥所用服务分组中已启用的模型标识。调用模型时，将选定的 data[].id 填入请求的 model 字段。",
    "GET /v1/models lists enabled model IDs in the service group used by the current key. Set the request's model field to the selected data[].id.",
    "يعرض GET /v1/models معرّفات النماذج المفعّلة في مجموعة الخدمة التي يستخدمها المفتاح الحالي. ضع قيمة data[].id المختارة في حقل model للطلب.",
  );
  const modelsScope = text(
    "列表按服务分组生成，不会按密钥的模型白名单逐项过滤。出现在列表中不表示该密钥获准调用，也不保证请求时有可用渠道；实际请求仍须通过授权与运行状态检查。",
    "The list is scoped to the service group, without filtering each entry against the key's model allowlist. A listed model does not guarantee key authorization or channel availability; requests still undergo authorization and runtime checks.",
    "تقتصر القائمة على مجموعة الخدمة ولا تُصفّى حسب قائمة النماذج المسموح بها للمفتاح. ظهور نموذج لا يضمن تفويض المفتاح أو توفر قناة؛ تظل الطلبات خاضعة لفحوص التفويض وحالة التشغيل.",
  );
  const modelsResponseNote = text(
    "下方 JSON 仅为格式示例，实际 data 可能为空。owned_by 是网关标记，created: 0 不是模型发布日期。",
    "The JSON below illustrates the response format; data may be empty. owned_by is a gateway label, and created: 0 is not the model's release date.",
    "يوضح JSON أدناه صيغة الاستجابة فقط وقد تكون data فارغة. الحقل owned_by هو تسمية للبوابة، وcreated: 0 ليس تاريخ إصدار النموذج.",
  );

  // Article search: full-text match against title, keywords, and article body.
  const articleKeywords: Record<ArticleId, string[]> = {
    quickstart: ["curl", "python", "node", "api key", "base url", "getting started", "快速开始", "入门", "创建密钥"],
    auth: ["authentication", "authorization", "bearer", "token", "api key", "认证", "授权", "密钥", "bearer"],
    tiers: ["service tier", "group", "ultra", "pro", "standard", "lite", "分组", "等级", "服务"],
    claudeCode: ["claude", "anthropic", "codex", "cursor", "gemini", "工具集成", "claude code", "编程工具"],
    cursor: ["cursor setup", "Cursor 接入指南", "إعداد Cursor"],
    errors: ["error", "400", "401", "403", "429", "502", "503", "错误", "错误码", "故障排除"],
    billing: ["billing", "quota", "cost", "charge", "balance", "top-up", "计费", "费用", "余额", "充值"],
    protocols: ["protocol", "endpoint", "api", "curl", "url", "path", "协议", "端点", "路径", "接口"],
    models: ["model list", "/v1/models", "模型列表", "قائمة النماذج"],
    "chat-completions": ["chat completions", "/v1/chat/completions", "聊天补全", "إكمال المحادثات"],
    responses: ["responses", "codex", "/v1/responses", "响应接口"],
    anthropic: ["anthropic messages", "/v1/messages", "消息协议", "رسائل Anthropic"],
    gemini: ["gemini compatibility", "Gemini 兼容接入", "الوصول المتوافق إلى Gemini"],
    routing: ["routing", "authorization", "group", "model", "allowlist", "auto", "路由", "授权", "分组", "白名单"],
    limits: ["limit", "rate", "quota", "concurrency", "throttle", "配额", "限流", "并发", "额度"],
    faq: ["faq", "question", "answer", "常见问题", "问题", "解答"],
  };
  // Searchable body text for each article, built from the article content so
  // the search covers the full text, not just preset keywords.
  const articleBody: Record<ArticleId, string> = {
    quickstart: "first request endpoint key curl python streaming demo domain environment variable chat request terminal connect application consistent interface",
    auth: "keys access boundaries usage quotas store securely project restrict access IP allowlist source control environment variables review request logs",
    tiers: "service level workload Ultra 1.00 99.5% 800ms Pro 0.80 99.0% 1500ms Standard 0.60 97.5% 3000ms Lite 0.40 price multiplier target latency demo configuration",
    claudeCode: "Claude Code Anthropic endpoint key environment variables ANTHROPIC_BASE_URL ANTHROPIC_API_KEY terminal configuration demo compatibility",
    cursor: [cursorLead, ...cursorSteps, cursorSettings, cursorBoundary, cursorTroubleshooting].join(" "),
    errors: "401 invalid_api_key token_disabled token_expired 403 insufficient_quota token_quota_exhausted auto_group_empty group_not_available 400 invalid_request_body invalid_json invalid_multipart model_required 429 rate_limit_exceeded 502 upstream_unavailable retry_budget_exhausted upstream_result_uncertain 503 no_available_channel 500 pre_consume_failed channel_selection_failed Authorization Bearer JSON multipart",
    billing: "balance precision usage totals request logs export details account balance 2 4 6 9 display reconciliation",
    protocols: "Chat Completions OpenAI Messages Anthropic Embeddings Images generations Audio TTS speech STT transcriptions translations Video submit poll Realtime WebSocket Models list Bearer token base URL path auth",
    models: [modelsLead, modelsScope, modelsResponseNote, modelsExample, modelsResponse].join(" "),
    "chat-completions": [chatLead, chatResponseNote, chatStreaming, chatExample, chatResponse].join(" "),
    responses: Object.values(responses).join(" "),
    anthropic: [anthropicLead, anthropicAuth, anthropicResponseNote, anthropicStreaming, anthropicExample, anthropicResponse].join(" "),
    gemini: [geminiLead, geminiBoundary, geminiResponseNote, geminiExample, ...Object.values(nativeGemini)].join(" "),
    routing: "routing flow key auth user quota check channel selection group model match pre-charge upstream call settle refund group model_limit allowed_ips remain_quota CIDR IP allowlist authorized model list",
    limits: "user rate limit 100 req/min key 50 model 500 per-request cap 2000000 quota pre-charge malicious large requests 429 rate_limit_exceeded 403 insufficient_quota",
    faq: "plaintext API key creation regeneration mask sk-**** failed requests charged pre-charged quota refunded switch models model field top-up credited immediately balance",
  };
  const filteredArticles = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return articles;
    return articles.filter(({ id, title }) => {
      const keywords = articleKeywords[id] || [];
      const body = articleBody[id] || "";
      return (
        title.toLowerCase().includes(q) ||
        body.toLowerCase().includes(q) ||
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
      <pre dir="ltr" tabIndex={0}>
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
          {activeTab === "cursor" && (
            <>
              <p className="docs-lead">{cursorLead}</p>
              <ol className="docs-steps">{cursorSteps.map((step) => <li key={step}>{step}</li>)}</ol>
              <CodeBlock title="Cursor Settings → Models" code={cursorSettings} />
              <div className="docs-callout"><p>{cursorBoundary}</p></div>
              <p>{cursorTroubleshooting}</p>
              <p><Link to="/docs?article=chat-completions">{text("查看 Chat Completions 请求示例", "Read the Chat Completions request example", "اقرأ مثال طلب Chat Completions")}</Link></p>
              <p>
                <a href="https://cursor.com/help/models-and-usage/api-keys" target="_blank" rel="noreferrer">{text("Cursor 官方密钥说明", "Cursor API key documentation", "وثائق مفاتيح Cursor")}</a>
                {" · "}
                <a href="https://developers.cloudflare.com/ai-gateway/integrations/coding-agents/cursor/" target="_blank" rel="noreferrer">{text("自定义地址与模式兼容说明", "Custom base URL and mode compatibility", "توافق العنوان المخصص والأوضاع")}</a>
              </p>
            </>
          )}
          {activeTab === "gemini" && (
            <>
              <p className="docs-lead">{nativeGemini.lead}</p>
              <CodeBlock title="Gemini REST" code={nativeGemini.example} />
              <p>{nativeGemini.response}</p>
              <div className="docs-callout"><p>{nativeGemini.streaming}</p></div>
              <div className="docs-callout"><p>{nativeGemini.boundary}</p></div>
              <p><a href="https://ai.google.dev/api/generate-content" target="_blank" rel="noreferrer">{text("Gemini 官方接口参考", "Official Gemini API reference", "مرجع Gemini الرسمي")}</a></p>
              <h2>{text("兼容聊天接入", "Compatible chat API", "واجهة المحادثة المتوافقة")}</h2>
              <p className="docs-lead">{geminiLead}</p>
              <CodeBlock title="POST /v1/chat/completions" code={geminiExample} />
              <div className="docs-callout">
                <p>{geminiBoundary}</p>
              </div>
              <p>{geminiResponseNote}</p>
              <Link to="/docs?article=chat-completions">
                {text("查看 Chat Completions 格式", "Read the Chat Completions format", "اقرأ صيغة Chat Completions")}
              </Link>
            </>
          )}
          {activeTab === "responses" && (
            <>
              <p className="docs-lead">{responses.lead}</p>
              <CodeBlock title="POST /v1/responses" code={responses.request} />
              <p>{responses.response}</p>
              <div className="docs-callout"><p>{responses.streaming}</p></div>
              <div className="docs-callout"><p>{responses.boundary}</p></div>
              <h2>Codex</h2>
              <p>{responses.codex}</p>
              <CodeBlock title="config.toml" code={responses.config} />
              <p>
                <a href="https://developers.openai.com/api/docs/guides/streaming-responses" target="_blank" rel="noreferrer">{text("Responses 官方事件说明", "Official Responses event guide", "دليل أحداث Responses الرسمي")}</a>
                {" · "}
                <a href="https://learn.chatgpt.com/docs/config-file/config-advanced" target="_blank" rel="noreferrer">{text("Codex 官方配置说明", "Official Codex configuration", "إعداد Codex الرسمي")}</a>
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
                title={text("响应格式示例", "Example response", "مثال على الاستجابة")}
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
              <CodeBlock title="POST /v1/chat/completions" code={chatExample} />
              <p>{chatResponseNote}</p>
              <CodeBlock
                title={text("响应格式示例", "Example response", "مثال على الاستجابة")}
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
                title={text("响应格式示例", "Example response", "مثال على الاستجابة")}
                code={modelsResponse}
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
                      ["Responses", placeholders.apiBase, "/responses", "Bearer token"],
                      ["Gemini", placeholders.apiBase.replace(/\/v1\/?$/, ""), "/v1beta/models/{model}:generateContent", "x-goog-api-key / Bearer"],
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
                  a: text("请求在准入前被拒绝不扣费。已开始执行的请求可能按已产生的用量结算，包括 Responses 失败、取消或流中断；未使用的预扣差额会退还。请以调用日志为准。", "Requests rejected before admission are not charged. Requests that started execution may settle incurred usage, including failed, cancelled, or interrupted Responses. Unused reservations are refunded. Check request logs for the final bill.", "لا تُحاسب الطلبات المرفوضة قبل القبول. قد تُحاسب الطلبات التي بدأ تنفيذها حسب الاستخدام، بما فيها Responses الفاشلة أو الملغاة أو المنقطعة. تُسترد الحصة المحجوزة غير المستخدمة. راجع سجلات الطلبات للفاتورة النهائية."),
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
