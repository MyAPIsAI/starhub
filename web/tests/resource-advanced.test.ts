import { expect, it } from "vitest";
import { validPublicationRoute } from "../src/lib/resource-publication";

it("separates reviewed provider identity and rejects undeclared advanced fields", () => {
  const route = {
    upstream_model: "provider-model",
    protocols: ["OpenAI"],
    capabilities: ["text", "vision", "function_call"],
  };
  expect(validPublicationRoute(route, "exact_resource_policy_v2")).toBe(true);
  expect(validPublicationRoute(route, "openai_text_exact_v1")).toBe(false);
  for (const invalid of [
    { ...route, protocols: ["Unknown"] },
    { ...route, protocols: [] },
    { ...route, upstream_model: "" },
    { ...route, capabilities: ["text", "Unknown"] },
    { ...route, capabilities: ["text", "text"] },
  ])
    expect(validPublicationRoute(invalid, "exact_resource_policy_v2")).toBe(
      false,
    );
});
