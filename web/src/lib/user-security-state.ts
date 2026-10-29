export interface UserSecurityState {
  schema: "security_state_v1";
  history_scope: "guarded_profile_changes_v1";
  user_id: number;
  events: {
    id: number;
    action: "password_changed" | "email_changed";
    created_at: number;
  }[];
}

export function readUserSecurityState(
  value: unknown,
  owner: number,
): UserSecurityState {
  const data = value as Record<string, unknown> | null;
  if (
    !data ||
    typeof data !== "object" ||
    data.schema !== "security_state_v1" ||
    data.history_scope !== "guarded_profile_changes_v1" ||
    !Number.isSafeInteger(owner) ||
    owner <= 0 ||
    data.user_id !== owner ||
    !Array.isArray(data.events) ||
    data.events.length > 5
  )
    throw Error("Account security history could not be verified");
  let previous = Infinity;
  const events = data.events.map((raw): UserSecurityState["events"][number] => {
    const row = raw as Record<string, unknown> | null;
    if (
      !row ||
      typeof row !== "object" ||
      !Number.isSafeInteger(row.id) ||
      (row.id as number) <= 0 ||
      (row.id as number) >= previous ||
      (row.action !== "password_changed" && row.action !== "email_changed") ||
      !Number.isSafeInteger(row.created_at) ||
      (row.created_at as number) <= 0 ||
      !Number.isFinite(new Date((row.created_at as number) * 1000).getTime())
    )
      throw Error("Invalid account security history");
    previous = row.id as number;
    return {
      id: row.id as number,
      action: row.action,
      created_at: row.created_at as number,
    };
  });
  return {
    schema: "security_state_v1",
    history_scope: "guarded_profile_changes_v1",
    user_id: owner,
    events,
  };
}
