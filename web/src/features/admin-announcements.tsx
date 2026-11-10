import { message as translatedMessage } from "../i18n/messages";
import { regionalLocale } from "../lib/locale-format";

import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import { apiGet, apiPost, apiPut, ApiError } from "../lib/api/http";
import { useStaffText } from "../lib/api/staff";
import { useSessionQueryIdentity } from "../lib/system-access";
import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import { AnnouncementTargetPicker } from "./announcement-target-picker";
import {
  announcementStatus,
  announcementStatuses,
  type Announcement,
  type AnnouncementList,
  type AnnouncementLanguage,
  type AnnouncementStatus,
} from "../lib/api/announcements";
import "./announcements.css";

const blankText = () => ({ zh: "", en: "", ar: "" });
function newDraft(row?: Announcement) {
  return {
    id: row?.id,
    expected_version: row?.version ?? 0,
    operation_id: crypto.randomUUID(),
    title: { ...(row?.title ?? blankText()) },
    content: { ...(row?.content ?? blankText()) },
    vendors: [...(row?.vendors ?? [])],
    models: [...(row?.models ?? [])],
    status: row?.status ?? ("investigating" as AnnouncementStatus),
  };
}

export function AdminAnnouncements() {
  const t = useStaffText(),
    language = writtenLanguage(usePreferences((s) => s.language)),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const [status, setStatus] = useState("all"),
    [page, setPage] = useState(1),
    [draft, setDraft] = useState<ReturnType<typeof newDraft> | null>(null),
    [ending, setEnding] = useState<Announcement | null>(null),
    [validation, setValidation] = useState("");
  const query = useQuery({
    queryKey: ["staff-announcements", ...identity, status, page],
    queryFn: ({ signal }) =>
      apiGet<AnnouncementList>(
        `/api/admin/announcements?status=${status}&page=${page}`,
        signal,
      ),
    refetchInterval: 30_000,
  });
  const mutation = useMutation({
    mutationFn: ({ id, data }: { id?: number; data: unknown }) =>
      id
        ? apiPut<Announcement>(`/api/admin/announcements/${id}`, data)
        : apiPost<Announcement>("/api/admin/announcements", data),
    onSuccess: () => {
      setDraft(null);
      setEnding(null);
      setValidation("");
      void cache.invalidateQueries({ queryKey: ["staff-announcements"] });
      void cache.invalidateQueries({ queryKey: ["published-announcements"] });
    },
  });
  const error =
    mutation.error instanceof ApiError && mutation.error.status === 409
      ? t(
          ...translatedMessage(
            "this_notice_changed_keep_your_draft_close_the_009a6e459d",
          ),
        )
      : mutation.error
        ? t(
            ...translatedMessage(
              "unable_to_save_check_the_content_or_refresh_t_8500026237",
            ),
          )
        : "";
  const open = (row?: Announcement) => {
    mutation.reset();
    setValidation("");
    setDraft(newDraft(row));
  };
  const date = (at: number) =>
    new Intl.DateTimeFormat(regionalLocale(language), {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(at * 1000));
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!draft) return;
    if (!draft.vendors.length && !draft.models.length) {
      setValidation(
        t(
          ...translatedMessage(
            "select_at_least_one_affected_vendor_or_model_ea47bf38d7",
          ),
        ),
      );
      return;
    }
    setValidation("");
    const { id, ...data } = draft;
    mutation.mutate({
      id,
      data,
    });
  };
  const rows = query.isError ? undefined : query.data;
  return (
    <div className="announcement-admin">
      <header className="announcement-heading">
        <div>
          <h1>{t(...translatedMessage("incident_notices_04954219fa"))}</h1>
          <p>
            {t(
              ...translatedMessage(
                "publish_multilingual_notices_to_the_public_st_74acc426f5",
              ),
            )}
          </p>
        </div>
        <Button variant="primary" onClick={() => open()}>
          {t(...translatedMessage("publish_notice_4dfde990ac"))}
        </Button>
      </header>
      <div className="announcement-actions">
        <label>
          {t(...translatedMessage("status_dda39dcd63"))}{" "}
          <select
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              setPage(1);
            }}
          >
            <option value="all">
              {t(...translatedMessage("all_ac49939df0"))}
            </option>
            <option value="active">
              {t(...translatedMessage("active_01efe42d2c"))}
            </option>
            <option value="resolved">
              {t(...translatedMessage("resolved_6f9b1bbcb3"))}
            </option>
          </select>
        </label>
        <Button
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {t(...translatedMessage("refresh_9c83c3600a"))}
        </Button>
        <a href="/status" target="_blank" rel="noreferrer">
          {t(...translatedMessage("view_public_status_page_399c881704"))}
        </a>
      </div>
      {query.isPending && (
        <p role="status">
          {t(...translatedMessage("loading_notices_d4586c0781"))}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {t(
            ...translatedMessage(
              "unable_to_load_notices_please_retry_d02763651c",
            ),
          )}
        </p>
      )}
      {rows && (
        <>
          <div className="announcement-list">
            {rows.items.map((row) => (
              <article
                className="announcement-card"
                key={row.id}
                data-announcement-id={row.id}
              >
                <header>
                  <h2>{row.title[language]}</h2>
                  <span className="announcement-badge" data-status={row.status}>
                    {announcementStatus(row.status, t)}
                  </span>
                </header>
                <p className="announcement-body">{row.content[language]}</p>
                <p className="announcement-impact">
                  {t(...translatedMessage("affected_services_92edbbef84"))}:{" "}
                  {[...row.vendors, ...row.models].join(" · ")}
                </p>
                <footer>
                  <span>
                    {t(...translatedMessage("updated_b5874cbe07"))}{" "}
                    {date(row.updated_at)} ·{" "}
                    {t(...translatedMessage("version_9d2b4f1f94"))}{" "}
                    {row.version}
                  </span>
                  <div className="announcement-actions">
                    <Button onClick={() => open(row)}>
                      {t(...translatedMessage("edit_6224671192"))}
                    </Button>
                    {row.status !== "resolved" && (
                      <Button
                        onClick={() => {
                          mutation.reset();
                          setEnding(row);
                        }}
                      >
                        {t(...translatedMessage("resolve_notice_6f749840ae"))}
                      </Button>
                    )}
                  </div>
                </footer>
              </article>
            ))}
          </div>
          {!rows.items.length && (
            <p>{t(...translatedMessage("no_notices_0456af6ada"))}</p>
          )}
          <nav
            className="announcement-actions"
            aria-label={t(...translatedMessage("notice_pages_375fe473bf"))}
          >
            <span>
              {t(...translatedMessage("total_301e42d384"))} {rows.total}
            </span>
            <Button disabled={page <= 1} onClick={() => setPage(page - 1)}>
              {t(...translatedMessage("previous_2672646f39"))}
            </Button>
            <span dir="ltr">
              {page} / {Math.max(1, Math.ceil(rows.total / 20))}
            </span>
            <Button
              disabled={page * 20 >= rows.total}
              onClick={() => setPage(page + 1)}
            >
              {t(...translatedMessage("next_c0fd39244f"))}
            </Button>
          </nav>
        </>
      )}
      <Modal
        size="wide"
        preventOutsideClose
        open={draft !== null}
        onOpenChange={(v) => {
          if (!v && !mutation.isPending) setDraft(null);
        }}
        title={
          draft?.id
            ? t(...translatedMessage("edit_notice_2f9d8482d6"))
            : t(...translatedMessage("publish_notice_4dfde990ac"))
        }
        description={
          draft?.status === "resolved"
            ? t(
                ...translatedMessage(
                  "editing_history_does_not_republish_a_resolved_e54ff8e2d8",
                ),
              )
            : t(
                ...translatedMessage(
                  "complete_all_three_languages_saving_publishes_b684d19aff",
                ),
              )
        }
        footer={
          <>
            <Button
              disabled={mutation.isPending}
              onClick={() => setDraft(null)}
            >
              {t(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            <Button
              variant="primary"
              form="announcement-form"
              type="submit"
              disabled={mutation.isPending}
            >
              {mutation.isPending
                ? t(...translatedMessage("saving_674c5a4814"))
                : draft?.id
                  ? t(...translatedMessage("save_changes_bce1fae3b4"))
                  : t(...translatedMessage("publish_notice_4dfde990ac"))}
            </Button>
          </>
        }
      >
        {draft && (
          <form
            id="announcement-form"
            className="announcement-form"
            onSubmit={submit}
          >
            <div className="announcement-locales">
              {(["zh", "en", "ar"] as AnnouncementLanguage[]).map((locale) => (
                <fieldset
                  key={locale}
                  lang={locale}
                  dir={locale === "ar" ? "rtl" : "ltr"}
                >
                  <legend>
                    {
                      {
                        zh: translatedMessage("ui_english_ce10e061")[0],
                        en: translatedMessage("ui_english_ce10e061")[1],
                        ar: translatedMessage("ui_english_ce10e061")[2],
                      }[locale]
                    }
                  </legend>
                  <label htmlFor={`notice-title-${locale}`}>
                    {t(...translatedMessage("title_98815d8ffe"))}
                  </label>
                  <Input
                    id={`notice-title-${locale}`}
                    required
                    maxLength={200}
                    value={draft.title[locale]}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        title: { ...draft.title, [locale]: e.target.value },
                      })
                    }
                  />
                  <label htmlFor={`notice-content-${locale}`}>
                    {t(...translatedMessage("description_c6bae5d4e7"))}
                  </label>
                  <textarea
                    id={`notice-content-${locale}`}
                    className="input"
                    required
                    maxLength={5000}
                    rows={7}
                    value={draft.content[locale]}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        content: { ...draft.content, [locale]: e.target.value },
                      })
                    }
                  />
                </fieldset>
              ))}
            </div>
            <AnnouncementTargetPicker
              vendors={draft.vendors}
              models={draft.models}
              disabled={mutation.isPending}
              onChange={(selection) => setDraft({ ...draft, ...selection })}
            />
            <label>
              {t(...translatedMessage("incident_status_d68557ec46"))}{" "}
              <select
                id="notice-status"
                value={draft.status}
                disabled={draft.status === "resolved"}
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    status: e.target.value as AnnouncementStatus,
                  })
                }
              >
                {announcementStatuses
                  .filter(
                    (s) => s !== "resolved" || draft.status === "resolved",
                  )
                  .map((s) => (
                    <option key={s} value={s}>
                      {announcementStatus(s, t)}
                    </option>
                  ))}
              </select>
            </label>
            {(validation || error) && <p role="alert">{validation || error}</p>}
          </form>
        )}
      </Modal>
      <Modal
        size="compact"
        open={ending !== null}
        onOpenChange={(v) => {
          if (!v && !mutation.isPending) setEnding(null);
        }}
        title={t(...translatedMessage("resolve_notice_6f749840ae"))}
        description={t(
          ...translatedMessage(
            "the_notice_will_leave_the_public_status_page__1f4ff2e0f9",
          ),
        )}
        footer={
          <>
            <Button
              disabled={mutation.isPending}
              onClick={() => setEnding(null)}
            >
              {t(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            <Button
              variant="primary"
              disabled={mutation.isPending}
              onClick={() =>
                ending &&
                mutation.mutate({
                  id: ending.id,
                  data: {
                    expected_version: ending.version,
                    title: ending.title,
                    content: ending.content,
                    vendors: ending.vendors,
                    models: ending.models,
                    status: "resolved",
                  },
                })
              }
            >
              {t(...translatedMessage("confirm_resolution_7933508b7a"))}
            </Button>
          </>
        }
      >
        <p>{ending?.title[language]}</p>
        {error && <p role="alert">{error}</p>}
      </Modal>
    </div>
  );
}
