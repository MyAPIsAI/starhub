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
  Modal: ({ children }: { children: ReactNode }) =>
    createElement("div", null, children),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ i18n: { language: "en" } }),
}));

describe("editing existing native keys", () => {
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
      if (limits)
        expect(html).toContain('type="checkbox" disabled="" checked=""');
      expect(html).not.toContain(
        'type="checkbox" checked=""/><span>current-model',
      );
    },
  );
});
