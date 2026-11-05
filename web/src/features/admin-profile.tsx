import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Drawer, ConfirmDialog } from "../components/ui/overlay";
import { apiGet, apiPut, apiPost } from "../lib/api/http";
import { useSessionQueryIdentity } from "../lib/system-access";
import {
  AdminFeedback,
  AdminTable,
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
          <h1>{t("地区档案", "Region profile")}</h1>
          <p>
            {t(
              "部署地区的品牌、语言、时区与功能配置。每次保存生成可回看的新版本。",
              "Deployment branding, languages, timezone and feature settings. Each save creates a reviewable version.",
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
          {t("编辑地区档案", "Edit region profile")}
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
                <h2>{t("基本信息", "Basic information")}</h2>
                <span className="adm-status">
                  v{shown.revision}
                  {shown.revision === current?.revision
                    ? ` · ${t("当前", "Current")}`
                    : ""}
                </span>
              </div>
              <dl className="adm-info">
                {[
                  [t("地区标识", "Region ID"), shown.profile.region_id],
                  [
                    t("品牌名 / 域名", "Brand / domain"),
                    `${shown.profile.brand} · ${shown.profile.domain || "—"}`,
                  ],
                  [
                    t("默认 / 可选语言", "Default / available languages"),
                    `${shown.profile.default_language} · ${shown.profile.languages.join(", ")}`,
                  ],
                  [t("时区", "Timezone"), shown.profile.timezone],
                  [t("币种", "Currency"), shown.profile.currency],
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
              <h2>{t("可售厂商", "Available vendors")}</h2>
              <p className="adm-meta">
                {t(
                  "资源需属于本地区，厂商也需在允许列表中，模型才能上架。",
                  "Resources must belong to this region and an allowed vendor before listing.",
                )}
              </p>
              <div className="adm-model-availability">
                {shown.profile.allowed_vendors.map((v) => (
                  <span key={v}>{v}</span>
                ))}
              </div>
              {!shown.profile.allowed_vendors.length && (
                <p>
                  {t("尚未配置可售厂商。", "No vendor allowlist configured.")}
                </p>
              )}
            </article>
            <article className="adm-card">
              <h2>{t("登录、支付与法律", "Login, payments and legal")}</h2>
              <dl className="adm-info">
                <div>
                  <dt>{t("登录方式", "Login methods")}</dt>
                  <dd>{shown.profile.login_methods.join(", ")}</dd>
                </div>
                <div>
                  <dt>{t("支付渠道", "Payment channels")}</dt>
                  <dd>{shown.profile.payment_channels.join(", ") || "—"}</dd>
                </div>
                <div>
                  <dt>{t("Cookie 同意", "Cookie consent")}</dt>
                  <dd>
                    {shown.profile.cookie_consent
                      ? t("展示", "Shown")
                      : t("不展示", "Hidden")}
                  </dd>
                </div>
                <div>
                  <dt>{t("请求内容存储", "Request-content retention")}</dt>
                  <dd>{t("禁止开启", "Disabled")}</dd>
                </div>
              </dl>
            </article>
            <article className="adm-card">
              <h2>{t("功能开关", "Feature flags")}</h2>
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
            <h2>{t("版本历史", "Version history")}</h2>
            {current?.revision === 0 && (
              <p>
                {t(
                  "尚未保存地区档案，当前显示部署默认值。",
                  "No saved profile; showing deployment defaults.",
                )}
              </p>
            )}
            {query.data?.history.map((v) => (
              <div key={v.revision}>
                <button onClick={() => setSelected(v.revision)}>
                  v{v.revision}
                  {v.revision === current?.revision
                    ? ` · ${t("当前", "Current")}`
                    : ""}
                </button>
                <p>
                  {adminDate(v.created_at, v.profile.timezone)} ·{" "}
                  {t("操作人", "Actor")} {v.actor_id}
                </p>
              </div>
            ))}
            {selected !== null && (
              <Button variant="ghost" onClick={() => setSelected(null)}>
                {t("返回当前版本", "Return to current version")}
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
        title={t("编辑地区档案", "Edit region profile")}
        description={t(
          "仅超级管理员可编辑，保存生成新版本并写入审计。",
          "Only Root can edit. Saving creates a version and audit record.",
        )}
        preventOutsideClose
        footer={
          <div className="adm-actions">
            <Button disabled={mutation.isPending} onClick={close}>
              {t("取消", "Cancel")}
            </Button>
            <Button
              variant="primary"
              disabled={mutation.isPending}
              onClick={() => mutation.mutate()}
            >
              {t("保存新版本", "Save new version")}
            </Button>
          </div>
        }
      >
        {draft && (
          <div className="adm-drawer-content">
            <div className="adm-form">
              {(
                [
                  ["region_id", t("地区标识", "Region ID")],
                  ["brand", t("品牌名", "Brand name")],
                  ["domain", t("域名", "Domain")],
                  ["logo_url", t("Logo 地址", "Logo URL")],
                  ["timezone", t("IANA 时区", "IANA timezone")],
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
                {t("计价币种", "Billing currency")}
                <select
                  value={draft.currency}
                  onChange={(e) => patch({ currency: e.target.value })}
                >
                  <option value="USD">USD</option>
                </select>
              </label>
              <label>
                {t("默认语言", "Default language")}
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
                <p>{t("可选语言", "Available languages")}</p>
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
                  "可售厂商（每行一个标识）",
                  "Allowed vendors (one ID per line)",
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
                  "支付渠道（逗号分隔）",
                  "Payment channels (comma separated)",
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
                    "当前银行卡收银台为模拟渠道；启用渠道本身不代表支付已到账。",
                    "The current card checkout is simulated. Enabling a channel does not imply payment confirmation.",
                  )}
                </span>
              </label>
              <label className="adm-check">
                <input
                  type="checkbox"
                  checked={draft.cookie_consent}
                  onChange={(e) => patch({ cookie_consent: e.target.checked })}
                />
                {t("展示 Cookie 同意", "Show Cookie consent")}
              </label>
              <div className="adm-wide">
                <h2>{t("功能开关", "Feature flags")}</h2>
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
                  "时区变更不重置已开始的 Key 额度周期；后续周期按新时区。请求内容存储不可开启。",
                  "Timezone changes preserve active Key quota periods; future periods use the new timezone. Request-content retention cannot be enabled.",
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
        title={t("放弃未保存修改？", "Discard unsaved changes?")}
        description={t(
          "当前草稿不会保存。",
          "The current draft will not be saved.",
        )}
      />
    </section>
  );
}

export function AdminLegalVersions() {
  const t = useAdminText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const query = useQuery({
    queryKey: ["legal-versions", ...identity],
    queryFn: () =>
      apiGet<{ items: LegalVersion[]; timezone: string; now: number }>(
        "/api/admin/legal",
      ),
    retry: false,
  });
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
      setOpen(false);
      setContent("");
      await cache.invalidateQueries({ queryKey: ["legal-versions"] });
      await cache.invalidateQueries({ queryKey: ["admin-audit"] });
    },
  });
  const kinds: Record<string, string> = {
    terms: t("服务条款", "Terms"),
    privacy: t("隐私政策", "Privacy"),
    cookies: t("Cookie 政策", "Cookie policy"),
    refund: t("退款政策", "Refund policy"),
  };
  return (
    <article className="adm-card adm-page">
      <header className="adm-heading">
        <div>
          <h2>{t("法律文本版本", "Legal document versions")}</h2>
          <p>
            {t(
              "按地区时区设定生效时间，公开页面到时自动选用新版本。",
              "Schedule effective times in the regional timezone; public pages select the new version when due.",
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
          {t("上传新版本", "Upload new version")}
        </Button>
      </header>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {query.data && (
        <>
          <AdminTable label={t("法律版本列表", "Legal version list")}>
            <thead>
              <tr>
                {[
                  t("文档", "Document"),
                  t("语言", "Language"),
                  t("版本", "Version"),
                  t("生效时间", "Effective time"),
                  t("状态", "Status"),
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
                    {d.effective_at > query.data!.now
                      ? t("待生效", "Scheduled")
                      : query.data!.items.some(
                            (other) =>
                              other.kind === d.kind &&
                              other.language === d.language &&
                              other.effective_at <= query.data!.now &&
                              other.effective_at > d.effective_at,
                          )
                        ? t("历史", "Historical")
                        : t("当前", "Current")}
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
          {query.data.items.length === 0 && (
            <p>{t("尚未上传法律文本版本。", "No legal versions uploaded.")}</p>
          )}
        </>
      )}
      <Drawer
        open={open}
        onOpenChange={(value) => {
          if (!mutation.isPending) setOpen(value);
        }}
        title={t("上传法律文本新版本", "Upload legal version")}
        description={`${t("生效时区", "Effective timezone")}: ${query.data?.timezone ?? ""}`}
        preventOutsideClose
        footer={
          <div className="adm-actions">
            <Button
              onClick={() => setOpen(false)}
              disabled={mutation.isPending}
            >
              {t("取消", "Cancel")}
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="legal-version-form"
              disabled={mutation.isPending}
            >
              {t("保存版本", "Save version")}
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
            {t("文档类型", "Document type")}
            <select
              aria-label={t("文档类型", "Document type")}
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
            {t("语言", "Language")}
            <select
              aria-label={t("语言", "Language")}
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
            {t("版本标识", "Version ID")}
            <Input
              required
              maxLength={64}
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            />
          </label>
          <label>
            {t("生效时间（地区时区）", "Effective time (regional timezone)")}
            <Input
              type="datetime-local"
              required
              value={effective}
              onChange={(e) => setEffective(e.target.value)}
            />
          </label>
          <label className="adm-wide">
            {t(
              "上传文本文件（TXT / Markdown）",
              "Upload text file (TXT / Markdown)",
            )}
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
            {t("正文", "Document content")}
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
