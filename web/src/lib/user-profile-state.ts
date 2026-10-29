export interface UserProfileState {
  schema: "profile_state_v1";
  guarded_updates: boolean;
  user: {
    id: number;
    username: string;
    email: string;
    role: number;
    status: number;
    created_at: number;
  };
}

export function readUserProfileState(
  value: unknown,
  owner: number,
): UserProfileState {
  const data = value as Record<string, unknown> | null;
  const user = data?.user as Record<string, unknown> | null;
  if (
    !data ||
    typeof data !== "object" ||
    data.schema !== "profile_state_v1" ||
    !user ||
    typeof user !== "object" ||
    !Number.isSafeInteger(owner) ||
    owner <= 0 ||
    user.id !== owner ||
    typeof user.username !== "string" ||
    !user.username.trim() ||
    typeof user.email !== "string" ||
    !Number.isSafeInteger(user.role) ||
    !Number.isSafeInteger(user.status) ||
    !Number.isSafeInteger(user.created_at) ||
    (user.created_at as number) < 0 ||
    !Number.isFinite(new Date((user.created_at as number) * 1000).getTime())
  )
    throw Error("Account profile ownership or metadata could not be verified");
  return {
    schema: "profile_state_v1",
    guarded_updates: data.guarded_updates === true,
    user: {
      id: owner,
      username: user.username,
      email: user.email,
      role: user.role as number,
      status: user.status as number,
      created_at: user.created_at as number,
    },
  };
}
