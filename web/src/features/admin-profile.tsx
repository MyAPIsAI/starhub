import { ListPagination } from "../components/list-pagination";
import { messagePair } from "../i18n/messages";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Drawer, ConfirmDialog } from "../components/ui/overlay";
import { apiGet, apiPut, apiPost } from "../lib/api/http";
import { useSessionQueryIdentity } from "../lib/system-access";
import {
  AdminFeedback,
  AdminTable,
  AdminPagination,
  adminDate,
  useAdminText,
} from "./admin-shared";

interface Profile {
  region_id: string;
  brand: string;
  domain: string;
  logo_url: string;
  default_language: string;
  languages: string[];
  timezone: string;
  currency: string;
  allowed_vendors: string[];
  login_methods: string[];
  payment_channels: string[];
  payment_config_refs: Record<string, string>;
  cookie_consent: boolean;
  cookie_consent_mode?: "banner" | "dialog";
  features: Record<string, boolean>;
  store_request_content: boolean;
}
interface ProfileVersion {
  revision: number;
  profile: Profile;
  actor_id: number;
  created_at: number;
}
interface LegalVersion {
  status: "current" | "scheduled" | "historical";
  id: number;
  kind: string;
  language: string;
  version: string;
  content: string;
  effective_at: number;
  timezone: string;
  actor_id: number;
  created_at: number;
}
const featureNames = [
  ["playground", "调试台", "Playground"],
  ["models", "模型广场", "Model plaza"],
  ["status", "服务状态页", "Service status"],
  ["smart_entry", "智能路由入口（myapis-auto）", "Smart entry (myapis-auto)"],
  ["video", "视频生成", "Video"],
  ["arabic", "阿拉伯文", "Arabic"],
];
export function AdminRegionProfile() {
  const [historyPage, setHistoryPage] = useState(1);
  const t = useAdminText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const query = useQuery({
    queryKey: ["deployment-profile", ...identity],
    queryFn: () =>
      apiGet<{ current: ProfileVersion; history: ProfileVersion[] }>(
        "/api/admin/region-profile",
      ),
    retry: false,
  });
  const [selected, setSelected] = useState<number | null>(null),
    [draft, setDraft] = useState<Profile | null>(null),
    [discard, setDiscard] = useState(false);
  const current = query.data?.current,
    shown = query.data?.history.find((v) => v.revision === selected) ?? current;
  const mutation = useMutation({
    retry: false,
    mutationFn: () =>
      apiPut("/api/admin/region-profile", {
        profile: draft,
        expected_revision: current!.revision,
      }),
    onSuccess: async () => {
      setDraft(null);
      setSelected(null);
      for (const key of [
        "deployment-profile",
        "admin-audit",
        "admin-models",
        "legal-versions",
      ])
        await cache.invalidateQueries({ queryKey: [key] });
    },
  });
  const patch = (value: Partial<Profile>) =>
    setDraft((p) => (p ? { ...p, ...value } : p));
  const close = () => {
    if (mutation.isPending) return;
    if (draft && JSON.stringify(draft) !== JSON.stringify(current?.profile))
      setDiscard(true);
    else setDraft(null);
  };
  return (
    <section className="adm-page">
      <header className="adm-heading">
        <div>
          <h1>{t(...messagePair("region_profile_cdebf4e938"))}</h1>
          <p>
            {t(
              ...messagePair(
                "deployment_branding_languages_timezone_and_fe_b6ac5795d7",
              ),
            )}
          </p>
        </div>
        <Button
          variant="secondary"
          disabled={!current}
          onClick={() => {
            mutation.reset();
            setDraft(structuredClone(current!.profile));
          }}
        >
          {t(...messagePair("edit_region_profile_2a5a958c4b"))}
        </Button>
      </header>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {shown && (
        <div className="adm-profile-grid">
          <div className="adm-page">
            <article className="adm-card">
              <div className="adm-heading">
                <h2>{t(...messagePair("basic_information_c3acdfaec2"))}</h2>
                <span className="adm-status">
                  v{shown.revision}
                  {shown.revision === current?.revision
                    ? ` · ${t(...messagePair("current_4e39c9b3d1"))}`
                    : ""}
                </span>
              </div>
              <dl className="adm-info">
                {[
                  [
                    t(...messagePair("region_id_ba4ad05ffd")),
                    shown.profile.region_id,
                  ],
                  [
                    t(...messagePair("brand_domain_86683df135")),
                    `${shown.profile.brand} · ${shown.profile.domain || "—"}`,
                  ],
                  [
                    t(...messagePair("default_available_languages_b4845ebf74")),
                    `${shown.profile.default_language} · ${shown.profile.languages.join(", ")}`,
                  ],
                  [
                    t(...messagePair("timezone_f4a3e16211")),
                    shown.profile.timezone,
                  ],
                  [
                    t(...messagePair("currency_7c690725a0")),
                    shown.profile.currency,
                  ],
                ].map(([name, value]) => (
                  <div key={name}>
                    <dt>{name}</dt>
                    <dd>
                      <bdi>{value}</bdi>
                    </dd>
                  </div>
                ))}
              </dl>
            </article>
            <article className="adm-card">
              <h2>{t(...messagePair("available_vendors_86fcc8bded"))}</h2>
              <p className="adm-meta">
                {t(
                  ...messagePair(
                    "resources_must_belong_to_this_region_and_an_a_e480019a1d",
                  ),
                )}
              </p>
              <div className="adm-model-availability">
                {shown.profile.allowed_vendors.map((v) => (
                  <span key={v}>{v}</span>
                ))}
              </div>
              {!shown.profile.allowed_vendors.length && (
                <p>
                  {t(
                    ...messagePair("no_vendor_allowlist_configured_9b7f3981b9"),
                  )}
                </p>
              )}
            </article>
            <article className="adm-card">
              <h2>
                {t(...messagePair("login_payments_and_legal_b607bf2158"))}
              </h2>
              <dl className="adm-info">
                <div>
                  <dt>{t(...messagePair("login_methods_eacd6ddf18"))}</dt>
                  <dd>{shown.profile.login_methods.join(", ")}</dd>
                </div>
                <div>
                  <dt>{t(...messagePair("payment_channels_5762b1f4ee"))}</dt>
                  <dd>{shown.profile.payment_channels.join(", ") || "—"}</dd>
                </div>
                <div>
                  <dt>{t(...messagePair("cookie_consent_ebf7e20b3c"))}</dt>
                  <dd>
                    {shown.profile.cookie_consent
                      ? t(...messagePair("shown_66b954bce0"))
                      : t(...messagePair("hidden_3cbbe9c8dd"))}
                  </dd>
                </div>
                <div>
                  <dt>
                    {t(...messagePair("request_content_retention_9e6b41e9f2"))}
                  </dt>
                  <dd>{t(...messagePair("disabled_504baa3564"))}</dd>
                </div>
              </dl>
            </article>
            <article className="adm-card">
              <h2>{t(...messagePair("feature_flags_5b39d03ecb"))}</h2>
              <div className="adm-checks">
                {featureNames.map(([key, zh, en]) => (
                  <label className="adm-check" key={key}>
                    <input
                      type="checkbox"
                      checked={shown.profile.features[key] ?? false}
                      disabled
                    />
                    {t(zh, en)}
                  </label>
                ))}
              </div>
            </article>
            <AdminLegalVersions />
          </div>
          <aside className="adm-card adm-history">
            <h2>{t(...messagePair("version_history_546a96987b"))}</h2>
            {current?.revision === 0 && (
              <p>
                {t(
                  ...messagePair(
                    "no_saved_profile_showing_deployment_defaults_08a7aa912a",
                  ),
                )}
              </p>
            )}
            {query.data?.history
              .slice((historyPage - 1) * 10, historyPage * 10)
              .map((v) => (
                <div key={v.revision}>
                  <button onClick={() => setSelected(v.revision)}>
                    v{v.revision}
                    {v.revision === current?.revision
                      ? ` · ${t(...messagePair("current_4e39c9b3d1"))}`
                      : ""}
                  </button>
                  <p>
                    {adminDate(v.created_at, v.profile.timezone)} ·{" "}
                    {v.profile.timezone} ·{" "}
                    {t(...messagePair("actor_965092921e"))} {v.actor_id}
                  </p>
                </div>
              ))}
            <ListPagination
              page={historyPage}
              size={10}
              total={query.data?.history.length ?? 0}
              onChange={setHistoryPage}
            />
            {selected !== null && (
              <Button variant="ghost" onClick={() => setSelected(null)}>
                {t(...messagePair("return_to_current_version_95315e1718"))}
              </Button>
            )}
          </aside>
        </div>
      )}
      <Drawer
        open={!!draft}
        onOpenChange={(open) => {
          if (!open) close();
        }}
        title={t(...messagePair("edit_region_profile_2a5a958c4b"))}
        description={t(
          ...messagePair(
            "only_root_can_edit_saving_creates_a_version_a_d07d18e5c3",
          ),
        )}
        preventOutsideClose
        footer={
          <div className="adm-actions">
            <Button disabled={mutation.isPending} onClick={close}>
              {t(...messagePair("cancel_82ac496be8"))}
            </Button>
            <Button
              variant="primary"
              disabled={mutation.isPending}
              onClick={() => mutation.mutate()}
            >
              {t(...messagePair("save_new_version_74fb5cc58f"))}
            </Button>
          </div>
        }
      >
        {draft && (
          <div className="adm-drawer-content">
            <div className="adm-form">
              {(
                [
                  ["region_id", t(...messagePair("region_id_ba4ad05ffd"))],
                  ["brand", t(...messagePair("brand_name_1d22dacc7d"))],
                  ["domain", t(...messagePair("domain_c1c7ba2ffa"))],
                  ["logo_url", t(...messagePair("logo_url_ee56a30e58"))],
                  ["timezone", t(...messagePair("iana_timezone_dbd3e05fa0"))],
                ] as const
              ).map(([field, label]) => (
                <label key={field}>
                  {label}
                  <Input
                    value={draft[field]}
                    onChange={(e) => patch({ [field]: e.target.value })}
                  />
                </label>
              ))}
              <label>
                {t(...messagePair("billing_currency_a46f635e0c"))}
                <select
                  value={draft.currency}
                  onChange={(e) => patch({ currency: e.target.value })}
                >
                  <option value="USD">USD</option>
                </select>
              </label>
              <label>
                {t(...messagePair("default_language_4027345b36"))}
                <select
                  value={draft.default_language}
                  onChange={(e) => patch({ default_language: e.target.value })}
                >
                  {draft.languages.map((lang) => (
                    <option key={lang} value={lang}>
                      {lang}
                    </option>
                  ))}
                </select>
              </label>
              <div>
                <p>{t(...messagePair("available_languages_8f64391277"))}</p>
                <div className="adm-checks">
                  {["en", "zh", "ar"].map((lang) => (
                    <label className="adm-check" key={lang}>
                      <input
                        type="checkbox"
                        checked={draft.languages.includes(lang)}
                        onChange={(e) =>
                          patch({
                            languages: e.target.checked
                              ? [...draft.languages, lang]
                              : draft.languages.filter((v) => v !== lang),
                          })
                        }
                      />
                      {lang}
                    </label>
                  ))}
                </div>
              </div>
              <label className="adm-wide">
                {t(
                  ...messagePair("allowed_vendors_one_id_per_line_26363483ef"),
                )}
                <textarea
                  rows={4}
                  defaultValue={draft.allowed_vendors.join("\n")}
                  onChange={(e) =>
                    patch({
                      allowed_vendors: e.target.value
                        .split("\n")
                        .map((v) => v.trim())
                        .filter(Boolean),
                    })
                  }
                />
              </label>
              <label className="adm-wide">
                {t(
                  ...messagePair("payment_channels_comma_separated_21bad5bcc5"),
                )}
                <Input
                  defaultValue={draft.payment_channels.join(",")}
                  onChange={(e) =>
                    patch({
                      payment_channels: e.target.value
                        .split(",")
                        .map((v) => v.trim())
                        .filter(Boolean),
                    })
                  }
                />
                <span className="adm-meta">
                  {t(
                    ...messagePair(
                      "the_current_card_checkout_is_simulated_enabli_8a284e3dde",
                    ),
                  )}
                </span>
              </label>
              <label className="adm-check">
                <input
                  type="checkbox"
                  checked={draft.cookie_consent}
                  onChange={(e) => patch({ cookie_consent: e.target.checked })}
                />
                {t(...messagePair("show_cookie_consent_0582aa2432"))}
              </label>
              <label>
                {t(...messagePair("cookie_display_mode"))}
                <select
                  value={draft.cookie_consent_mode || "banner"}
                  onChange={(e) =>
                    patch({
                      cookie_consent_mode: e.target.value as
                        "banner" | "dialog",
                    })
                  }
                >
                  <option value="banner">
                    {t(...messagePair("cookie_banner_mode"))}
                  </option>
                  <option value="dialog">
                    {t(...messagePair("cookie_dialog_mode"))}
                  </option>
                </select>
              </label>
              <div className="adm-wide">
                <h2>{t(...messagePair("feature_flags_5b39d03ecb"))}</h2>
                <div className="adm-checks">
                  {featureNames.map(([key, zh, en]) => (
                    <label className="adm-check" key={key}>
                      <input
                        type="checkbox"
                        checked={draft.features[key] ?? false}
                        onChange={(e) =>
                          patch({
                            features: {
                              ...draft.features,
                              [key]: e.target.checked,
                            },
                          })
                        }
                      />
                      {t(zh, en)}
                    </label>
                  ))}
                </div>
              </div>
              <p className="adm-wide adm-meta">
                {t(
                  ...messagePair(
                    "timezone_changes_preserve_active_key_quota_pe_3bf6ddb9ef",
                  ),
                )}
              </p>
            </div>
            <AdminFeedback error={mutation.error} />
          </div>
        )}
      </Drawer>
      <ConfirmDialog
        open={discard}
        onOpenChange={setDiscard}
        onConfirm={() => {
          setDiscard(false);
          setDraft(null);
        }}
        title={t(...messagePair("discard_unsaved_changes_1ef0512c77"))}
        description={t(
          ...messagePair("the_current_draft_will_not_be_saved_e11f5f1b86"),
        )}
      />
    </section>
  );
}

