import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { History } from "lucide-react";
import { Button } from "../components/ui/button";
import { useUserSecurityHistory } from "../lib/api/user";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import { useSystemAccess } from "../lib/system-access";

export function AccountSecurityHistory() {
  const query = useUserSecurityHistory();
  const events = query.isError ? [] : (query.data ?? []);
  const language = usePreferences((state) => state.language);
  const owner = useSystemAccess((state) => state.session.accountId);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const mismatch =
    query.error instanceof ApiError && query.error.status === 409;
  const date = (value: number) =>
    new Intl.DateTimeFormat(regionalLocale(language), {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value * 1000));
  return (
    <section className="ac-card" aria-labelledby="ac-history-title">
      <div className="ac-section-heading">
        <div>
          <h2 id="ac-history-title">
            <History size={18} />
            {c(...translatedMessage("recent_security_activity_4124050334"))}
          </h2>
          <p>
            {c(
              ...translatedMessage(
                "latest_5_changes_covers_successful_password_a_4a946c31d1",
              ),
            )}
          </p>
        </div>
      </div>
      {events.length > 0 && (
        <ol className="ac-events">
          {events.map((event) => (
            <li key={event.id}>
              <span className="ac-event-dot" />
              <div>
                <strong>
                  {event.action === "password_changed"
                    ? c(...translatedMessage("password_changed_014a46f6c9"))
                    : c(
                        ...translatedMessage(
                          "profile_email_changed_84c6c2069a",
                        ),
                      )}
                </strong>
                <time
                  dateTime={new Date(event.created_at * 1000).toISOString()}
                >
                  {date(event.created_at)}
                </time>
              </div>
            </li>
          ))}
        </ol>
      )}
      {query.isLoading && (
        <p role="status" className="ac-history-status">
          {c(...translatedMessage("loading_security_history_1dbcc73cce"))}
        </p>
      )}
      {query.isError && (
        <p role="alert" className="ac-history-status">
          {mismatch
            ? c(
                ...translatedMessage(
                  "the_signed_in_account_may_have_changed_confir_2764a99c4c",
                ),
              )
            : c(
                ...translatedMessage(
                  "security_history_is_unavailable_previous_reco_3dd1bc577a",
                ),
              )}
        </p>
      )}
      {!query.isError &&
        !query.isLoading &&
        query.isSuccess &&
        events.length === 0 && (
          <p className="ac-history-status">
            {c(
              ...translatedMessage(
                "no_changes_recorded_in_this_scope_5f776b1c64",
              ),
            )}
          </p>
        )}
      <div className="ac-device-actions">
        <Button
          size="small"
          variant="ghost"
          disabled={query.isFetching || owner === null}
          onClick={() =>
            mismatch ? window.location.reload() : void query.refetch()
          }
        >
          {mismatch
            ? c(...translatedMessage("confirm_current_account_9bdf8613b1"))
            : c(...translatedMessage("reload_security_history_c476457ed3"))}
        </Button>
      </div>
    </section>
  );
}
