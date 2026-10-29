import { describe, expect, it } from "vitest";
import type { Token } from "../src/lib/api/tokens";
import { reusableModelToken } from "../src/lib/token-model-access";

const session = { accountId: 7, group: "default", epoch: 1 };
const token: Token = {
  id: 9,
  user_id: 7,
  name: "Owned key",
  key: "sk-masked",
  status: 1,
  group: "default",
  expired_time: -1,
  remain_quota: 0,
  unlimited_quota: false,
  used_quota: 0,
  model_limits: "",
  allow_ips: "",
  auto_groups: "",
  created_at: 0,
};
const model = "vendor/server-only:model";
const now = 2_000_000;

describe("server model connection suggestions", () => {
  it("uses exact backend default groups and server-only model IDs without a Demo catalog", () => {
    expect(reusableModelToken([token], session, model, "default", now)).toBe(
      token,
    );
    expect(
      reusableModelToken(
        [{ ...token, group: "pro" }],
        session,
        model,
        "default",
        now,
      ),
    ).toBeUndefined();
    expect(
      reusableModelToken(
        [{ ...token, group: "" }],
        session,
        model,
        "default",
        now,
      )?.id,
    ).toBe(9);
    expect(
      reusableModelToken(
        [{ ...token, group: "" }],
        { ...session, group: "vip" },
        model,
        "default",
        now,
      ),
    ).toBeUndefined();
  });
  it("never recommends another user's key or a guest's cached key", () => {
    expect(
      reusableModelToken(
        [token],
        { ...session, accountId: 8 },
        model,
        "default",
        now,
      ),
    ).toBeUndefined();
    expect(
      reusableModelToken(
        [token],
        { ...session, accountId: null },
        model,
        "default",
        now,
      ),
    ).toBeUndefined();
    expect(
      reusableModelToken(
        [{ ...token, user_id: 8 }, token],
        session,
        model,
        "default",
        now,
      ),
    ).toBe(token);
  });
  it("honors enabled state, exact expiry and safe identity while leaving quota admission to the server", () => {
    for (const change of [
      { status: 2 },
      { id: 0 },
      { id: Number.MAX_SAFE_INTEGER + 1 },
      { expired_time: 2000 },
      { expired_time: NaN },
    ])
      expect(
        reusableModelToken(
          [{ ...token, ...change }],
          session,
          model,
          "default",
          now,
        ),
      ).toBeUndefined();
    expect(
      reusableModelToken(
        [{ ...token, expired_time: 2001 }],
        session,
        model,
        "default",
        now,
      )?.id,
    ).toBe(9);
    expect(
      reusableModelToken([token], session, model, "default", NaN),
    ).toBeUndefined();
  });
  it("requires exact model allowlists and never promotes a virtual-only or prefix grant", () => {
    for (const model_limits of [
      "vendor/server-only",
      "myapis-auto,veyra-auto",
      "other-model",
    ])
      expect(
        reusableModelToken(
          [{ ...token, model_limits }],
          session,
          model,
          "default",
          now,
        ),
      ).toBeUndefined();
    expect(
      reusableModelToken(
        [{ ...token, model_limits: ` other, ${model} ` }],
        session,
        model,
        "default",
        now,
      )?.id,
    ).toBe(9);
    expect(
      reusableModelToken([token], session, "myapis-auto", "default", now),
    ).toBeUndefined();
  });
  it("uses only an actual auto-group member and rejects missing quote context", () => {
    const auto = { ...token, group: "auto", auto_groups: "vip, default" };
    expect(reusableModelToken([auto], session, model, "default", now)).toBe(
      auto,
    );
    expect(
      reusableModelToken([auto], session, model, "pro", now),
    ).toBeUndefined();
    expect(
      reusableModelToken(
        [{ ...auto, auto_groups: "" }],
        session,
        model,
        "default",
        now,
      ),
    ).toBeUndefined();
    expect(
      reusableModelToken([token], session, "", "default", now),
    ).toBeUndefined();
    expect(
      reusableModelToken([token], session, model, "", now),
    ).toBeUndefined();
    expect(
      reusableModelToken([auto], session, model, "auto", now),
    ).toBeUndefined();
  });
});
