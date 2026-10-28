import { describe, it, expect } from "vitest";
import { execFileSync } from "node:child_process";
import {
  buildClientConfiguration,
  findClientModel,
  getClientModels,
} from "./client-config";
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
  describe("connection model selection", () => {
    it.each(["myapis-auto", "veyra-auto"])(
      "matches %s only to an authorized smart option",
      (requested) => {
        for (const target of ["openai", "claude-code"] as const) {
          const options = getClientModels(key, target);
          const selected = findClientModel(options, requested)!;
          expect(selected.kind).toBe("smart");
          expect(options).toContain(selected);
          expect(
            buildClientConfiguration({ target, model: selected.id })!.model,
          ).toBe("myapis-auto");
        }
      },
    );
    it.each(["myapis-auto", "veyra-auto"])(
      "matches either entry against %s options without changing them",
      (id) => {
        const options = [{ id, name: "Auto", kind: "smart" as const }];
        const snapshot = structuredClone(options);
        for (const requested of ["myapis-auto", "veyra-auto"]) {
          expect(findClientModel(options, requested)).toBe(options[0]);
        }
        expect(options).toEqual(snapshot);
      },
    );
    it.each(["myapis-auto", "veyra-auto"])(
      "does not grant smart access for %s through alias matching",
      (requested) => {
        const pricing = createDefaultPricing();
        const denied = [
          getClientModels({ ...key, smartAccess: false }, "openai", pricing),
          getClientModels(
            { ...key, modelAccess: "custom", allowedModels: [requested] },
            "openai",
            pricing,
          ),
          getClientModels(
            { ...key, modelAccess: "custom", allowedModels: ["deepseek-chat"] },
            "claude-code",
            pricing,
          ),
          getClientModels(key, "codex", pricing),
        ];
        for (const options of denied) {
          expect(findClientModel(options, requested)).toBeUndefined();
        }
      },
    );
    it("does not substitute explicit, unavailable, or similar model names", () => {
      const options = getClientModels(key, "openai");
      expect(findClientModel(options, "gpt-4.1")?.id).toBe("gpt-4.1");
      for (const id of [
        "",
        "missing-model",
        "myapis-auto-preview",
        "veyra-auto-preview",
        "MYAPIS-AUTO",
      ]) {
        expect(findClientModel(options, id)).toBeUndefined();
      }
      expect(findClientModel([], "gpt-4.1")).toBeUndefined();
    });
  });
  describe.each(["openai", "claude-code"] as const)(
    "%s model exports",
    (target) => {
      it.each(["myapis-auto", "veyra-auto"])(
        "exports the canonical smart entry for %s",
        (model) => {
          const input = { target, model };
          const config = buildClientConfiguration(input)!;
          expect(config.model).toBe("myapis-auto");
          expect(config.mapping).toEqual(
            target === "openai"
              ? { OPENAI_MODEL: "myapis-auto" }
              : {
                  ANTHROPIC_MODEL: "myapis-auto",
                  ANTHROPIC_DEFAULT_OPUS_MODEL: "myapis-auto",
                  ANTHROPIC_DEFAULT_SONNET_MODEL: "myapis-auto",
                  ANTHROPIC_DEFAULT_HAIKU_MODEL: "myapis-auto",
                  CLAUDE_CODE_SUBAGENT_MODEL: "myapis-auto",
                },
          );
          for (const variable of Object.keys(config.mapping)) {
            expect(config.snippet).toContain(
              `export ${variable}='myapis-auto'`,
            );
          }
          expect(config.snippet).not.toContain("veyra-auto");
          expect(config.credential).toBe("$VEYRA_API_KEY");
          expect(input.model).toBe(model);
        },
      );
      it("preserves explicit model identifiers without prefix normalization", () => {
        for (const model of [
          "gpt-4.1",
          "veyra-auto-preview",
          "myapis-auto-preview",
          "value'\"\n$(printf injected)",
        ]) {
          const config = buildClientConfiguration({ target, model })!;
          expect(config.model).toBe(model);
          expect(
            Object.values(config.mapping).every((value) => value === model),
          ).toBe(true);
        }
      });
      it("keeps masked credentials rejected when normalizing smart entries", () => {
        expect(() =>
          buildClientConfiguration({
            target,
            model: "veyra-auto",
            secret: "sk-review******mask",
          }),
        ).toThrow("masked");
      });
    },
  );
  it("only presents Anthropic candidates to Claude Code and never treats chat as Responses", () => {
    expect(getClientModels(key, "claude-code").map((item) => item.id)).toEqual([
      "claude-sonnet-4-20250514",
      "myapis-auto",
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
      "myapis-auto",
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