export function AdminLegalVersions() {
  const [page, setPage] = useState(1),
    [filterKind, setFilterKind] = useState(""),
    [filterLanguage, setFilterLanguage] = useState("");
  const t = useAdminText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const query = useQuery({
    queryKey: ["legal-versions", ...identity, page, filterKind, filterLanguage],
    queryFn: () =>
      apiGet<{
        items: LegalVersion[];
        timezone: string;
        now: number;
        total: number;
      }>(
        `/api/admin/legal?page=${page}&page_size=20&kind=${filterKind}&language=${filterLanguage}`,
      ),
    retry: false,
    refetchInterval: 30000,
    refetchOnWindowFocus: true,
  });
  useEffect(() => {
    if (query.data && page > Math.max(1, Math.ceil(query.data.total / 20)))
      setPage(Math.max(1, Math.ceil(query.data.total / 20)));
  }, [query.data, page]);
  const [open, setOpen] = useState(false),
    [preview, setPreview] = useState<LegalVersion | null>(null);
  const [kind, setKind] = useState("terms"),
    [language, setLanguage] = useState("en"),
    [version, setVersion] = useState(""),
    [content, setContent] = useState(""),
    [effective, setEffective] = useState(""),
    [fileError, setFileError] = useState<unknown>(null);
  const mutation = useMutation({
    retry: false,
    mutationFn: () =>
      apiPost("/api/admin/legal", {
        kind,
        language,
        version,
        content,
        effective_local: effective,
      }),
    onSuccess: async () => {
      setPage(1);
      setOpen(false);
      setContent("");
      await cache.invalidateQueries({ queryKey: ["legal-versions"] });
      await cache.invalidateQueries({ queryKey: ["admin-audit"] });
    },
  });
  const kinds: Record<string, string> = {
    terms: t(...messagePair("terms_fee31aef93")),
    privacy: t(...messagePair("privacy_ed4547df9b")),
    cookies: t(...messagePair("cookie_policy_64e22dd741")),
    refund: t(...messagePair("refund_policy_2c30de8020")),
  };
  return (
    <article className="adm-card adm-page">
      <header className="adm-heading">
        <div>
          <h2>{t(...messagePair("legal_document_versions_060390dd7b"))}</h2>
          <p>
            {t(
              ...messagePair(
                "schedule_effective_times_in_the_regional_time_8aac698b66",
              ),
            )}
          </p>
        </div>
        <Button
          variant="secondary"
          disabled={!query.data}
          onClick={() => {
            setVersion("");
            setContent("");
            setEffective("");
            setFileError(null);
            mutation.reset();
            setOpen(true);
          }}
        >
          {t(...messagePair("upload_new_version_3f17bda7b0"))}
        </Button>
      </header>
      <div className="adm-filters legal-history-filters">
        <label>
          {t(...messagePair("document_type_3d060d8b6b"))}
          <select
            value={filterKind}
            onChange={(e) => {
              setFilterKind(e.target.value);
              setPage(1);
            }}
          >
            <option value="">{t(...messagePair("legal_all_documents"))}</option>
            {Object.entries(kinds).map(([id, label]) => (
              <option key={id} value={id}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t(...messagePair("language_8d96f89c3d"))}
          <select
            value={filterLanguage}
            onChange={(e) => {
              setFilterLanguage(e.target.value);
              setPage(1);
            }}
          >
            <option value="">{t(...messagePair("legal_all_languages"))}</option>
            {["en", "zh", "ar"].map((lang) => (
              <option key={lang} value={lang}>
                {lang}
              </option>
            ))}
          </select>
        </label>
      </div>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {query.data && (
        <>
          <AdminTable
            label={t(...messagePair("legal_version_list_de815ee63f"))}
          >
            <thead>
              <tr>
                {[
                  t(...messagePair("document_5f25330be8")),
                  t(...messagePair("language_8d96f89c3d")),
                  t(...messagePair("version_85d3816499")),
                  t(...messagePair("effective_time_1585702a72")),
                  t(...messagePair("status_05fdd567d6")),
                ].map((x) => (
                  <th key={x}>{x}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data.items.map((d) => (
                <tr key={d.id}>
                  <td>
                    <button className="adm-link" onClick={() => setPreview(d)}>
                      {kinds[d.kind] ?? d.kind}
                    </button>
                  </td>
                  <td>{d.language}</td>
                  <td>{d.version}</td>
                  <td>
                    {adminDate(d.effective_at, d.timezone)}
                    <small className="adm-meta">{d.timezone}</small>
                  </td>
                  <td>
                    {d.status === "scheduled"
                      ? t(...messagePair("scheduled_ad1d12b5ae"))
                      : d.status === "historical"
                        ? t(...messagePair("historical_510b42ac0c"))
                        : t(...messagePair("current_4e39c9b3d1"))}
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
          <AdminPagination
            page={page}
            total={query.data.total}
            onChange={setPage}
          />
          {query.data.items.length === 0 && (
            <p>{t(...messagePair("no_legal_versions_uploaded_e0c6a4b6e7"))}</p>
          )}
        </>
      )}
      <Drawer
        open={open}
        onOpenChange={(value) => {
          if (!mutation.isPending) setOpen(value);
        }}
        title={t(...messagePair("upload_legal_version_8d0e293660"))}
        description={`${t(...messagePair("effective_timezone_d6f7b39234"))}: ${query.data?.timezone ?? ""}`}
        preventOutsideClose
        footer={
          <div className="adm-actions">
            <Button
              onClick={() => setOpen(false)}
              disabled={mutation.isPending}
            >
              {t(...messagePair("cancel_82ac496be8"))}
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="legal-version-form"
              disabled={mutation.isPending}
            >
              {t(...messagePair("save_version_82a24f8202"))}
            </Button>
          </div>
        }
      >
        <form
          className="adm-form"
          id="legal-version-form"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          <label>
            {t(...messagePair("document_type_3d060d8b6b"))}
            <select
              aria-label={t(...messagePair("document_type_3d060d8b6b"))}
              value={kind}
              onChange={(e) => setKind(e.target.value)}
            >
              {Object.entries(kinds).map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t(...messagePair("language_8d96f89c3d"))}
            <select
              aria-label={t(...messagePair("language_8d96f89c3d"))}
              value={language}
              onChange={(e) => setLanguage(e.target.value)}
            >
              {["en", "zh", "ar"].map((lang) => (
                <option key={lang} value={lang}>
                  {lang}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t(...messagePair("version_id_998cc5d7b0"))}
            <Input
              required
              maxLength={64}
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            />
          </label>
          <label>
            {t(...messagePair("effective_time_regional_timezone_6584e96f08"))}
            <Input
              type="datetime-local"
              required
              value={effective}
              onChange={(e) => setEffective(e.target.value)}
            />
          </label>
          <label className="adm-wide">
            {t(...messagePair("upload_text_file_txt_markdown_8e8d31adac"))}
            <Input
              type="file"
              accept=".txt,.md,text/plain,text/markdown"
              onChange={async (e) => {
                const file = e.target.files?.[0];
                if (!file) return;
                if (file.size > 512 * 1024) {
                  setFileError(Error("Maximum file size: 512 KiB"));
                  return;
                }
                setContent(await file.text());
                setFileError(null);
              }}
            />
          </label>
          <label className="adm-wide">
            {t(...messagePair("document_content_e421d03621"))}
            <textarea
              rows={14}
              required
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
          </label>
          <div className="adm-wide">
            <AdminFeedback error={mutation.error ?? fileError} />
          </div>
        </form>
      </Drawer>
      <Drawer
        open={!!preview}
        onOpenChange={(value) => {
          if (!value) setPreview(null);
        }}
        title={`${preview ? kinds[preview.kind] : ""} ${preview?.version ?? ""}`}
        description={
          preview
            ? `${preview.language} · ${adminDate(preview.effective_at, preview.timezone)}`
            : ""
        }
      >
        <article className="adm-legal-content">{preview?.content}</article>
      </Drawer>
    </article>
  );
}
