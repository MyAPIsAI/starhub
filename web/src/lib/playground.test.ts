import { describe, expect, it } from "vitest";
import {
  playgroundExamples,
  playgroundModels,
  readPlaygroundResponse,
} from "./playground";
import type { PreviewKey } from "./product-preview";
import type { TokenChoices } from "./api/token-choices";
import { defaultDemoRequest } from "./request-contract";

function response(text: string, bytes = 1) {
  const data = new TextEncoder().encode(text);
  return new Response(
    new ReadableStream({
      start(controller) {
        for (let i = 0; i < data.length; i += bytes)
          controller.enqueue(data.slice(i, i + bytes));
        controller.close();
      },
    }),
    { headers: { "Content-Type": "text/event-stream" } },
  );
}

describe("live playground streams", () => {
  it("renders split UTF-8 frames before completion", async () => {
    const updates: string[] = [];
    const text =
      'data: {"choices":[{"delta":{"content":"你好"}}]}\r\n\r\ndata: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}\r\n\r\ndata: [DONE]\r\n\r\n';
    expect(
      await readPlaygroundResponse(response(text), "OpenAI", (value) =>
        updates.push(value),
      ),
    ).toBe("你好 world");
    expect(updates).toEqual(["你好", "你好 world"]);
  });
  it.each(["Anthropic", "Gemini"] as const)(
    "reads %s events and terminal markers",
    async (protocol) => {
      const text =
        protocol === "Anthropic"
          ? 'event: content_block_delta\ndata: {"type":"content_block_delta","delta":{"text":"hello"}}\n\nevent: message_stop\ndata: {"type":"message_stop"}\n\n'
          : 'data: {"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}]}\n\n';
      expect(
        await readPlaygroundResponse(response(text, 7), protocol, () => {}),
      ).toBe("hello");
    },
  );
  it("rejects incomplete streams while retaining partial output", async () => {
    let partial = "";
    await expect(
      readPlaygroundResponse(
        response('data: {"choices":[{"delta":{"content":"partial"}}]}\n\n'),
        "OpenAI",
        (text) => {
          partial = text;
        },
      ),
    ).rejects.toThrow("Stream interrupted");
    expect(partial).toBe("partial");
  });
  it("does not turn provider errors into success", async () => {
    await expect(
      readPlaygroundResponse(
        response('data: {"error":{"message":"synthetic rejection"}}\n\n'),
        "OpenAI",
        () => {},
      ),
    ).rejects.toThrow("synthetic rejection");
  });
  it("handles ordinary JSON without waiting for SSE", async () => {
    const json = new Response(
      JSON.stringify({ choices: [{ message: { content: "hello" } }] }),
      { headers: { "Content-Type": "application/json" } },
    );
    expect(await readPlaygroundResponse(json, "OpenAI", () => {})).toBe(
      "hello",
    );
  });
});

describe("playground catalog and request previews", () => {
  const key = {
    tier: "group-a",
    modelAccess: "custom",
    allowedModels: ["text-a", "text-b", "image-a"],
    smartAccess: true,
    smartCandidates: ["text-a"],
  } as PreviewKey;
  const choices = {
    choices: [
      {
        group: "group-a",
        model: "text-a",
        protocols: ["OpenAI"],
        smart_eligible: true,
      },
      {
        group: "group-a",
        model: "text-b",
        protocols: ["Anthropic"],
        smart_eligible: true,
      },
      {
        group: "group-a",
        model: "image-a",
        protocols: ["OpenAI"],
        smart_eligible: false,
      },
      {
        group: "group-a",
        model: "not-authorized",
        protocols: ["OpenAI"],
        smart_eligible: true,
      },
      {
        group: "other",
        model: "foreign",
        protocols: ["OpenAI"],
        smart_eligible: true,
      },
    ],
  } as TokenChoices;
  it("puts the smart entry first within the authorized text models", () => {
    expect(playgroundModels(key, choices, "OpenAI").map((c) => c.id)).toEqual([
      "myapis-auto",
      "text-a",
    ]);
    expect(
      playgroundModels(key, choices, "Anthropic").map((c) => c.id),
    ).toEqual(["text-b"]);
    expect(playgroundModels(key, undefined, "OpenAI")).toEqual([]);
  });
  it.each(["OpenAI", "Anthropic", "Gemini"] as const)(
    "exports %s code without embedding credentials",
    (protocol) => {
      const request = {
        ...defaultDemoRequest("text-a"),
        protocol,
        stream: true,
      };
      const examples = playgroundExamples(request, "https://gateway.example");
      for (const code of Object.values(examples)) {
        expect(code).toContain("MYAPIS_API_KEY");
        expect(code).toContain("gateway.example");
        expect(code).not.toContain("sk-example");
        expect(code).not.toContain("X-MyAPIs-Key-Grant");
      }
      expect(examples.curl).toContain(
        protocol === "Gemini"
          ? "streamGenerateContent?alt=sse"
          : '"stream": true',
      );
    },
  );
});
