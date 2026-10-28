import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import CreateKey from "./create-key";

const fixture = vi.hoisted(() => ({ smartAccess: true, active: true }));
vi.mock("../lib/api/key-adapter", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api/key-adapter")>()),
  useBackendKeys: () => ({
    keys: [
      {
        id: "review",
        name: "Review key",
        model: "gpt-4.1",
        tier: "pro",
        type: "client",
        modelAccess: "all",
        ...fixture,
      },
    ],
  }),
}));
vi.mock("../lib/api/tokens", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api/tokens")>()),
  useCreateToken: () => ({ isPending: false }),
}));
vi.mock("../components/ui/overlay", () => ({
  Modal: ({ children }: { children: ReactNode }) =>
    createElement("div", null, children),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: "en" } }),
}));
function render(initialModel: string) {
  return renderToStaticMarkup(
    createElement(MemoryRouter, {
      children: createElement(CreateKey, {
        open: true,
        setOpen: () => {},
        initialModel,
      }),
    }),
  );
}
describe("key creation reuses authorized model options", () => {
  beforeEach(() => {
    fixture.smartAccess = true;
    fixture.active = true;
  });
  it.each(["myapis-auto", "veyra-auto"])(
    "reuses a smart key for %s without rewriting the example",
    (model) => {
      const html = render(model);
      expect(html).toContain('class="key-reuse"');
      expect(html).toContain("Reuse existing key");
      expect(html).toContain(`<code>${model}</code>`);
    },
  );
  it.each(["myapis-auto", "veyra-auto"])(
    "does not suggest a key without smart permission for %s",
    (model) => {
      fixture.smartAccess = false;
      expect(render(model)).not.toContain('class="key-reuse"');
    },
  );
  it("does not suggest a disabled key for an old smart link", () => {
    fixture.active = false;
    expect(render("veyra-auto")).not.toContain('class="key-reuse"');
  });
  it("keeps explicit-model reuse", () => {
    expect(render("gpt-4.1")).toContain('class="key-reuse"');
  });
  it("does not match an unknown model by prefix", () => {
    expect(render("veyra-auto-preview")).not.toContain('class="key-reuse"');
  });
});
