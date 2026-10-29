import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AccessCenter from "./access-center";
import type { PreviewRequestLike } from "../lib/api/usage";

vi.mock("../lib/api/token-choices", () => ({
  useTokenChoices: () => ({
    data: {
      revision: "a".repeat(64),
      choices: [
        {
          group: "standard",
          model: "gpt-4.1",
          protocols: ["OpenAI", "Anthropic"],
        },
      ],
    },
    isError: false,
  }),
}));

const fixture = vi.hoisted(() => ({
  smartAccess: true,
  active: true,
  rows: [] as PreviewRequestLike[],
}));
vi.mock("../lib/api/key-adapter", () => ({
  useBackendKeys: () => ({
    keys: [
      {
        id: "review",
        name: "Review key",
        tier: "standard",
        type: "client",
        model: "gpt-4.1",
        modelAccess: "all",
        keySecret: "sk-review******mask",
        smartAccess: fixture.smartAccess,
        active: fixture.active,
      },
    ],
  }),
}));
vi.mock("../lib/api/usage", () => ({
  useBackendRequests: () => ({ rows: fixture.rows }),
}));

function render(model: string) {
  return renderToStaticMarkup(
    createElement(MemoryRouter, {
      initialEntries: [`/console/connect?model=${encodeURIComponent(model)}`],
      children: createElement(AccessCenter),
    }),
  );
}

describe("connection links respect authorized smart options", () => {
  beforeEach(() => {
    fixture.smartAccess = true;
    fixture.active = true;
    fixture.rows = [];
  });
  it.each(["myapis-auto", "veyra-auto"])(
    "selects a compatible key and exports canonical configuration for %s",
    (model) => {
      const html = render(model);
      expect(html).toContain('value="review" selected=""');
      expect(html).toContain('value="myapis-auto" selected=""');
      expect(html).toContain("export OPENAI_MODEL=&#x27;myapis-auto&#x27;");
      expect(html).toContain(
        `/console/playground?key=review&amp;model=${model}&amp;protocol=OpenAI`,
      );
      expect(html).not.toContain("export OPENAI_API_KEY=&#x27;sk-review");
    },
  );
  it.each(["myapis-auto", "veyra-auto"])(
    "does not select a key without smart permission for %s",
    (model) => {
      fixture.smartAccess = false;
      expect(render(model)).not.toContain("export OPENAI_MODEL=");
    },
  );
  it("does not select an inactive key through an alias", () => {
    fixture.active = false;
    expect(render("myapis-auto")).not.toContain("export OPENAI_MODEL=");
  });
  function request(
    overrides: Partial<PreviewRequestLike> = {},
  ): PreviewRequestLike {
    return {
      trace_id: "log_7",
      time: "2026-10-28T00:00:00Z",
      model: "gpt-4.1",
      actual_model: "gpt-4.1",
      requested_model: "myapis-auto",
      protocol: "OpenAI",
      vendor: "OpenAI",
      status: "success",
      cost_nano: "123000",
      duration_ms: 100,
      key_id: "review",
      input_tokens: 1,
      output_tokens: 1,
      error_code: "",
      error_message: "",
      ...overrides,
    };
  }
  it.each(["myapis-auto", "veyra-auto"])(
    "matches the opposite historical entry for %s",
    (model) => {
      fixture.rows = [
        request({
          requested_model:
            model === "myapis-auto" ? "veyra-auto" : "myapis-auto",
        }),
      ];
      expect(render(model)).toMatch(
        /同协议成功调用|A matching API request succeeded/,
      );
      expect(render(model)).toContain("trace=log_7");
    },
  );
  it.each([
    { protocol: undefined },
    { protocol: "Anthropic" as const },
    { key_id: "other" },
    { requested_model: "myapis-auto-preview" },
  ])("does not verify mismatched or unknown metadata: %j", (overrides) => {
    fixture.rows = [request(overrides)];
    expect(render("myapis-auto")).not.toContain("trace=log_7");
  });
  it("uses the latest matching failure and does not infer that it was free", () => {
    fixture.rows = [
      request({ status: "failed", outcome: "cancelled" }),
      request({ trace_id: "log_6" }),
    ];
    const html = render("veyra-auto");
    expect(html).toContain("trace=log_7");
    expect(html).not.toMatch(/同协议成功调用|A matching API request succeeded/);
    expect(html).toMatch(/实际费用|actual charge/);
    expect(html).not.toContain("without charge");
  });
  it("does not verify a disabled key using older successful calls", () => {
    fixture.active = false;
    fixture.rows = [request()];
    expect(render("myapis-auto")).not.toContain("trace=log_7");
  });
});
