import type { Token } from "./api/tokens";
import type { SessionIdentity } from "./system-access";
import { isSmartModelId } from "./smart-model";

// This is a connection suggestion based on server-returned ownership and the
// current server model/group quote. Every actual request is authorized again.
export function reusableModelToken(
  tokens: readonly Token[],
  session: SessionIdentity,
  model: string,
  quoteGroup: string,
  now = Date.now(),
): Token | undefined {
  if (
    session.accountId === null ||
    !model ||
    !quoteGroup ||
    quoteGroup === "auto" ||
    isSmartModelId(model) ||
    !Number.isFinite(now)
  )
    return undefined;
  const list = (value: string) =>
    value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  return tokens.find((token) => {
    const group = token.group || session.group || "default";
    const limits = list(token.model_limits);
    return (
      Number.isSafeInteger(token.id) &&
      token.id > 0 &&
      token.user_id === session.accountId &&
      token.status === 1 &&
      Number.isSafeInteger(token.expired_time) &&
      (token.expired_time <= 0 || token.expired_time * 1000 > now) &&
      (group === quoteGroup ||
        (group === "auto" && list(token.auto_groups).includes(quoteGroup))) &&
      (limits.length === 0 || limits.includes(model))
    );
  });
}
