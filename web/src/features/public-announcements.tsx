import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { Button } from "../components/ui/button";
import { useStaffText } from "../lib/api/staff";
import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import {
  announcementStatus,
  usePublishedAnnouncements,
} from "../lib/api/announcements";
import "./announcements.css";

export function PublicAnnouncements({
  query,
  timezone,
}: {
  query: ReturnType<typeof usePublishedAnnouncements>;
  timezone?: string;
}) {
  const t = useStaffText(),
    language = writtenLanguage(usePreferences((s) => s.language));
  if (query.isError)
    return (
      <div className="announcement-card" role="alert">
        {t(
          ...translatedMessage(
            "incident_notices_are_temporarily_unavailable_6cb4f6ae77",
          ),
        )}{" "}
        <Button onClick={() => void query.refetch()}>
          {t(...translatedMessage("retry_c16455021c"))}
        </Button>
      </div>
    );
  if (query.isPending)
    return (
      <p role="status">
        {t(...translatedMessage("loading_incident_notices_c20e459312"))}
      </p>
    );
  if (!query.data.items.length) return null;
  return (
    <section
      className="public-announcements"
      aria-label={t(...translatedMessage("active_incident_notices_9c5bbc612e"))}
    >
      <h2>{t(...translatedMessage("incident_notices_04954219fa"))}</h2>
      {query.data.items.map((row) => (
        <article
          key={row.id}
          className="announcement-card"
          data-public-announcement={row.id}
        >
          <header>
            <h3>{row.title[language]}</h3>
            <span className="announcement-badge" data-status={row.status}>
              {announcementStatus(row.status, t)}
            </span>
          </header>
          <p className="announcement-body">{row.content[language]}</p>
          <p className="announcement-impact">
            {t(...translatedMessage("affected_services_92edbbef84"))}:{" "}
            {[...row.vendors, ...row.models].join(" · ")}
          </p>
          <small>
            {t(...translatedMessage("updated_b5874cbe07"))}{" "}
            <time dateTime={new Date(row.updated_at * 1000).toISOString()}>
              {new Intl.DateTimeFormat(regionalLocale(language), {
                dateStyle: "medium",
                timeStyle: "short",
                timeZone: timezone,
              }).format(new Date(row.updated_at * 1000))}
            </time>
            {timezone && (
              <>
                {" "}
                · <bdi>{timezone}</bdi>
              </>
            )}
          </small>
        </article>
      ))}
    </section>
  );
}
