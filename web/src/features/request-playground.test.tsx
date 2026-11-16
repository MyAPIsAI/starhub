import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Playground from "./request-playground";
import { useSystemAccess } from "../lib/system-access";

const initialAccess = useSystemAccess.getState();

vi.mock("../lib/system-access", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../lib/system-access")>();
  return {
    ...actual,
    useSessionQueryIdentity: () =>
      actual.sessionQueryIdentity(actual.useSystemAccess.getState()),
  };
});

const fixture = vi.hoisted(() => ({
  smartAccess: true,
  modelAccess: "all" as "all" | "custom",
  allowedModels: undefined as string[] | undefined,
  choicesError: false,
}));
vi.mock("../lib/api/token-choices", () => ({
  useTokenChoices: () => ({
    data: {
      choices: [
        {
          group: "standard",
          model: "gpt-4.1",
          protocols: ["OpenAI", "Anthropic"],
        },
        { group: "standard", model: "glm-5.2", protocols: ["OpenAI"] },
      ],
    },
    isError: fixture.choicesError,
  }),
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
        active: true,
        keySecret: "sk-review******mask",
        ...fixture,
      },
    ],
  }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: "en" } }),
}));

function render(model?: string, protocol = "OpenAI") {
  const query = new URLSearchParams({ key: "review", protocol });
  if (model !== undefined) query.set("model", model);
  return renderToStaticMarkup(
    createElement(MemoryRouter, {
      initialEntries: [`/console/playground?${query}`],
      children: createElement(Playground, {
        onCreate: () => {},
      }),
    }),
  );
}
function modelOptions(html: string) {
  return html.match(
    /<select[^>]*id="verify-model"[^>]*>([\s\S]*?)<\/select>/,
  )![1];
}
const routingHint = "Cost routing selects an authorized, compatible candidate";

