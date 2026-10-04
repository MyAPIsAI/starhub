import { create } from "zustand";

// Role codes mirror the Go backend (common/config.go): user=1,
// admin=10, root=100. The numeric form lets the frontend compare
// against server-returned roles without a string mapping table.
export const ROLE_USER = 1;
export const ROLE_ADMIN = 10;
export const ROLE_ROOT = 100;

export type SystemRole = number;
const storageKey = "veyra-demo-account-role-v1";

// isAdmin reports whether role carries management privileges. This
// is the single source of truth for admin-only UI; the backend still
// enforces authorization on every write.
export const isAdmin = (role: SystemRole): boolean => role >= ROLE_ADMIN;

function initialRole(): SystemRole {
  try {
    const raw = localStorage.getItem(storageKey);
    if (raw == null) return ROLE_USER;
    const n = Number(raw);
    return Number.isFinite(n) && n > 0 ? n : ROLE_USER;
  } catch {
    return ROLE_USER;
  }
}

// Local demo identity only. Production permissions must be enforced
// by a server. role defaults to ROLE_USER; the demo control bar can
// promote to ROLE_ADMIN for scenario exploration.
export const useSystemAccess = create<{
  role: SystemRole;
  setRole: (role: SystemRole) => void;
}>((set) => ({
  role: initialRole(),
  setRole: (role) => {
    try {
      localStorage.setItem(storageKey, String(role));
    } catch {
      // Keep the identity usable in the current session without persistence.
    }
    set({ role });
  },
}));

export const systemRoleForScenario = (scenario: string): SystemRole =>
  scenario === "pricing" ? ROLE_ADMIN : ROLE_USER;
