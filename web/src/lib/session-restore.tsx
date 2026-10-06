import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { authUserProfile, getSelf } from "./api/auth";
import { ApiError } from "./api/http";
import { useProductPreview } from "./product-preview";
import { ROLE_USER, useSystemAccess } from "./system-access";

type RestoreStatus = "checking" | "ready" | "unavailable";

interface SessionRestoreState {
  status: RestoreStatus;
  complete: () => void;
  retry: () => void;
}

const SessionRestoreContext = createContext<SessionRestoreState>({
  status: "ready",
  complete: () => {},
  retry: () => {},
});

export function useSessionRestore() {
  return useContext(SessionRestoreContext);
}

export function SessionRestoreProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<RestoreStatus>("checking");
  const [attempt, setAttempt] = useState(0);
  const activeRequest = useRef<AbortController | null>(null);
  const complete = useCallback(() => {
    // A successful interactive login supersedes an older startup probe.
    activeRequest.current?.abort();
    setStatus("ready");
  }, []);
  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    activeRequest.current = controller;
    setStatus("checking");
    getSelf({ handleUnauthorizedLocally: true }, controller.signal)
      .then((user) => {
        if (controller.signal.aborted) return;
        useSystemAccess.getState().setRole(user.role);
        useProductPreview
          .getState()
          .login(user.email ?? "", user.username, authUserProfile(user));
        setStatus("ready");
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiError && error.status === 401) {
          useProductPreview.getState().logout();
          useSystemAccess.getState().setRole(ROLE_USER);
          setStatus("ready");
        } else {
          // A transport or server failure does not establish an anonymous session.
          setStatus("unavailable");
        }
      });
    return () => controller.abort();
  }, [attempt]);

  const value = useMemo(
    () => ({ status, complete, retry }),
    [status, complete, retry],
  );
  return (
    <SessionRestoreContext.Provider value={value}>
      {children}
    </SessionRestoreContext.Provider>
  );
}
