const object = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
const text = (value: unknown) => (typeof value === "string" ? value : "");
const count = (value: unknown) =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : 0;
const sum = (a: unknown, b: unknown) => {
  const total = count(a) + count(b);
  return Number.isSafeInteger(total) ? total : 0;
};

// UI summaries never determine settlement; backend logs remain authoritative.
export function readGeminiGatewayResponse(data: unknown) {
  const response = object(data);
  const candidate = object(
    Array.isArray(response.candidates) ? response.candidates[0] : undefined,
  );
  const content = object(candidate.content);
  const parts = Array.isArray(content.parts) ? content.parts : [];
  const usage = object(response.usageMetadata);
  const blockReason = text(object(response.promptFeedback).blockReason);
  const stopReason =
    text(object(response.error).status) ||
    (blockReason === "BLOCK_REASON_UNSPECIFIED" ? "" : blockReason) ||
    text(candidate.finishReason);
  const success = stopReason === "STOP" || stopReason === "MAX_TOKENS";
  return {
    content: parts
      .filter((part) => object(part).thought !== true)
      .map((part) => text(object(part).text))
      .join(""),
    inputTokens: sum(usage.promptTokenCount, usage.toolUsePromptTokenCount),
    outputTokens: sum(usage.candidatesTokenCount, usage.thoughtsTokenCount),
    actualModel: text(response.modelVersion) || undefined,
    status: success ? ("success" as const) : ("failed" as const),
    errorCode: success ? "" : stopReason || "INVALID_RESPONSE",
  };
}
