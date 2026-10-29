import { useQuery } from "@tanstack/react-query";
import { apiGet, ApiError } from "./http";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";
export interface TokenChoices {
  schema: "owned_token_choices_v1";
  guarded_actions: true;
  user_id: number;
  revision: string;
  choices: { group: string; model: string; protocols: string[] }[];
}
export function readTokenChoices(value: unknown, owner: number): TokenChoices {
  const v = value as TokenChoices | null;
  if (
    !v ||
    v.schema !== "owned_token_choices_v1" ||
    v.guarded_actions !== true ||
    v.user_id !== owner ||
    !/^[a-f0-9]{64}$/.test(v.revision) ||
    !Array.isArray(v.choices) ||
    v.choices.some(
      (c) =>
        !c ||
        typeof c.group !== "string" ||
        !c.group ||
        typeof c.model !== "string" ||
        !c.model ||
        !Array.isArray(c.protocols) ||
        c.protocols.some((p) => !["OpenAI", "Anthropic", "Gemini"].includes(p)),
    )
  )
    throw Error("Guarded key choices are unavailable");
  return {
    ...v,
    choices: v.choices.map((c) => ({ ...c, protocols: [...c.protocols] })),
  };
}
export function useTokenChoices() {
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["token-choices", scope],
    enabled: scope[1] !== null,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) => {
      const check = () => {
        if (
          scope[1] === null ||
          JSON.stringify(scope) !==
            JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
        )
          throw Error("Unchanged key session required");
      };
      check();
      try {
        const data = await apiGet(
          `/api/token/owned-choices?expected_user_id=${scope[1]}`,
          signal,
          { handleUnauthorizedLocally: true },
        );
        check();
        return readTokenChoices(data, scope[1]!);
      } catch (e) {
        check();
        if (e instanceof ApiError && e.status === 401)
          useSystemAccess.getState().setSession(null);
        throw e;
      }
    },
  });
}
