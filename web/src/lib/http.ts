export class ApiError extends Error {
  constructor(
    message: string,
    public status = 0,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
let generation = 0;
export function invalidateSessionRequests() {
  generation++;
}

function checkNumbers(value: unknown): void {
  if (
    typeof value === "number" &&
    (!Number.isFinite(value) ||
      (Number.isInteger(value) && !Number.isSafeInteger(value)))
  ) {
    throw new ApiError(
      "Unsafe integer in API response; a lossless API format is required.",
      422,
    );
  }
  if (Array.isArray(value)) value.forEach(checkNumbers);
  else if (value && typeof value === "object")
    Object.values(value).forEach(checkNumbers);
}

export async function api<T>(
  path: string,
  options: {
    method?: string;
    body?: unknown;
    signal?: AbortSignal;
    timeout?: number;
    quiet401?: boolean;
  } = {},
): Promise<T> {
  const started = generation;
  const timeout = AbortSignal.timeout(options.timeout ?? 15000);
  const signal = options.signal
    ? AbortSignal.any([timeout, options.signal])
    : timeout;
  let response: Response;
  try {
    response = await fetch(`/api${path}`, {
      method: options.method ?? "GET",
      credentials: "same-origin",
      signal,
      headers:
        options.body === undefined
          ? undefined
          : { "Content-Type": "application/json" },
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch (error) {
    if (options.signal?.aborted) throw error;
    throw new ApiError(
      timeout.aborted
        ? "Request timed out. Refresh to verify the result before trying again."
        : "Network unavailable. Please try again.",
    );
  }
  let payload: { success?: boolean; message?: string; data?: T } | undefined;
  try {
    payload = await response.json();
  } catch {
    /* Proxy errors may not contain JSON. */
  }
  if (started !== generation)
    throw new DOMException("Session changed", "AbortError");
  if (response.status === 401 && !options.quiet401)
    window.dispatchEvent(new Event("starhub:unauthorized"));
  if (!response.ok || payload?.success !== true)
    throw new ApiError(
      payload?.message || `Request failed (${response.status})`,
      response.status,
    );
  checkNumbers(payload.data);
  return payload.data as T;
}

export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
