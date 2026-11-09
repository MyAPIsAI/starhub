import type { PlazaRow } from "../../lib/api/plaza";
import { placeholders } from "../../config/placeholders";
import { bashQuote } from "../docs/examples";

export function catalogExamples(row: PlazaRow) {
  const base = placeholders.apiBase;
  const model = row.id;
  const protocols: {
    id: string;
    label: string;
    url: string;
    body: unknown;
    header?: string;
  }[] = [];
  const messages = [{ role: "user", content: "Hello" }];
  if (row.protocols.includes("OpenAI") && row.tags.includes("text")) {
    protocols.push({
      id: "chat",
      label: "OpenAI Chat Completions",
      url: `${base}/chat/completions`,
      body: { model, messages, max_tokens: 64 },
    });
  }
  if (row.protocols.includes("Responses") && row.tags.includes("text")) {
    protocols.push({
      id: "responses",
      label: "OpenAI Responses",
      url: `${base}/responses`,
      body: { model, input: "Hello", max_output_tokens: 64 },
    });
  }
  if (row.protocols.includes("Anthropic") && row.tags.includes("text"))
    protocols.push({
      id: "anthropic",
      label: "Anthropic Messages",
      url: `${base}/messages`,
      body: { model, messages, max_tokens: 64 },
      header: "anthropic",
    });
  if (row.protocols.includes("Gemini") && row.tags.includes("text"))
    protocols.push({
      id: "gemini",
      label: "Gemini",
      url: `${base.replace(/\/v1$/, "")}/v1beta/models/${encodeURIComponent(model)}:generateContent`,
      body: { contents: [{ parts: [{ text: "Hello" }] }] },
      header: "gemini",
    });
  if (row.protocols.includes("OpenAI") && row.tags.includes("embedding"))
    protocols.push({
      id: "embedding",
      label: "OpenAI Embeddings",
      url: `${base}/embeddings`,
      body: { model, input: "Hello" },
    });
  if (row.protocols.includes("OpenAI") && row.tags.includes("image"))
    protocols.push({
      id: "image",
      label: "OpenAI Images",
      url: `${base}/images/generations`,
      body: { model, prompt: "A blue sky", n: 1 },
    });
  return protocols.map((p) => {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (p.header === "anthropic") headers["anthropic-version"] = "2023-06-01";
    const keyHeader =
      p.header === "gemini"
        ? "x-goog-api-key"
        : p.header === "anthropic"
          ? "x-api-key"
          : "Authorization";
    const prefix = keyHeader === "Authorization" ? "Bearer " : "";
    const body = JSON.stringify(p.body, null, 2);
    const pythonHeaders = `{**${JSON.stringify(headers)}, ${JSON.stringify(keyHeader)}: ${JSON.stringify(prefix)} + os.environ["MYAPIS_API_KEY"]}`;
    return {
      id: p.id,
      label: p.label,
      curl: `curl --fail-with-body --silent --show-error ${bashQuote(p.url)} \\\n  -H "${keyHeader}: ${prefix}$MYAPIS_API_KEY" \\\n${Object.entries(
        headers,
      )
        .map(([k, v]) => `  -H ${bashQuote(`${k}: ${v}`)} \\\n`)
        .join("")}  --data-raw ${bashQuote(body)}`,
      python: `import json\nimport os\nimport urllib.request\n\nbody = json.loads(r'''${body.replaceAll("'", "\\u0027")}''')\nrequest = urllib.request.Request(\n    ${JSON.stringify(p.url)},\n    data=json.dumps(body).encode("utf-8"),\n    headers=${pythonHeaders},\n    method="POST",\n)\nwith urllib.request.urlopen(request, timeout=60) as response:\n    print(response.read().decode("utf-8"))`,
      javascript: `const response = await fetch(${JSON.stringify(p.url)}, {\n  method: "POST",\n  headers: { ...${JSON.stringify(headers)}, ${JSON.stringify(keyHeader)}: ${JSON.stringify(prefix)} + process.env.MYAPIS_API_KEY },\n  body: JSON.stringify(${body}),\n});\nif (!response.ok) throw new Error(await response.text());\nconsole.log(await response.json());`,
    };
  });
}
