import { describe, expect, it } from "vitest";
import { readGeminiGatewayResponse } from "./gemini-response";

describe("native Gemini verification response", () => {
  it("reads visible text and distinct token totals without adding the total or cached subsets", () => {
    expect(
      readGeminiGatewayResponse({
        modelVersion: "gemini-test",
        candidates: [
          {
            finishReason: "STOP",
            content: {
              parts: [
                { text: "private reasoning", thought: true },
                { text: "Hello" },
                { text: " world" },
              ],
            },
          },
        ],
        usageMetadata: {
          promptTokenCount: 20,
          toolUsePromptTokenCount: 3,
          candidatesTokenCount: 7,
          thoughtsTokenCount: 5,
          cachedContentTokenCount: 8,
          totalTokenCount: 35,
        },
      }),
    ).toEqual({
      content: "Hello world",
      inputTokens: 23,
      outputTokens: 12,
      actualModel: "gemini-test",
      status: "success",
      errorCode: "",
    });
  });
  it.each(["SAFETY", "RECITATION", "MALFORMED_FUNCTION_CALL"])(
    "does not present %s as successful or invent an actual model",
    (finishReason) => {
      expect(
        readGeminiGatewayResponse({
          candidates: [{ finishReason }],
          usageMetadata: { promptTokenCount: 3 },
        }),
      ).toMatchObject({
        status: "failed",
        errorCode: finishReason,
        actualModel: undefined,
        inputTokens: 3,
      });
    },
  );
  it("keeps blocked prompts failed and treats max tokens as normal completion", () => {
    expect(
      readGeminiGatewayResponse({ promptFeedback: { blockReason: "SAFETY" } })
        .status,
    ).toBe("failed");
    expect(
      readGeminiGatewayResponse({
        candidates: [{ finishReason: "MAX_TOKENS" }],
      }).status,
    ).toBe("success");
    expect(
      readGeminiGatewayResponse({
        promptFeedback: { blockReason: "BLOCK_REASON_UNSPECIFIED" },
        candidates: [{ finishReason: "STOP" }],
      }).status,
    ).toBe("success");
  });
  it.each([null, {}, [], { candidates: [{}] }])(
    "requires a terminal native result: %j",
    (data) =>
      expect(readGeminiGatewayResponse(data)).toMatchObject({
        status: "failed",
        errorCode: "INVALID_RESPONSE",
      }),
  );
  it("does not display malformed token counts", () => {
    expect(
      readGeminiGatewayResponse({
        usageMetadata: {
          promptTokenCount: -1,
          toolUsePromptTokenCount: Infinity,
          candidatesTokenCount: 1.5,
          thoughtsTokenCount: "5",
        },
      }),
    ).toMatchObject({ inputTokens: 0, outputTokens: 0 });
  });
});
