import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  formatTimeQuoteInstant,
  readTimeQuote,
  timeQuoteRefreshDelay,
} from "../src/lib/time-quote";
import { TimePricingNote } from "../src/components/time-pricing-note";
import "../src/i18n";

const fixture = {
  multiplier: 0.5,
  window_id: "offpeak",
  timezone: "UTC",
  quoted_at: "2026-10-29T12:00:40Z",
  valid_until: "2026-10-29T12:01:00Z",
};
describe("actual server time quotes", () => {
  it("preserves exact quote timing and separates missing from unsupported data", () => {
    expect(readTimeQuote(fixture)).toEqual(fixture);
    expect(readTimeQuote(undefined)).toBeUndefined();
    expect(readTimeQuote(null)).toBeUndefined();
    for (const value of [
      {},
      false,
      { ...fixture, multiplier: 0 },
      { ...fixture, multiplier: 101 },
      { ...fixture, window_id: "bad id" },
      { ...fixture, timezone: "" },
      { ...fixture, quoted_at: "invalid" },
      { ...fixture, valid_until: fixture.quoted_at },
      { ...fixture, valid_until: "2026-10-29T12:02:00Z" },
    ])
      expect(readTimeQuote(value)).toBeNull();
  });
  it("refreshes from the server lifetime without depending on browser clock skew", () => {
    expect(timeQuoteRefreshDelay([undefined, null])).toBe(false);
    expect(timeQuoteRefreshDelay([fixture])).toBe(20_000);
    expect(
      timeQuoteRefreshDelay([
        { ...fixture, quoted_at: "2026-10-29T12:00:59.999Z" },
      ]),
    ).toBe(1000);
    expect(timeQuoteRefreshDelay([{}])).toBe(60_000);
  });
  it("shows only actual server window information and keeps unknown fields unavailable", () => {
    expect(renderToStaticMarkup(<TimePricingNote value={undefined} />)).toBe(
      "",
    );
    expect(renderToStaticMarkup(<TimePricingNote value={{}} />)).toContain(
      'role="alert"',
    );
    const html = renderToStaticMarkup(<TimePricingNote value={fixture} />);
    expect(html).toContain('data-time-quote-window="offpeak"');
    expect(html).toMatch(/datetime="2026-10-29T12:01:00Z"/i);
    expect(html).toContain("0.5");
    expect(() =>
      formatTimeQuoteInstant(fixture.quoted_at, "unsupported/timezone", "en"),
    ).not.toThrow();
    expect(
      formatTimeQuoteInstant(fixture.quoted_at, "unsupported/timezone", "en"),
    ).toBe("2026-10-29T12:00:40.000Z");
  });
});
