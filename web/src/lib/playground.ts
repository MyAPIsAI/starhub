import type { PreviewKey } from "./product-preview";
import type { TokenChoices } from "./api/token-choices";
import type { DemoRequest, DemoProtocol } from "./request-contract";
import { buildCurlRequest } from "./request-contract";
import { buildGatewayRequest } from "./gateway-request";
import { SMART_MODEL_ID, isSmartModelId } from "./smart-model";

export function playgroundModels(
  key: PreviewKey | undefined,
  choices: TokenChoices | undefined,
  protocol: DemoProtocol,
) {
  if (!key || !choices) return [];
  const allowed = choices.choices.filter(
    (c) =>
      c.group === key.tier &&
      c.smart_eligible === true &&
      c.protocols.includes(protocol) &&
      (key.modelAccess === "all" || key.allowedModels?.includes(c.model)),
  );
  const result = allowed.map((c) => ({
    id: c.model,
    kind: "specified" as "specified" | "smart",
  }));
  if (
    key.smartAccess &&
    allowed.some(
      (c) => !key.smartCandidates || key.smartCandidates.includes(c.model),
    )
  )
    result.unshift({ id: SMART_MODEL_ID, kind: "smart" });
  return result;
}

export function selectedPlaygroundModel(
  options: { id: string; kind: string }[],
  model: string,
) {
  return options.find(
    (option) =>
      option.id === model || (option.kind === "smart" && isSmartModelId(model)),
  );
}

export interface PlaygroundResult {
  trace_id: string;
  key_id: number;
  requested_model: string;
  actual_model: string;
  vendor: string;
  vendor_display: string;
  fallback: boolean;
  duration_ms: number;
  ttft_ms: number;
  input_tokens: number;
  output_tokens: number;
  cost_nano: string;
  status: number;
  source: "playground";
}

function responseText(data: any, protocol: DemoProtocol): string {
  if (protocol === "Gemini")
    return (data.candidates?.[0]?.content?.parts ?? [])
      .map((p: any) => p.text ?? "")
      .join("");
  if (protocol === "Anthropic")
    return (
      data.delta?.text ??
      (data.content ?? []).map((p: any) => p.text ?? "").join("")
    );
  return (
    data.choices?.[0]?.delta?.content ??
    data.choices?.[0]?.message?.content ??
    ""
  );
}

// Consume arbitrarily split UTF-8/SSE frames; completion must be explicit.
export async function readPlaygroundResponse(
  response: Response,
  protocol: DemoProtocol,
  update: (text: string) => void,
) {
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(
      data?.error?.message ?? data?.message ?? `HTTP ${response.status}`,
    );
  }
  if (!response.headers.get("content-type")?.startsWith("text/event-stream")) {
    const data = await response.json();
    if (data.error) throw new Error(data.error.message ?? "Request failed");
    const text = responseText(data, protocol);
    update(text);
    return text;
  }
  if (!response.body) throw new Error("Response stream unavailable");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = "",
    text = "",
    terminal = false,
    size = 0;
  const frame = (value: string) => {
    const data = value
      .split("\n")
      .filter((line) => line.startsWith("data:"))
      .map((line) => line.slice(5).trimStart())
      .join("\n");
    if (!data) return;
    if (data === "[DONE]") {
      terminal = true;
      return;
    }
    const parsed = JSON.parse(data);
    if (parsed.error || parsed.type === "error")
      throw new Error(parsed.error?.message ?? "Stream failed");
    const delta = responseText(parsed, protocol);
    if (delta) {
      text += delta;
      update(text);
    }
    terminal ||=
      protocol === "Anthropic"
        ? parsed.type === "message_stop"
        : protocol === "Gemini"
          ? ["STOP", "MAX_TOKENS"].includes(
              parsed.candidates?.[0]?.finishReason,
            )
          : !!parsed.choices?.[0]?.finish_reason;
  };
  try {
    while (true) {
      const { value, done } = await reader.read();
      if (value) {
        size += value.byteLength;
        if (size > 8 << 20)
          throw new Error("Response exceeds the playground limit");
      }
      pending += decoder.decode(value, { stream: !done });
      pending = pending.replace(/\r\n/g, "\n");
      let boundary;
      while ((boundary = pending.indexOf("\n\n")) >= 0) {
        frame(pending.slice(0, boundary));
        pending = pending.slice(boundary + 2);
      }
      if (done) {
        if (pending.trim()) frame(pending);
        break;
      }
    }
    if (!terminal)
      throw new Error("Stream interrupted; check request logs before retrying");
    return text;
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

export function playgroundExamples(request: DemoRequest, origin: string) {
  const call = buildGatewayRequest(request, "sk-example");
  const headers = { ...call.init.headers };
  const auth =
    request.protocol === "OpenAI"
      ? "Authorization"
      : request.protocol === "Anthropic"
        ? "x-api-key"
        : "x-goog-api-key";
  delete headers[auth];
  const prefix = request.protocol === "OpenAI" ? "Bearer " : "";
  const url = origin + call.endpoint;
  return {
    curl: buildCurlRequest(request, `${origin}/v1`),
    python: `import json, os\nfrom urllib.request import Request, urlopen\n\nheaders = json.loads(${JSON.stringify(JSON.stringify(headers))})\nheaders[${JSON.stringify(auth)}] = ${JSON.stringify(prefix)} + os.environ["MYAPIS_API_KEY"]\npayload = ${JSON.stringify(call.init.body)}\nrequest = Request(${JSON.stringify(url)}, data=payload.encode("utf-8"), headers=headers, method="POST")\nwith urlopen(request) as response:\n    for line in response:\n        print(line.decode("utf-8"), end="", flush=True)`,
    node: `const response = await fetch(${JSON.stringify(url)}, {\n  method: "POST",\n  headers: { ...${JSON.stringify(headers, null, 2)}, ${JSON.stringify(auth)}: ${JSON.stringify(prefix)} + process.env.MYAPIS_API_KEY },\n  body: ${JSON.stringify(call.init.body)}\n});\nif (!response.ok) throw new Error(await response.text());\nfor await (const chunk of response.body) process.stdout.write(chunk);`,
  };
}
