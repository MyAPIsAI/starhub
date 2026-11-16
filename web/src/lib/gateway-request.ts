import type { DemoRequest } from "./request-contract";
import { requestPayload, geminiRequestPayload } from "./request-contract";
import { validDemoSecret } from "./client-config";

/** Build only protocols actually registered by the gateway. */
export function buildGatewayRequest(request: DemoRequest, secret: string) {
  if (!validDemoSecret(secret))
    throw new Error("A full credential is required");
  const call = buildRelayRequest(request);
  call.init.headers[
    request.protocol === "Anthropic"
      ? "x-api-key"
      : request.protocol === "Gemini"
        ? "x-goog-api-key"
        : "Authorization"
  ] = request.protocol === "OpenAI" ? `Bearer ${secret}` : secret;
  return call;
}

/** Build the protocol payload without exposing or requiring a credential. */
export function buildRelayRequest(request: DemoRequest) {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  let body: unknown = requestPayload(request);
  if (request.protocol === "Anthropic") {
    headers["anthropic-version"] = "2023-06-01";
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
  } else if (request.protocol === "Gemini") {
    body = geminiRequestPayload(request);
  }
  if (request.capability) headers["X-MyAPIs-Capability"] = request.capability;
  if (request.chainId) headers["X-MyAPIs-Chain-Id"] = request.chainId;
  return {
    endpoint:
      request.protocol === "Anthropic"
        ? "/v1/messages"
        : request.protocol === "Gemini"
          ? `/v1beta/models/${encodeURIComponent(request.model)}:generateContent`
          : "/v1/chat/completions",
    init: { method: "POST", headers, body: JSON.stringify(body) },
  };
}
