import { describe, expect, it } from "vitest";
import { tokenToPreviewKey } from "./key-adapter";
import type { Token } from "./tokens";
import { getClientModels } from "../client-config";

const token: Token = {
  id: 1,
  user_id: 1,
  name: "Review",
  key: "sk-review******mask",
  status: 1,
  group: "pro",
  expired_time: -1,
  remain_quota: 1000000,
  unlimited_quota: true,
  used_quota: 0,
  model_limits: "gpt-4.1",
  allow_ips: "",
  auto_groups: "",
  created_at: 0,
};
describe("backend smart-routing capability", () => {
  it("uses the explicit capability and preserves the actual-model allowlist", () => {
    const key = tokenToPreviewKey({ ...token, smart_access: true });
    expect(getClientModels(key, "openai").map((model) => model.id)).toEqual([
      "gpt-4.1",
      "myapis-auto",
    ]);
    expect(getClientModels(key, "claude-code")).toEqual([]);
    expect(key.allowedModels).toEqual(["gpt-4.1"]);
    expect(key.keySecret).toBe(token.key);
  });
  it.each([false, undefined])(
    "does not infer missing smart permission: %s",
    (smart_access) => {
      const key = tokenToPreviewKey({ ...token, smart_access });
      expect(getClientModels(key, "openai").map((model) => model.id)).toEqual([
        "gpt-4.1",
      ]);
    },
  );
  it("does not turn a virtual-only allowlist into actual model access", () => {
    const key = tokenToPreviewKey({
      ...token,
      smart_access: true,
      model_limits: "veyra-auto,myapis-auto",
    });
    expect(getClientModels(key, "openai")).toEqual([]);
  });
  it("does not treat an expired key as active just because its status is enabled", () => {
    const key = tokenToPreviewKey({
      ...token,
      smart_access: true,
      expired_time: Math.floor(Date.now() / 1000) - 60,
    });
    expect(key.active).toBe(false);
  });
});
