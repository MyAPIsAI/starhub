import type { ModelItem } from "../config/models";

export type DemoProtocol = ModelItem["protocols"][number];
export type DemoCapability = ModelItem["capabilities"][number];
export interface DemoRequest {
  model: string;
  messages: { role: "system" | "user" | "assistant"; content: string }[];
  temperature: number;
  max_tokens: number;
  protocol: DemoProtocol;
  capability?: DemoCapability;
  chainId?: string;
}

export function defaultDemoRequest(model = "gpt-4.1"): DemoRequest {
  return {
    model,
    messages: [{ role: "user", content: "请简要说明如何开始使用 MyAPIs。" }],
    temperature: 0.7,
    max_tokens: 512,
    protocol: "OpenAI",
    capability: "text",
  };
}

export function validateDemoRequest(request: DemoRequest): boolean {
  return Boolean(
    request &&
    typeof request.model === "string" &&
    request.model.trim() &&
    ["OpenAI", "Anthropic", "Gemini"].includes(request.protocol) &&
    (!request.capability ||
      ["text", "code", "vision", "reasoning"].includes(request.capability)) &&
    Number.isFinite(request.temperature) &&
    request.temperature >= 0 &&
    request.temperature <= 2 &&
    Number.isSafeInteger(request.max_tokens) &&
    request.max_tokens > 0 &&
    request.max_tokens <= 32768 &&
    Array.isArray(request.messages) &&
    request.messages.length > 0 &&
    request.messages.every(
      (message) =>
        ["system", "user", "assistant"].includes(message.role) &&
        typeof message.content === "string",
    ) &&
    request.messages.some((message) => message.content.trim()) &&
    (request.chainId === undefined ||
      (typeof request.chainId === "string" &&
        /^[A-Za-z0-9._:-]{1,100}$/.test(request.chainId))),
  );
}

/** A deterministic character-based estimate, explicitly not vendor tokenization. */
export function estimateDemoTokens(text: string): number {
  return Math.max(
    1,
    Math.ceil(
      Array.from(text).reduce(
        (total, char) => total + (/[^\x00-\x7F]/.test(char) ? 1 : 0.25),
        0,
      ),
    ),
  );
}
export function demoInputTokens(request: DemoRequest): number {
  return request.messages.reduce(
    (total, message) => total + estimateDemoTokens(message.content) + 4,
    3,
  );
}

/** The response describes the submitted task without claiming a real model result. */
export function simulateDemoResponse(
  request: DemoRequest,
  actualModel: string,
) {
  const prompt =
    [...request.messages].reverse().find((message) => message.role === "user")
      ?.content ??
    request.messages.at(-1)?.content ??
    "";
  const task =
    request.capability === "code"
      ? "代码任务"
      : request.capability === "vision"
        ? "视觉任务"
        : /总结|摘要|summari/i.test(prompt)
          ? "摘要任务"
          : /翻译|translate/i.test(prompt)
            ? "翻译任务"
            : "文本任务";
  const message = `【确定性模拟响应 · 非真实模型效果】\n已收到${task}：${prompt}\n请求模型：${request.model}；实际模型：${actualModel}。\n这是本地接入与计费验证；内容按输入回显，未调用任何付费模型。`;
  let output = "",
    weightedTokens = 0;
  for (const char of message) {
    const weight = /[^\x00-\x7F]/.test(char) ? 1 : 0.25;
    if (Math.ceil(weightedTokens + weight) > request.max_tokens) break;
    weightedTokens += weight;
    output += char;
  }
  return {
    text: output,
    inputTokens: demoInputTokens(request),
    outputTokens: estimateDemoTokens(output),
  };
}

/** Export exactly the request fields the simulator reads; JSON handles quotes/newlines. */
export function requestPayload(request: DemoRequest) {
  return {
    model: request.model,
    messages: request.messages,
    temperature: request.temperature,
    max_tokens: request.max_tokens,
  };
}
export function serializeRequest(request: DemoRequest): string {
  return JSON.stringify(requestPayload(request), null, 2);
}
const shellQuote = (value: string) => `'${value.replace(/'/g, `'"'"'`)}'`;
export function buildCurlRequest(
  request: DemoRequest,
  baseUrl = "https://api.myapis.example/v1",
  key = "$MYAPIS_API_KEY",
) {
  const protocol = request.protocol;
  const endpoint =
    protocol === "Anthropic"
      ? `${baseUrl.replace(/\/v1\/?$/, "")}/v1/messages`
      : protocol === "Gemini"
        ? `${baseUrl.replace(/\/v1\/?$/, "")}/v1beta/models/${encodeURIComponent(request.model)}:generateContent`
        : `${baseUrl.replace(/\/$/, "")}/chat/completions`;
  let body: unknown = requestPayload(request);
  if (protocol === "Anthropic")
    body = {
      ...requestPayload(request),
      messages: request.messages.filter((message) => message.role !== "system"),
      ...(request.messages.some((message) => message.role === "system")
        ? {
            system: request.messages
              .filter((message) => message.role === "system")
              .map((message) => message.content)
              .join("\n"),
          }
        : {}),
    };
  if (protocol === "Gemini")
    body = {
      contents: request.messages
        .filter((message) => message.role !== "system")
        .map((message) => ({
          role: message.role === "assistant" ? "model" : "user",
          parts: [{ text: message.content }],
        })),
      generationConfig: {
        temperature: request.temperature,
        maxOutputTokens: request.max_tokens,
      },
      ...(request.messages.some((message) => message.role === "system")
        ? {
            systemInstruction: {
              parts: request.messages
                .filter((message) => message.role === "system")
                .map((message) => ({ text: message.content })),
            },
          }
        : {}),
    };
  const safeKey = /[•●*]/.test(key) ? "$MYAPIS_API_KEY" : key;
  const header =
    protocol === "Anthropic"
      ? `x-api-key: ${safeKey}`
      : protocol === "Gemini"
        ? `x-goog-api-key: ${safeKey}`
        : `Authorization: Bearer ${safeKey}`;
  const authorization =
    safeKey === "$MYAPIS_API_KEY" ? `"${header}"` : shellQuote(header);
  const routingHeaders = [
    request.capability
      ? `X-MyAPIs-Capability: ${request.capability}`
      : undefined,
    request.chainId ? `X-MyAPIs-Chain-Id: ${request.chainId}` : undefined,
  ]
    .filter((header): header is string => Boolean(header))
    .map((header) => " \\\n  -H " + shellQuote(header))
    .join("");
  return `# MyAPIs gateway request; set the full credential before running.
# X-MyAPIs headers are gateway routing metadata.\ncurl ${shellQuote(endpoint)} \\\n  -H ${authorization} \\\n  -H 'Content-Type: application/json'${protocol === "Anthropic" ? " \\\n  -H 'anthropic-version: 2023-06-01'" : ""} \\\n  --data-raw ${shellQuote(JSON.stringify(body, null, 2))}${routingHeaders}`;
}
