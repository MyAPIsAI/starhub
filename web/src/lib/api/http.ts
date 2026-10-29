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

/** Auth pages handle their own unauthorized responses to avoid redirect loops. */
const AUTH_PATHS = ["/login", "/register", "/forgot-password"];

/** Listener notified once when the session is lost (401). The auth
 *  layer subscribes to clear local state and navigate to /login. */
type AuthLossListener = () => void;
type AuthLossSubscription = {
  notify: AuthLossListener;
  readIdentity?: () => string;
  readOwner?: () => number | null;
};
let authLossSubscription: AuthLossSubscription | null = null;

export const onAuthLoss = (
  listener: AuthLossListener,
  readIdentity?: () => string,
  readOwner?: () => number | null,
): (() => void) => {
  const subscription = { notify: listener, readIdentity, readOwner };
  authLossSubscription = subscription;
  return () => {
    if (authLossSubscription === subscription) authLossSubscription = null;
  };
};

export interface ApiRequestOptions {
  /** Session probes handle anonymous visitors without a global redirect. */
  handleUnauthorizedLocally?: boolean;
}

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
  options: ApiRequestOptions = {},
): Promise<T> {
  const subscription = authLossSubscription;
  const requestIdentity = subscription?.readIdentity?.();
  const pathname = path.split(/[?#]/, 1)[0];
  const discoversIdentity =
    (pathname === "/api/user/self" &&
      (init.method ?? "GET").toUpperCase() === "GET") ||
    pathname === "/api/user/login" ||
    pathname === "/api/user/register";
  const owner =
    pathname.startsWith("/api/") && !discoversIdentity
      ? subscription?.readOwner?.()
      : null;
  if (owner != null && (!Number.isSafeInteger(owner) || owner <= 0)) {
    throw new ApiError(
      409,
      "confirmed account is invalid; confirm login again",
    );
  }
  const headers = new Headers(init.headers);
  if (!headers.has("Accept")) headers.set("Accept", "application/json");
  if (init.body && !headers.has("Content-Type"))
    headers.set("Content-Type", "application/json");
  if (owner != null) headers.set("X-StarHub-Expected-User-ID", String(owner));
  const res = await fetch(BASE_URL + path, {
    ...init,
    credentials: "same-origin",
    headers,
  });

  if (res.status === 401) {
    // Avoid redirect loops: don't fire auth-loss when already on an
    // auth page. A superseded identity or subscription cannot sign out its
    // replacement. The caller still receives the 401 for local error handling.
    const pathname = window.location.pathname;
    if (
      !options.handleUnauthorizedLocally &&
      !AUTH_PATHS.includes(pathname) &&
      subscription &&
      authLossSubscription === subscription &&
      requestIdentity === subscription.readIdentity?.()
    ) {
      subscription.notify();
    }
    throw new ApiError(401, "session expired");
  }

  if (owner != null) {
    const guard = res.headers?.get("X-StarHub-Owner-Guard");
    const subject = res.headers?.get("X-StarHub-User-ID");
    // Legacy servers may omit both headers. Explicit owner metadata must match;
    // action-specific capabilities still govern writes to older deployments.
    if (
      (guard != null || subject != null) &&
      (guard !== "v1" || subject !== String(owner))
    ) {
      throw new ApiError(
        409,
        "response account differs from the confirmed account",
      );
    }
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

export const apiGet = <T>(
  path: string,
  signal?: AbortSignal,
  options?: ApiRequestOptions,
): Promise<T> => apiRequest<T>(path, { method: "GET", signal }, options);

export const apiPost = <T>(
  path: string,
  body?: unknown,
  options?: ApiRequestOptions,
): Promise<T> =>
  apiRequest<T>(
    path,
    {
      method: "POST",
      body: body ? JSON.stringify(body) : undefined,
    },
    options,
  );

export const apiPut = <T>(
  path: string,
  body?: unknown,
  options?: ApiRequestOptions,
): Promise<T> =>
  apiRequest<T>(
    path,
    {
      method: "PUT",
      body: body ? JSON.stringify(body) : undefined,
    },
    options,
  );

export const apiDelete = <T>(
  path: string,
  options?: ApiRequestOptions,
): Promise<T> => apiRequest<T>(path, { method: "DELETE" }, options);