describe("playground smart entry matching", () => {
  beforeEach(() => {
    useSystemAccess
      .getState()
      .setSession({ id: 7, group: "standard", role: 1 });
    fixture.smartAccess = true;
    fixture.modelAccess = "all";
    fixture.allowedModels = undefined;
    fixture.choicesError = false;
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    useSystemAccess.setState(initialAccess, true);
  });

  it("restores the same account's draft after its session epoch changes", () => {
    const getItem = vi.fn((key: string) =>
      key === "veyra-request-draft-v4:7"
        ? JSON.stringify({ model: "glm-5.2", prompt: "Latest account draft" })
        : null,
    );
    vi.stubGlobal("sessionStorage", { getItem });
    expect(render()).toContain("Latest account draft");
    useSystemAccess
      .getState()
      .setSession({ id: 7, group: "standard", role: 1 });
    expect(render()).toContain("Latest account draft");
    expect(getItem.mock.calls).toEqual([
      ["veyra-request-draft-v4:7"],
      ["veyra-request-draft-v4:7"],
    ]);
  });

  it("does not restore a different account's draft", () => {
    vi.stubGlobal("sessionStorage", {
      getItem: (key: string) =>
        key === "veyra-request-draft-v4:7"
          ? JSON.stringify({ prompt: "Private account seven draft" })
          : null,
    });
    useSystemAccess
      .getState()
      .setSession({ id: 8, group: "standard", role: 1 });
    expect(render()).not.toContain("Private account seven draft");
  });

  it("does not read drafts without a confirmed account", () => {
    const getItem = vi.fn();
    vi.stubGlobal("sessionStorage", { getItem });
    useSystemAccess.getState().setSession(null);
    render();
    expect(getItem).not.toHaveBeenCalled();
  });

  it("ignores legacy drafts instead of guessing which epoch was latest", () => {
    const getItem = vi.fn((key: string) =>
      key.startsWith("veyra-request-draft-v3:")
        ? JSON.stringify({ prompt: "Unordered legacy draft" })
        : null,
    );
    vi.stubGlobal("sessionStorage", { getItem });
    expect(render()).not.toContain("Unordered legacy draft");
    expect(getItem).toHaveBeenCalledExactlyOnceWith("veyra-request-draft-v4:7");
  });

  describe.each(["OpenAI", "Anthropic"])("%s links", (protocol) => {
    it.each(["myapis-auto", "veyra-auto"])(
      "selects the authorized smart option while preserving %s in the request",
      (model) => {
        const html = render(model, protocol);
        expect(modelOptions(html)).toContain('value="myapis-auto" selected=""');
        expect(modelOptions(html)).not.toContain("Unavailable");
        expect(html).toContain(routingHint);
        expect(html).toContain(`&quot;model&quot;: &quot;${model}&quot;`);
        expect(html).toContain(
          `/console/connect?key=review&amp;model=${model}`,
        );
      },
    );
  });
  it.each(["myapis-auto", "veyra-auto"])(
    "matches the saved %s draft without overwriting its model",
    (model) => {
      const draft = JSON.stringify({
        keyId: "review",
        model,
        protocol: "OpenAI",
      });
      const setItem = vi.fn();
      vi.stubGlobal("sessionStorage", { getItem: () => draft, setItem });
      const html = render();
      expect(modelOptions(html)).toContain('value="myapis-auto" selected=""');
      expect(modelOptions(html)).not.toContain("Unavailable");
      expect(html).toContain(`&quot;model&quot;: &quot;${model}&quot;`);
      expect(setItem).not.toHaveBeenCalled();
    },
  );
  describe.each(["myapis-auto", "veyra-auto"])("%s authorization", (model) => {
    function expectUnavailable(protocol = "OpenAI") {
      const html = render(model, protocol);
      expect(modelOptions(html)).toContain(`${model} · Unavailable`);
      expect(modelOptions(html)).not.toContain("Smart routing");
      expect(html).not.toContain(routingHint);
      expect(html).toContain(`&quot;model&quot;: &quot;${model}&quot;`);
    }
    it("does not grant smart permission from the URL", () => {
      fixture.smartAccess = false;
      expectUnavailable();
    });
    it("does not expand a virtual-only allowlist", () => {
      fixture.modelAccess = "custom";
      fixture.allowedModels = [model];
      expectUnavailable();
    });
    it("does not match smart routing without protocol-compatible candidates", () => {
      fixture.modelAccess = "custom";
      fixture.allowedModels = ["deepseek-chat"];
      expectUnavailable("Anthropic");
    });
  });
  it("preserves explicit models without routing hints", () => {
    const html = render("gpt-4.1");
    expect(modelOptions(html)).toContain('value="gpt-4.1" selected=""');
    expect(html).not.toContain(routingHint);
    expect(html).toContain("&quot;model&quot;: &quot;gpt-4.1&quot;");
  });
  it("lists backend models that are absent from the demo catalog", () => {
    const html = render("glm-5.2");
    expect(modelOptions(html)).toContain('value="glm-5.2" selected=""');
    expect(modelOptions(html)).not.toContain("Unavailable");
  });
  it("requires unlocking a masked key instead of simulating a request", () => {
    const html = render("glm-5.2");
    expect(html).toContain('id="verify-password"');
    expect(html).toContain("Unlock the key before sending");
    expect(html).not.toContain("SIMULATION");
    expect(html).not.toContain("will simulate");
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>[\s\S]*?Send request/);
  });
  it("does not use demo model choices when the backend catalog is unavailable", () => {
    fixture.choicesError = true;
    const html = render("glm-5.2");
    expect(modelOptions(html)).toContain("Unavailable");
    expect(modelOptions(html)).not.toContain('value="gpt-4.1"');
    expect(html).toContain("Available models could not be loaded");
  });
  it.each(["myapis-auto-preview", "veyra-auto-preview", "missing-model"])(
    "does not substitute %s with a smart model",
    (model) => {
      const html = render(model);
      expect(modelOptions(html)).toContain(`${model} · Unavailable`);
      expect(modelOptions(html)).toContain(`value="${model}" selected=""`);
      expect(html).not.toContain(routingHint);
    },
  );
});
