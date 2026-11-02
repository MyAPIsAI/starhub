import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { NativeKeyForm } from "./native-key-form";
import { tokenToPreviewKey } from "../lib/api/key-adapter";

vi.mock("../lib/api/token-choices", () => ({
  useTokenChoices: () => ({
    data: {
      revision: "a".repeat(64),
      choices: [
        { group: "default", model: "current-model", protocols: ["OpenAI"] },
      ],
    },
    isError: false,
  }),
}));
vi.mock("../lib/api/key-adapter", async (original) => ({
  ...(await original<typeof import("../lib/api/key-adapter")>()),
  useBackendKeys: () => ({ keys: [] }),
}));
vi.mock("../lib/api/tokens", () => ({
  useCreateToken: () => ({ isPending: false }),
  useUpdateToken: () => ({ isPending: false }),
}));
vi.mock("../components/ui/overlay", () => ({
  Drawer: ({ children }: { children: ReactNode }) =>
    createElement("div", null, children),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ i18n: { language: "en" } }),
}));

describe("editing existing native keys", () => {
  it("offers smart candidates only within the existing authorization", () => {
    const item = tokenToPreviewKey({
      id: 1,
      user_id: 1,
      name: "Restricted",
      key: "masked",
      status: 1,
      group: "default",
      auto_groups: "",
      expired_time: -1,
      model_limits: "allowed-model",
      smart_access: true,
      smart_candidates: "allowed-model",
      allow_ips: "",
      unlimited_quota: true,
      remain_quota: 0,
      used_quota: 0,
      created_at: 0,
    });
    const html = renderToStaticMarkup(
      createElement(MemoryRouter, {
        children: createElement(NativeKeyForm, { item, onClose: () => {} }),
      }),
    );
    expect(html).toContain("Smart candidate models");
    expect(html).toContain("Unavailable");
    const candidates = html
      .split("<legend>Smart candidate models</legend>")[1]
      .split("</fieldset>")[0];
    expect(candidates).not.toContain("current-model");
    expect(candidates).toContain(
      "No authorized text candidates are available.",
    );
  });
  it.each(["legacy-model", ""])(
    "preserves automatic routing with allowlist %s",
    (limits) => {
      const item = tokenToPreviewKey({
        id: 1,
        user_id: 1,
        name: "Automatic",
        key: "masked",
        status: 1,
        group: "auto",
        auto_groups: "default,legacy-group",
        expired_time: -1,
        model_limits: limits,
        allow_ips: "",
        unlimited_quota: false,
        remain_quota: 30_000_000,
        used_quota: 20_000_000,
        created_at: 0,
      });
      const html = renderToStaticMarkup(
        createElement(MemoryRouter, {
          children: createElement(NativeKeyForm, { item, onClose: () => {} }),
        }),
      );
      expect(html).toContain(
        "Existing automatic groups and model permissions are preserved.",
      );
      expect(html).toContain('value="auto" selected=""');
      expect(html).toContain('id="new-key-quota"');
      expect(html).toContain('value="100"');
      if (limits) expect(html).toContain('disabled="" checked=""');
      expect(html).not.toContain(
        'type="checkbox" checked=""/><span>current-model',
      );
    },
  );
});
