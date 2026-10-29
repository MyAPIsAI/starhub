import { describe, expect, it } from "vitest";
import {
  readDeactivationState,
  readCloseReceipt,
  type CloseInput,
} from "../src/lib/api/deactivation";
const state = {
  schema: "deactivation_state_v1",
  policy: "dust_below_cent_v1",
  guarded_deactivation: true,
  user_id: 7,
  account_revision: "a".repeat(64),
  wallet_balance_nano: "9999999",
  pending_recharges: 0,
  last_active_root: false,
  requires_dust_acknowledgement: true,
  dust_limit_nano: "10000000",
  blocked_reasons: [],
};
const input: CloseInput = {
  schema: "deactivation_action_v1",
  expected_user_id: 7,
  account_revision: state.account_revision,
  operation_id: "b".repeat(32),
  acknowledge_dust: true,
  expected_writeoff_nano: "9999999",
};
const receipt = {
  schema: "deactivation_receipt_v1",
  user_id: 7,
  account_revision: state.account_revision,
  operation_id: input.operation_id,
  closed: true,
  keys_disabled: true,
  written_off_nano: "9999999",
};
describe("guarded closure contract", () => {
  it("preserves exact dust and detaches blocker arrays", () => {
    const got = readDeactivationState(state, 7);
    expect(got.wallet_balance_nano).toBe("9999999");
    expect(got.blocked_reasons).not.toBe(state.blocked_reasons);
    expect(readCloseReceipt(receipt, input)).toEqual(receipt);
  });
  it.each([
    {},
    { user_id: 8 },
    { wallet_balance_nano: 9999999 },
    { wallet_balance_nano: "099" },
    { wallet_balance_nano: "9223372036854775808" },
    { requires_dust_acknowledgement: false },
    { last_active_root: true },
    { pending_recharges: 1 },
    { blocked_reasons: ["unknown"] },
    { account_revision: "bad" },
  ])("rejects malformed or unguarded preview fields %j", (patch) => {
    const v = Object.keys(patch).length ? { ...state, ...patch } : {};
    expect(() => readDeactivationState(v, 7)).toThrow();
  });
  it("does not infer a write capability on an old server", () => {
    expect(
      readDeactivationState({ ...state, guarded_deactivation: undefined }, 7)
        .guarded_deactivation,
    ).toBe(false);
  });
  it.each([
    { user_id: 8 },
    { operation_id: "c".repeat(32) },
    { account_revision: "d".repeat(64) },
    { written_off_nano: "1" },
    { closed: false },
    { keys_disabled: false },
    { schema: "legacy" },
  ])("requires a receipt matching the entire frozen action %j", (patch) => {
    expect(() => readCloseReceipt({ ...receipt, ...patch }, input)).toThrow(
      /not confirmed/,
    );
  });
});
