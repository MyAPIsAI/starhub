import type { DemoRequest } from "./request-contract";
import { requestPayload } from "./request-contract";
import { validDemoSecret } from "./client-config";

/** Build only protocols actually registered by the gateway. */
export function buildGatewayRequest(request: DemoRequest, secret: string) {
  if (!validDemoSecret(secret))
    throw new Error("A full credential is required");
  if (request.protocol === "Gemini")
    throw new Error("Gemini native requests are not supported by this gateway");
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  let body: unknown = requestPayload(request);
  if (request.protocol === "Anthropic") {
    headers["x-api-key"] = secret;
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
  } else headers.Authorization = `Bearer ${secret}`;
  if (request.capability) headers["X-MyAPIs-Capability"] = request.capability;
  if (request.chainId) headers["X-MyAPIs-Chain-Id"] = request.chainId;
  return {
    endpoint:
      request.protocol === "Anthropic"
        ? "/v1/messages"
        : "/v1/chat/completions",
    init: { method: "POST", headers, body: JSON.stringify(body) },
  };
}
