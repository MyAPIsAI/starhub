import { create } from "zustand";

export type SystemRole = "superAdmin" | "customer";
const storageKey = "veyra-demo-account-role-v1";

function initialRole(): SystemRole {
  try {
    return localStorage.getItem(storageKey) === "customer"
      ? "customer"
      : "superAdmin";
  } catch {
    return "superAdmin";
  }
}

// Local demo identity only. Production permissions must be enforced by a server.
export const useSystemAccess = create<{
  role: SystemRole;
  setRole: (role: SystemRole) => void;
}>((set) => ({
  role: initialRole(),
  setRole: (role) => {
    try {
      localStorage.setItem(storageKey, role);
    } catch {
      // Keep the identity usable in the current session without persistence.
    }
    set({ role });
  },
}));

export const systemRoleForScenario = (scenario: string): SystemRole =>
  scenario === "pricing" ? "superAdmin" : "customer";
