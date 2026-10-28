import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AccessCenter from "./access-center";

const fixture = vi.hoisted(() => ({ smartAccess: true, active: true }));
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
        ...fixture,
      },
    ],
  }),
}));
vi.mock("../lib/api/usage", () => ({
  useBackendRequests: () => ({ rows: [] }),
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
  });
  it.each(["myapis-auto", "veyra-auto"])(
    "selects a compatible key and exports canonical configuration for %s",
    (model) => {
      const html = render(model);
      expect(html).toContain('value="review" selected=""');
      expect(html).toContain('value="veyra-auto" selected=""');
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
});
