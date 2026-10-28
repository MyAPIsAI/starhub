export interface TimeQuote {
  multiplier: number;
  window_id: string;
  timezone: string;
  quoted_at: string;
  valid_until: string;
}

export function readTimeQuote(value: unknown): TimeQuote | null | undefined {
  if (value === undefined || value === null) return undefined;
  if (!value || typeof value !== "object") return null;
  const quote = value as Record<string, unknown>;
  if (
    typeof quote.multiplier !== "number" ||
    !Number.isFinite(quote.multiplier) ||
    quote.multiplier <= 0 ||
    quote.multiplier > 100 ||
    typeof quote.window_id !== "string" ||
    !/^$|^[A-Za-z0-9_.-]{1,64}$/.test(quote.window_id) ||
    typeof quote.timezone !== "string" ||
    !quote.timezone ||
    quote.timezone.length > 64 ||
    typeof quote.quoted_at !== "string" ||
    typeof quote.valid_until !== "string"
  )
    return null;
  const at = Date.parse(quote.quoted_at),
    until = Date.parse(quote.valid_until);
  if (
    !Number.isFinite(at) ||
    !Number.isFinite(until) ||
    until <= at ||
    until - at > 60_000
  )
    return null;
  return quote as unknown as TimeQuote;
}

// Use the server's quote lifetime instead of the browser's wall clock. This
// avoids an incorrect refresh loop when a client's system clock is skewed.
export function timeQuoteRefreshDelay(values: unknown[]): number | false {
  const delays = values
    .map(readTimeQuote)
    .filter((quote) => quote !== undefined)
    .map((quote) =>
      quote
        ? Date.parse(quote.valid_until) - Date.parse(quote.quoted_at)
        : 60_000,
    );
  return delays.length ? Math.max(1000, Math.min(60_000, ...delays)) : false;
}

export function formatTimeQuoteInstant(
  value: string,
  timezone: string,
  language: string,
): string {
  try {
    return new Intl.DateTimeFormat(language, {
      timeZone: timezone,
      dateStyle: "medium",
      timeStyle: "long",
    }).format(new Date(value));
  } catch {
    return new Date(value).toISOString();
  }
}
