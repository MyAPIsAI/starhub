import { create } from "zustand";

// Role codes mirror the Go backend (common/config.go): user=1,
// admin=10, root=100. The numeric form lets the frontend compare
// against server-returned roles without a string mapping table.
export const ROLE_USER = 1;
export const ROLE_ADMIN = 10;
export const ROLE_ROOT = 100;

export type SystemRole = number;

// isAdmin reports whether role carries management privileges. This
// is the single source of truth for admin-only UI; the backend still
// enforces authorization on every write.
export const isAdmin = (role: SystemRole): boolean => role >= ROLE_ADMIN;

// In-memory only — no localStorage persistence. The role is set by
// session-restore.tsx from GET /api/user/self on every page load, so
// there is no need to persist it across sessions. This prevents
// client-side role tampering via devtools.
export const useSystemAccess = create<{
  role: SystemRole;
  setRole: (role: SystemRole) => void;
}>((set) => ({
  role: ROLE_USER,
  setRole: (role) => set({ role }),
}));

export const systemRoleForScenario = (scenario: string): SystemRole =>
  scenario === "pricing" ? ROLE_ADMIN : ROLE_USER;
