import { message as translatedMessage } from "../../i18n/messages";
import { placeholders } from "../../config/placeholders";

type Text = (zh: string, en: string, ar: string) => string;

export function claudeCodeGuide(text: Text) {
  const base = placeholders.apiBase.replace(/\/v1\/?$/, "");
  return {
    lead: text(
      ...translatedMessage(
        "claude_code_connects_through_anthropic_messag_e94660748b",
      ),
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
      ...translatedMessage(
        "anthropic_base_url_is_the_gateway_root_withou_eb10f09121",
      ),
    ),
    boundary: text(
      ...translatedMessage(
        "messages_json_sse_transport_is_locally_tested_937f09a572",
      ),
    ),
  };
}
