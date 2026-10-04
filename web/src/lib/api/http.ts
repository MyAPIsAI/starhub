// Centralized HTTP client for the management API (/api/*) and relay
// API (/v1/*).
//
// Same-origin by design: the Vite dev server proxies /api and /v1 to
// the Go backend (vite.config.ts); the release build embeds web/dist
// so the browser and API share one origin. The backend has no CORS
// support and rejects cross-origin writes, so credentials:"same-origin"
// is both sufficient and correct.
//
// Response envelope (management API):
//   success -> { success: true, data: <T> }
//   error   -> { success: false, message: string }
//
// The relay API uses OpenAI-shaped errors ({ error: { message, type,
// code } }); that path is handled separately by relay callers.

/** Base URL — empty for same-origin (dev proxy or embedded). Override
 *  with VITE_API_BASE_URL only for standalone frontend deployment. */
const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "";

/** Paths that must not trigger a 401 redirect (the auth pages
 *  themselves, and the self-check request). Prevents redirect loops. */
const AUTH_PATHS = ["/login", "/register", "/forgot-password"];

/** Listener notified once when the session is lost (401). The auth
 *  layer subscribes to clear local state and navigate to /login. */
type AuthLossListener = () => void;
let authLossListener: AuthLossListener | null = null;

export const onAuthLoss = (listener: AuthLossListener): void => {
  authLossListener = listener;
};

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export type ApiResult<T> = T;

/** Low-level request helper. Parses the {success, data|message}
 * envelope, throws ApiError on failure, and passes AbortSignal
 * through for React Query cancellation. */
export async function apiRequest<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const res = await fetch(BASE_URL + path, {
    ...init,
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(init.body ? { "Content-Type": "application/json" } : {}),
      ...(init.headers ?? {}),
    },
  });

  if (res.status === 401) {
    // Avoid redirect loops: don't fire auth-loss when already on an
    // auth page. The caller may still get the 401 to handle locally.
    const pathname = window.location.pathname;
    if (!AUTH_PATHS.includes(pathname) && authLossListener) {
      authLossListener();
    }
    throw new ApiError(401, "session expired");
  }

  let body: unknown;
  try {
    body = await res.json();
  } catch {
    throw new ApiError(res.status, res.statusText || "invalid response");
  }

  if (body && typeof body === "object" && "success" in body) {
    const envelope = body as { success: boolean; data?: T; message?: string };
    if (envelope.success) {
      return envelope.data as T;
    }
    throw new ApiError(res.status, envelope.message ?? "request failed");
  }

  // Non-envelope responses (e.g. relay errors) — surface the status.
  throw new ApiError(res.status, `unexpected response (${res.status})`);
}

export const apiGet = <T>(path: string, signal?: AbortSignal): Promise<T> =>
  apiRequest<T>(path, { method: "GET", signal });

export const apiPost = <T>(path: string, body?: unknown): Promise<T> =>
  apiRequest<T>(path, {
    method: "POST",
    body: body ? JSON.stringify(body) : undefined,
  });

export const apiPut = <T>(path: string, body?: unknown): Promise<T> =>
  apiRequest<T>(path, {
    method: "PUT",
    body: body ? JSON.stringify(body) : undefined,
  });

export const apiDelete = <T>(path: string): Promise<T> =>
  apiRequest<T>(path, { method: "DELETE" });
