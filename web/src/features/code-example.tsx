import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Copy, Check } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../components/ui/button";
import * as Tabs from "@radix-ui/react-tabs";
import { placeholders } from "../config/placeholders";

export async function copyText(
  value: string,
  success: string,
  failure: string,
) {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(success);
    return true;
  } catch {
    toast.error(failure);
    return false;
  }
}
const samples: Record<string, string> = {
  audio: `# Use a speech model allowed by your API key.
curl --fail --silent --show-error ${placeholders.apiBase}/audio/speech \\
  -H "Authorization: Bearer $VEYRA_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "tts-1",
    "input": "Hello from MyAPIs!",
    "voice": "alloy",
    "response_format": "mp3"
  }' \\
  --output speech.mp3`,
  curl: `curl ${placeholders.apiBase}/chat/completions \\\n  -H "Authorization: Bearer $VEYRA_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "gpt-4.1",\n    "messages": [{"role": "user", "content": "Hello"}],\n    "stream": true\n  }'`,
  python: `import os\nfrom openai import OpenAI\n\nclient = OpenAI(\n    base_url="${placeholders.apiBase}",\n    api_key=os.environ["VEYRA_API_KEY"]\n)\n\nfor chunk in client.chat.completions.create(\n    model="gpt-4.1",\n    messages=[{"role": "user", "content": "Hello"}],\n    stream=True,\n):\n    print(chunk.choices[0].delta.content or "", end="")`,
  node: `import OpenAI from "openai";\n\nconst client = new OpenAI({\n  baseURL: "${placeholders.apiBase}",\n  apiKey: process.env.VEYRA_API_KEY,\n});\n\nconst stream = await client.chat.completions.create({\n  model: "gpt-4.1",\n  messages: [{ role: "user", content: "Hello" }],\n  stream: true,\n});\nfor await (const chunk of stream) {\n  process.stdout.write(chunk.choices[0]?.delta.content || "");\n}`,
  "claude-code": `# Claude Code integration\nexport ANTHROPIC_BASE_URL="${placeholders.apiBase}"\nexport ANTHROPIC_AUTH_TOKEN="$VEYRA_API_KEY"\nexport ANTHROPIC_MODEL="claude-sonnet-4-20250514"\n\n# Then run Claude Code in your project directory\nclaude`,
  cursor: `# Cursor configuration\n# Settings → Models → OpenAI API Key\nOpenAI API Key: $VEYRA_API_KEY\nOpenAI Base URL: ${placeholders.apiBase}\n\n# Add authorized model identifiers in the Models tab:\n# gpt-4.1\n# claude-sonnet-4-20250514\n# gemini-2.5-pro`,
  gemini: `# Gemini native API\ncurl "${placeholders.apiBase.replace(/\/v1\/?$/, "")}/v1beta/models/gemini-2.5-pro:generateContent" \\\n  -H "x-goog-api-key: $VEYRA_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "contents": [{"parts": [{"text": "Hello"}]}]\n  }'`,
};
function highlight(code: string) {
  const regex =
    /("[^"\n]*"|'[^'\n]*'|\b(?:import|from|const|await|for|in|true|True|new|print)\b|\b\d+\b)/g;
  return code.split(regex).map((part, i) => (
    <span
      key={i}
      className={
        /^["']/.test(part)
          ? "code-string"
          : /^(import|from|const|await|for|in|true|True|new|print)$/.test(part)
            ? "code-key"
            : /^\d+$/.test(part)
              ? "code-number"
              : undefined
      }
    >
      {part}
    </span>
  ));
}
export function CodeExample() {
  const { t, i18n } = useTranslation();
  const [language, setLanguage] = useState("curl");
  const [copied, setCopied] = useState(false);
  const labels: Record<string, string> = {
    curl: t("codeCurl"),
    python: t("codePython"),
    node: t("codeNode"),
    "claude-code": i18n.language === "zh" ? "Claude Code" : "Claude Code",
    cursor: "Cursor",
    gemini: i18n.language === "zh" ? "Gemini" : "Gemini",
    audio: t("codeAudio"),
  };
  return (
    <Tabs.Root
      className="code-panel"
      value={language}
      onValueChange={(value) => {
        setLanguage(value);
        setCopied(false);
      }}
      dir={i18n.language === "ar" ? "rtl" : "ltr"}
    >
      <div className="code-toolbar">
        <Tabs.List className="segmented" aria-label={t("codeSample")}>
          {Object.entries(labels).map(([value, label]) => (
            <Tabs.Trigger key={value} value={value}>
              {label}
            </Tabs.Trigger>
          ))}
        </Tabs.List>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("copy")}
          onClick={async () => {
            if (await copyText(samples[language], t("copied"), t("copyFailed")))
              setCopied(true);
          }}
        >
          {copied ? <Check /> : <Copy />}
        </Button>
      </div>
      {Object.entries(samples).map(([value, sample]) => (
        <Tabs.Content
          key={value}
          value={value}
          forceMount
          hidden={language !== value}
        >
          <pre dir="ltr" tabIndex={0} aria-label={t("codeSample")}>
            <code>{highlight(sample)}</code>
          </pre>
        </Tabs.Content>
      ))}
      <div className="code-caption">
        <span className="status-dot" />
        <span dir="ltr">
          {language === "audio"
            ? "POST /v1/audio/speech"
            : "POST /v1/chat/completions"}
        </span>
      </div>
    </Tabs.Root>
  );
}
