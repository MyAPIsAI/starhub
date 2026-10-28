import { describe, expect, it } from "vitest";
import { buildGatewayRequest } from "./gateway-request";
import {
  defaultDemoRequest,
  geminiRequestPayload,
  serializeRequest,
} from "./request-contract";

describe("real gateway protocol requests", () => {
  it.each(["sk-review******mask", "sk-●●●", "sk-••••", ""])(
    "does not put a masked or missing key into a real request: %s",
    (secret) => {
      expect(() => buildGatewayRequest(defaultDemoRequest(), secret)).toThrow(
        "credential",
      );
    },
  );
  it.each(["myapis-auto", "veyra-auto"])(
    "preserves the %s entry in both supported protocols",
    (model) => {
      for (const protocol of ["OpenAI", "Anthropic"] as const) {
        const request = {
          ...defaultDemoRequest(model),
          protocol,
          chainId: "review-chain",
          messages: [
            { role: "system" as const, content: "Review system" },
            { role: "user" as const, content: "Review prompt" },
          ],
        };
        const original = structuredClone(request);
        const call = buildGatewayRequest(request, "sk-review-valid");
        expect(call.endpoint).toBe(
          protocol === "Anthropic" ? "/v1/messages" : "/v1/chat/completions",
        );
        const body = JSON.parse(call.init.body);
        expect(body.model).toBe(model);
        expect(call.init.headers["X-MyAPIs-Capability"]).toBe("text");
        expect(call.init.headers["X-MyAPIs-Chain-Id"]).toBe("review-chain");
        if (protocol === "Anthropic") {
          expect(body.system).toBe("Review system");
          expect(body.messages).toEqual([
            { role: "user", content: "Review prompt" },
          ]);
          expect(call.init.headers["x-api-key"]).toBe("sk-review-valid");
        } else
          expect(call.init.headers.Authorization).toBe(
            "Bearer sk-review-valid",
          );
        expect(request).toEqual(original);
      }
    },
  );
  it.each(["gemini-2.5-pro", "myapis-auto", "veyra-auto"])(
    "sends native Gemini %s with the same body as JSON export",
    (model) => {
      const request = {
        ...defaultDemoRequest(model),
        protocol: "Gemini" as const,
        messages: [
          { role: "system" as const, content: "System" },
          { role: "user" as const, content: "Hello" },
          { role: "assistant" as const, content: "Hi" },
        ],
      };
      const call = buildGatewayRequest(request, "sk-review-valid");
      expect(call.endpoint).toBe(`/v1beta/models/${model}:generateContent`);
      expect(call.init.headers["x-goog-api-key"]).toBe("sk-review-valid");
      expect(call.init.headers.Authorization).toBeUndefined();
      const body = JSON.parse(call.init.body);
      expect(body).toEqual(JSON.parse(serializeRequest(request)));
      expect(body).toEqual(geminiRequestPayload(request));
      expect(body.contents).toEqual([
        { role: "user", parts: [{ text: "Hello" }] },
        { role: "model", parts: [{ text: "Hi" }] },
      ]);
      expect(body.systemInstruction).toEqual({ parts: [{ text: "System" }] });
      expect(body.model).toBeUndefined();
      expect(body.generationConfig.maxOutputTokens).toBe(request.max_tokens);
    },
  );
});
