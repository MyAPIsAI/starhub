import { describe, expect, it } from "vitest";
import {
  readUserSessionState,
  readSessionTermination,
} from "../src/lib/user-session-state";
const state = {
  schema: "session_state_v1",
  guarded_management: true,
  user_id: 7,
  sessions: [
    { created_at: 100, expires_at: 300, is_current: true },
    { created_at: 200, expires_at: 400, is_current: false },
  ],
};
describe("guarded account device protocol", () => {
  it("accepts a matching explicit contract and detaches the returned public metadata", () => {
    const value = structuredClone(state),
      result = readUserSessionState(value, 7);
    expect(result).toEqual(state);
    value.sessions[0].created_at = 150;
    expect(result.sessions[0].created_at).toBe(100);
  });
  it("never infers support or ownership from a legacy array or incomplete capability", () => {
    for (const value of [
      null,
      [],
      state.sessions,
      {},
      { ...state, schema: "future" },
      { ...state, guarded_management: false },
      { ...state, guarded_management: "true" },
      { ...state, user_id: 8 },
    ])
      expect(() => readUserSessionState(value, 7)).toThrow();
  });
  it("rejects malformed timing, booleans and ambiguous current devices", () => {
    for (const change of [
      { created_at: -1 },
      { created_at: "100" },
      { expires_at: 99 },
      { expires_at: NaN },
      { expires_at: Number.MAX_SAFE_INTEGER },
      { is_current: 1 },
    ])
      expect(() =>
        readUserSessionState(
          { ...state, sessions: [{ ...state.sessions[0], ...change }] },
          7,
        ),
      ).toThrow();
    expect(() =>
      readUserSessionState(
        { ...state, sessions: [state.sessions[0], state.sessions[0]] },
        7,
      ),
    ).toThrow(/Ambiguous/);
  });
  it("retains a genuine empty list without inventing a current device", () => {
    expect(
      readUserSessionState({ ...state, sessions: [] }, 7).sessions,
    ).toEqual([]);
    expect(() => readUserSessionState({ ...state, user_id: 0 }, 0)).toThrow();
  });
  it("requires exact owner and nonnegative whole termination counts including zero", () => {
    expect(readSessionTermination({ user_id: 7, terminated: 0 }, 7)).toEqual({
      user_id: 7,
      terminated: 0,
    });
    for (const value of [
      undefined,
      { terminated: 1 },
      { user_id: 8, terminated: 1 },
      { user_id: 7, terminated: -1 },
      { user_id: 7, terminated: 1.5 },
      { user_id: 7, terminated: "1" },
    ])
      expect(() => readSessionTermination(value, 7)).toThrow(/confirmed/);
    expect(() =>
      readSessionTermination({ user_id: 0, terminated: 0 }, 0),
    ).toThrow();
  });
});
