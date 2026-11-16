import { describe, expect, it } from "vitest";
import { phoneCooldownKey, phoneRetrySeconds } from "./phone-cooldown";

describe("phone cooldown identity and retry intervals", () => {
  it("groups mainland China dialing and formatting aliases", () => {
    const expected = phoneCooldownKey("13812345678");
    for (const value of [
      "+8613812345678",
      "008613812345678",
      "8613812345678",
      "+86 (138) 1234-5678",
    ])
      expect(phoneCooldownKey(value, "cn")).toBe(expected);
    expect(phoneCooldownKey("13912345678")).not.toBe(expected);
  });

  it("normalizes international formatting and isolates deployment regions", () => {
    expect(phoneCooldownKey("00971 50 123-4567", "AE")).toBe(
      phoneCooldownKey("+971501234567", "AE"),
    );
    expect(phoneCooldownKey("13812345678", "CN")).not.toBe(
      phoneCooldownKey("13812345678", "AE"),
    );
  });

  it("uses server intervals without imposing a fixed minute", () => {
    expect(phoneRetrySeconds(3547)).toBe(3547);
    expect(phoneRetrySeconds(17)).toBe(17);
    for (const value of [undefined, null, 0, -5, 1.5, Infinity, "3600"])
      expect(phoneRetrySeconds(value)).toBe(60);
  });
});
