import { message as translatedMessage } from "../i18n/messages";
import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import { formatTimeQuoteInstant, readTimeQuote } from "../lib/time-quote";
import "./time-pricing-note.css";

export function TimePricingNote({
  value,
  compact = false,
}: {
  value: unknown;
  compact?: boolean;
}) {
  const lang = writtenLanguage(usePreferences((state) => state.language));
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const quote = readTimeQuote(value);
  if (quote === undefined) return null;
  if (quote === null)
    return (
      <p role="alert" className="time-pricing-note">
        {text(
          ...translatedMessage(
            "time_pricing_information_is_unavailable_refre_171f1c4c7b",
          ),
        )}
      </p>
    );
  const at = formatTimeQuoteInstant(quote.quoted_at, quote.timezone, lang),
    until = formatTimeQuoteInstant(quote.valid_until, quote.timezone, lang);
  return (
    <div
      className={`time-pricing-note${compact ? " is-compact" : ""}`}
      data-time-quote-window={quote.window_id}
      data-token-price-window={quote.token_window?.window_id}
    >
      <p>
        {text(...translatedMessage("time_multiplier_included_d0d4bc3779"))}:{" "}
        <bdi>{quote.multiplier}×</bdi> · <bdi>{quote.timezone}</bdi>
        {quote.window_id && (
          <>
            {" "}
            · <bdi>{quote.window_id}</bdi>
          </>
        )}
      </p>
      {quote.token_window && (
        <p>
          {quote.token_window.window_id
            ? text(
                ...translatedMessage(
                  "independent_window_token_prices_included_2dfe517da9",
                ),
              )
            : text(
                ...translatedMessage(
                  "outside_independent_price_windows_base_token__cd56a7d218",
                ),
              )}
          {" · "}
          <bdi>{quote.token_window.timezone}</bdi>
          {quote.token_window.window_id && (
            <>
              {" "}
              · <bdi>{quote.token_window.window_id}</bdi>
            </>
          )}
        </p>
      )}
      {compact ? (
        <p>
          <time dateTime={quote.quoted_at}>{at}</time>
        </p>
      ) : (
        <>
          <p>
            {text(...translatedMessage("quoted_at_408d9c705d"))}:{" "}
            <time dateTime={quote.quoted_at}>{at}</time>
          </p>
          <p>
            {text(...translatedMessage("this_quote_is_valid_until_c18095f798"))}
            : <time dateTime={quote.valid_until}>{until}</time>
          </p>
          <p>
            {text(
              ...translatedMessage(
                "the_page_refreshes_using_the_server_quote_lif_49d51d0eb2",
              ),
            )}
          </p>
        </>
      )}
    </div>
  );
}
