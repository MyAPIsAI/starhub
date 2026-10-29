import { create } from "zustand";

// Role codes mirror the Go backend (common/config.go): user=1,
// finance=5, admin=10, root=100. The numeric form lets the frontend compare
// against server-returned roles without a string mapping table.
export const ROLE_USER = 1;
export const ROLE_FINANCE = 5;
export const ROLE_ADMIN = 10;
export const ROLE_ROOT = 100;

export type SystemRole = number;

export interface SessionIdentity {
  accountId: number | null;
  group: string | null;
  epoch: number;
  username?: string | null;
}

// Display only the username supplied by a confirmed server session. Legacy or
// incomplete identities stay unknown rather than inheriting a Demo profile.
export function sessionUsername(session: SessionIdentity): string | null {
  return session.accountId !== null &&
    typeof session.username === "string" &&
    session.username.trim() !== ""
    ? session.username
    : null;
}

export function sessionQueryIdentity(state: {
  session: SessionIdentity;
  role: SystemRole;
}) {
  return [
    state.session.epoch,
    state.session.accountId,
    state.session.group,
    state.role,
  ] as const;
}

export function useSessionQueryIdentity() {
  const session = useSystemAccess((state) => state.session);
  const role = useSystemAccess((state) => state.role);
  return sessionQueryIdentity({ session, role });
}

// isAdmin reports whether role carries management privileges. This
// is the single source of truth for admin-only UI; the backend still
// enforces authorization on every write.
export const isAdmin = (role: SystemRole): boolean =>
  role === ROLE_ADMIN || role === ROLE_ROOT;
export const canReadFinance = (role: SystemRole): boolean =>
  role === ROLE_FINANCE || role === ROLE_ADMIN || role === ROLE_ROOT;

// In-memory only — no localStorage persistence. The role is set by
// session-restore.tsx from GET /api/user/self on every page load, so
// there is no need to persist it across sessions. This controls presentation;
// every protected operation is authorized by the backend independently.
export const useSystemAccess = create<{
  role: SystemRole;
  session: SessionIdentity;
  setRole: (role: SystemRole) => void;
  setSession: (
    user: { id: number; group: string; role: number; username?: string } | null,
  ) => void;
}>((set) => ({
  role: ROLE_USER,
  session: { accountId: null, group: null, epoch: 0, username: null },
  setRole: (role) => set({ role }),
  // Only confirmed session events rotate this identity, never Demo profile edits.
  setSession: (user) =>
    set((state) => ({
      role: user?.role ?? ROLE_USER,
      session: {
        accountId: user?.id ?? null,
        group: user?.group ?? null,
        epoch: state.session.epoch + 1,
        username: user?.username ?? null,
      },
    })),
}));
