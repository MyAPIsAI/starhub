import { placeholders } from "../../config/placeholders";

type Text = (zh: string, en: string, ar: string) => string;

export function claudeCodeGuide(text: Text) {
  const base = placeholders.apiBase.replace(/\/v1\/?$/, "");
  return {
    lead: text(
      "Claude Code 通过 Anthropic Messages（消息接口）连接网关。先在客户端中选择密钥已授权、渠道实际支持的 Claude 模型；下面使用占位模型名，不代表模型可用性。",
      "Claude Code connects through Anthropic Messages. Select a Claude model authorized by your key and supported by an actual channel. The model placeholder below does not establish availability.",
      "يتصل Claude Code عبر Anthropic Messages. اختر نموذج Claude مصرحًا لمفتاحك وتدعمه قناة فعلية. لا يعني اسم النموذج النائب أدناه توفره.",
    ),
    bash: `export ANTHROPIC_BASE_URL="${base}"
export ANTHROPIC_AUTH_TOKEN="$MYAPIS_API_KEY"

claude --model "YOUR_AUTHORIZED_CLAUDE_MODEL"`,
    powershell: `$env:ANTHROPIC_BASE_URL = "${base}"
$env:ANTHROPIC_AUTH_TOKEN = $env:MYAPIS_API_KEY

claude --model "YOUR_AUTHORIZED_CLAUDE_MODEL"`,
    request: `curl --fail --silent --show-error "$ANTHROPIC_BASE_URL/v1/messages" \\
  -H "Authorization: Bearer $ANTHROPIC_AUTH_TOKEN" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"YOUR_AUTHORIZED_CLAUDE_MODEL","max_tokens":64,"messages":[{"role":"user","content":"Say hello"}]}'`,
    endpoint: text(
      "ANTHROPIC_BASE_URL 使用网关根地址，不追加 /v1；客户端请求路径为 /v1/messages，避免重复成 /v1/v1/messages。先设置 MYAPIS_API_KEY 环境变量；ANTHROPIC_AUTH_TOKEN 发送 Bearer 认证，ANTHROPIC_API_KEY 则发送 x-api-key，本网关均接受。示例只作用于当前终端。",
      "ANTHROPIC_BASE_URL is the gateway root, without /v1. Requests use /v1/messages, avoiding /v1/v1/messages. Set MYAPIS_API_KEY first. ANTHROPIC_AUTH_TOKEN sends Bearer auth; ANTHROPIC_API_KEY sends x-api-key. This gateway accepts both. These examples affect the current shell only.",
      "استخدم جذر البوابة في ANTHROPIC_BASE_URL دون /v1. مسار الطلب /v1/messages لتجنب تكراره. اضبط MYAPIS_API_KEY أولًا. يرسل ANTHROPIC_AUTH_TOKEN مصادقة Bearer ويرسل ANTHROPIC_API_KEY ترويسة x-api-key، وتقبل البوابة كليهما. الأمثلة تخص الطرفية الحالية فقط.",
    ),
    boundary: text(
      "本地已验证 Messages 的 JSON/SSE 传输，但真实 Claude Code 客户端、工具回合和后台模型仍待联调。目前未开放 /v1/messages/count_tokens，且不转发 anthropic-beta，上游版本固定为 2023-06-01；依赖 beta 标志的客户端功能不能视为可用。此配置不是完整兼容性验收。",
      "Messages JSON/SSE transport is locally tested; real Claude Code, tool turns and background models still need integration testing. /v1/messages/count_tokens is unavailable, anthropic-beta is not forwarded, and the upstream version is fixed at 2023-06-01. Features requiring beta flags are not established as working. This setup is not full compatibility acceptance.",
      "اختُبر نقل Messages بصيغتي JSON وSSE محليًا؛ ما زال العميل الفعلي وجولات الأدوات والنماذج الخلفية بحاجة إلى اختبار تكامل. لا تتوفر count_tokens ولا تُمرر anthropic-beta، وإصدار المنبع ثابت عند 2023-06-01. لا يُثبت هذا الإعداد توافق الميزات المعتمدة على beta أو التوافق الكامل.",
    ),
  };
}
