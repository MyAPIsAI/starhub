import { describe, it, expect } from "vitest";
import { execFileSync } from "node:child_process";
import { buildClientConfiguration, getClientModels } from "./client-config";
import { createDefaultPricing } from "./model-pricing";
import { getDirectoryModel, directoryPrices } from "../config/model-directory";
import type { KeyModelPolicy } from "./key-model-policy";

const isUnix = process.platform !== "win32";
const itUnix = isUnix ? it : it.skip;

const key: KeyModelPolicy = {
  tier: "standard",
  type: "client",
  model: "gpt-4.1",
  modelAccess: "all",
  smartAccess: true,
};
describe("client configuration respects model and credential boundaries", () => {
  it("only presents Anthropic candidates to Claude Code and never treats chat as Responses", () => {
    expect(getClientModels(key, "claude-code").map((item) => item.id)).toEqual([
      "claude-sonnet-4-20250514",
      "veyra-auto",
    ]);
    expect(getClientModels(key, "codex")).toEqual([]);
    expect(
      buildClientConfiguration({ target: "codex", model: "gpt-4.1" }),
    ).toBeNull();
  });
  it("a limited key cannot gain cross-protocol candidates via smart routing", () => {
    const limited = {
      ...key,
      modelAccess: "custom" as const,
      allowedModels: ["deepseek-chat"],
    };
    expect(getClientModels(limited, "claude-code")).toEqual([]);
    expect(getClientModels(limited, "openai").map((item) => item.id)).toEqual([
      "deepseek-chat",
      "veyra-auto",
    ]);
  });
  itUnix("rejects a mask and uses an environment variable while locked", () => {
    expect(() =>
      buildClientConfiguration({
        target: "openai",
        model: "gpt-4.1",
        secret: "sk-demo-••••1234",
      }),
    ).toThrow("masked");
    const config = buildClientConfiguration({
      target: "openai",
      model: "gpt-4.1",
    })!;
    expect(config.credential).toBe("$VEYRA_API_KEY");
    const credential = execFileSync(
      "/bin/sh",
      ["-c", `${config.snippet}\nprintf '%s' "$OPENAI_API_KEY"`],
      { env: { VEYRA_API_KEY: "sk-demo-valid-value" } },
    ).toString();
    expect(credential).toBe("sk-demo-valid-value");
  });
  itUnix(
    "exports selected model and mapping losslessly even with shell metacharacters",
    () => {
      const model = "value'\"\n$(printf injected)";
      const config = buildClientConfiguration({
        target: "claude-code",
        model,
        secret: "sk-demo-test123",
      })!;
      const actual = execFileSync(
        "/bin/sh",
        ["-c", `${config.snippet}\nprintf '%s' "$ANTHROPIC_MODEL"`],
        { env: {} },
      ).toString();
      expect(actual).toBe(model);
      expect(
        Object.values(config.mapping).every((value) => value === model),
      ).toBe(true);
      expect(config.baseUrl).toBe("https://api.veyra.example");
    },
  );
  it("returns no substituted detail for unknown model IDs", () => {
    expect(getDirectoryModel("missing-model")).toBeUndefined();
    expect(
      directoryPrices(
        { ...getDirectoryModel("gpt-4.1")!, online: false },
        "standard",
        createDefaultPricing(),
      ).available,
    ).toBe(false);
  });
  it("computes estimates from one group multiplier and respects operator availability", () => {
    const pricing = createDefaultPricing();
    const model = getDirectoryModel("gpt-4.1")!;
    const current = directoryPrices(model, "standard", pricing);
    expect(current.rates[0].input).toBe("1.2");
    expect(current.minEstimate).toBeCloseTo((1000 * 1.2 + 300 * 4.8) / 1e6);
    pricing.models["gpt-4.1"].tierAvailability.standard = false;
    expect(directoryPrices(model, "standard", pricing).available).toBe(false);
  });
});
