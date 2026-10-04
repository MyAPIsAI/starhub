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
  curl: `curl ${placeholders.apiBase}/chat/completions \\\n  -H "Authorization: Bearer $VEYRA_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "gpt-4.1",\n    "messages": [{"role": "user", "content": "Hello"}],\n    "stream": true\n  }'`,
  python: `import os\nfrom openai import OpenAI\n\nclient = OpenAI(\n    base_url="${placeholders.apiBase}",\n    api_key=os.environ["VEYRA_API_KEY"]\n)\n\nfor chunk in client.chat.completions.create(\n    model="gpt-4.1",\n    messages=[{"role": "user", "content": "Hello"}],\n    stream=True,\n):\n    print(chunk.choices[0].delta.content or "", end="")`,
  node: `import OpenAI from "openai";\n\nconst client = new OpenAI({\n  baseURL: "${placeholders.apiBase}",\n  apiKey: process.env.VEYRA_API_KEY,\n});\n\nconst stream = await client.chat.completions.create({\n  model: "gpt-4.1",\n  messages: [{ role: "user", content: "Hello" }],\n  stream: true,\n});\nfor await (const chunk of stream) {\n  process.stdout.write(chunk.choices[0]?.delta.content || "");\n}`,
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
  const labels = {
    curl: t("codeCurl"),
    python: t("codePython"),
    node: t("codeNode"),
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
        <span dir="ltr">POST /v1/chat/completions</span>
      </div>
    </Tabs.Root>
  );
}
