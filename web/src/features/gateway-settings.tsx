import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Settings, ShieldCheck } from "lucide-react";
import { Button } from "../components/ui/button";
import { ApiError, apiGet, apiPut } from "../lib/api/http";
import {
  type Option,
  type OptionChange,
  useOptionStatus,
} from "../lib/api/option";
import {
  ROLE_ROOT,
  useSystemAccess,
  useSessionQueryIdentity,
} from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import { ConfigurationSync } from "./configuration-sync";
import "./gateway-settings.css";
import { UpstreamProtectionSettings } from "./upstream-protection";
import { ChannelTrafficSettings } from "./channel-traffic";

const keys = [
  "ip_rate_limit",
  "user_rate_limit",
  "token_rate_limit",
  "model_rate_limit",
] as const;
type LimitKey = (typeof keys)[number];
type Values = Record<LimitKey, string>;
type Snapshot = { version: number; options: Option[]; history: OptionChange[] };
type Draft = { version: number; values: Values; original: Values };

function readValues(data: Snapshot): Values {
  return Object.fromEntries(
    keys.map((key) => [
      key,
      data.options.find((o) => o.key === key)?.value ?? "",
    ]),
  ) as Values;
}

function validateSnapshot(data: Snapshot): Snapshot {
  if (
    !data ||
    !Number.isSafeInteger(data.version) ||
    data.version < 0 ||
    !Array.isArray(data.options) ||
    !Array.isArray(data.history)
  )
    throw new Error("Invalid gateway configuration response");
  for (const key of keys) {
    const options = data.options.filter((o) => o.key === key);
    if (
      options.length !== 1 ||
      typeof options[0].value !== "string" ||
      !/^\d+$/.test(options[0].value)
    )
      throw new Error("Incomplete gateway configuration");
    const value = Number(options[0].value);
    if (
      value > 1000000 ||
      ((key === "ip_rate_limit" || key === "user_rate_limit") && value < 1)
    )
      throw new Error("Invalid gateway limit");
  }
  return data;
}

export function GatewaySettings() {
  const role = useSystemAccess((state) => state.role);
  const identity = useSessionQueryIdentity();
  const { text } = useSettingsText();
  if (role !== ROLE_ROOT)
    return (
      <section className="gateway-settings gateway-panel">
        <ShieldCheck size={28} />
        <h1>
          {text(
            "需要超级管理员权限",
            "Root access required",
            "يلزم إذن المسؤول الأعلى",
          )}
        </h1>
        <p>
          {text(
            "请使用超级管理员账号访问网关配置。",
            "Sign in as root to manage gateway settings.",
            "سجّل الدخول كمسؤول أعلى لإدارة إعدادات البوابة.",
          )}
        </p>
        <Button asChild>
          <Link to="/console">
            {text("返回概览", "Back to overview", "العودة للنظرة العامة")}
          </Link>
        </Button>
      </section>
    );
  return <GatewaySettingsForm key={JSON.stringify(identity)} />;
}

function GatewaySettingsForm() {
  const { text, lang } = useSettingsText();
  const identity = useSessionQueryIdentity();
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["gateway-options", ...identity],
    queryFn: async ({ signal }) =>
      validateSnapshot(await apiGet<Snapshot>("/api/option/gateway", signal)),
    retry: false,
  });
  const sync = useOptionStatus(5000);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [notice, setNotice] = useState("");
  const [syncOpen, setSyncOpen] = useState(false);
  const [unlimitedPrevious, setUnlimitedPrevious] = useState<Partial<Values>>(
    {},
  );
  const data = query.data;
  const values = draft?.values ?? (data ? readValues(data) : null);
  const dirty =
    !!draft && keys.some((key) => draft.values[key] !== draft.original[key]);
  const conflict = !!draft && !!data && draft.version !== data.version;
  const labels: Record<LimitKey, string> = {
    ip_rate_limit: text("每个 IP", "Per IP address", "لكل عنوان IP"),
    user_rate_limit: text("每个用户", "Per user", "لكل مستخدم"),
    token_rate_limit: text("每个 API 密钥", "Per API key", "لكل مفتاح API"),
    model_rate_limit: text("每个模型", "Per model", "لكل نموذج"),
  };
  const descriptions: Record<LimitKey, string> = {
    ip_rate_limit: text(
      "同一来源 IP 的请求共享限额。",
      "Requests from the same IP share this limit.",
      "تتشارك طلبات عنوان IP نفسه هذا الحد.",
    ),
    user_rate_limit: text(
      "同一用户的多个 API 密钥共享限额。",
      "All API keys owned by one user share this limit.",
      "تتشارك مفاتيح المستخدم الواحد هذا الحد.",
    ),
    token_rate_limit: text(
      "同一密钥调用不同模型、渠道时共享限额。",
      "One key shares this limit across models and channels.",
      "يتشارك المفتاح هذا الحد عبر النماذج والقنوات.",
    ),
    model_rate_limit: text(
      "同一模型的请求跨用户共享限额。",
      "Requests for the same model share this limit across users.",
      "تتشارك طلبات النموذج نفسه هذا الحد عبر المستخدمين.",
    ),
  };
  const validValue = (key: LimitKey, value: string) =>
    /^\d+$/.test(value) &&
    Number(value) <= 1000000 &&
    Number(value) >=
      (key === "ip_rate_limit" || key === "user_rate_limit" ? 1 : 0);
  const valid = !!values && keys.every((key) => validValue(key, values[key]));
  const save = useMutation({
    mutationFn: (input: Draft) =>
      apiPut<{ version: number; applied: boolean }>("/api/option/gateway", {
        expected_version: input.version,
        options: keys.map((key) => ({ key, value: input.values[key] })),
      }),
    retry: false,
    onSuccess: async (result) => {
      setDraft(null);
      setUnlimitedPrevious({});
      setNotice(
        result.applied
          ? text(
              "配置已保存并加载，当前分钟的累计请求数保持不变。",
              "Settings saved and loaded. This minute’s request counts are unchanged.",
              "حُفظت الإعدادات وحُمّلت. لم تتغير أعداد طلبات الدقيقة الحالية.",
            )
          : text(
              "配置已保存，当前进程尚未成功加载。请查看配置同步详情。",
              "Settings saved, but this process has not loaded them. Check synchronization details.",
              "حُفظت الإعدادات لكن العملية لم تحمّلها. راجع تفاصيل المزامنة.",
            ),
      );
      if (!result.applied) setSyncOpen(true);
      await Promise.all([
        query.refetch(),
        ...["options", "option-status", "option-history"].map((key) =>
          client.invalidateQueries({ queryKey: [key] }),
        ),
      ]);
    },
  });
  useEffect(() => {
    if (!dirty) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [dirty]);
  const edit = (key: LimitKey, value: string) => {
    if (!data || !values) return;
    setNotice("");
    save.reset();
    setDraft((current) => {
      const base = current ?? {
        version: data.version,
        original: readValues(data),
        values: readValues(data),
      };
      return { ...base, values: { ...base.values, [key]: value } };
    });
  };
  const reset = async () => {
    const result = await query.refetch();
    if (result.isSuccess) {
      setDraft(null);
      setUnlimitedPrevious({});
      setNotice("");
      save.reset();
    }
  };
  const isConflict =
    conflict || (save.error instanceof ApiError && save.error.status === 409);
  const synchronized =
    !sync.isError &&
    sync.data?.in_sync &&
    data?.version === sync.data.database_version;
  const formatTime = (seconds: number) =>
    new Intl.DateTimeFormat(lang, {
      dateStyle: "medium",
      timeStyle: "medium",
    }).format(new Date(seconds * 1000));
  return (
    <div className="gateway-settings">
      <header className="gateway-heading">
        <div>
          <span className="gateway-eyebrow">
            <Settings size={15} />
            {text("系统设置", "System settings", "إعدادات النظام")}
          </span>
          <h1>{text("网关配置", "Gateway settings", "إعدادات البوابة")}</h1>
          <p>
            {text(
              "管理网关请求限额，保存后自动应用。仅超级管理员可修改。",
              "Manage gateway request limits. Changes apply on save. Root access only.",
              "إدارة حدود طلبات البوابة وتطبيقها عند الحفظ. للمسؤول الأعلى فقط.",
            )}
          </p>
        </div>
        <span className="gateway-sync-badge" data-synced={!!synchronized}>
          {synchronized
            ? text(
                "当前副本配置已同步",
                "Serving replica synchronized",
                "تمت مزامنة النسخة الحالية",
              )
            : text(
                "配置同步待确认",
                "Synchronization unconfirmed",
                "المزامنة غير مؤكدة",
              )}
        </span>
      </header>
      {notice && (
        <p className="gateway-notice" role="status">
          {notice}
        </p>
      )}
      {query.isPending && (
        <p role="status">
          {text(
            "正在读取网关配置…",
            "Loading gateway settings…",
            "جارٍ تحميل إعدادات البوابة…",
          )}
        </p>
      )}
      {query.isError && (
        <div className="gateway-error" role="alert">
          <p>
            {text(
              "无法读取网关配置，请确认后端已更新并重新登录或重试。",
              "Cannot read gateway settings. Ensure the backend is updated, then sign in again or retry.",
              "تعذر قراءة الإعدادات. تأكد من تحديث الخادم ثم أعد تسجيل الدخول أو المحاولة.",
            )}
          </p>
          <Button
            onClick={() => void query.refetch()}
            loading={query.isFetching}
          >
            {text("重新读取", "Retry", "إعادة المحاولة")}
          </Button>
        </div>
      )}
      {data && values && (
        <>
          <form
            className="gateway-panel"
            onSubmit={(event) => {
              event.preventDefault();
              if (
                draft &&
                dirty &&
                valid &&
                !isConflict &&
                !query.isError &&
                !save.isPending
              )
                save.mutate(draft);
            }}
          >
            <h2>{text("请求限流", "Request limits", "حدود الطلبات")}</h2>
            <p className="gateway-description">
              {text(
                "以下限额分别应用于每个网关副本。副本间同步配置，不共享请求计数。",
                "Limits apply separately to each gateway replica. Settings synchronize; request counts are independent.",
                "تطبق الحدود على كل نسخة بوابة بشكل مستقل. تتزامن الإعدادات ولا تتشارك أعداد الطلبات.",
              )}
            </p>
            <fieldset
              disabled={save.isPending || query.isError}
              className="gateway-fields"
            >
              <legend className="sr-only">
                {text(
                  "每分钟请求限额",
                  "Requests per minute",
                  "الطلبات في الدقيقة",
                )}
              </legend>
              {keys.map((key) => {
                const canDisable =
                  key === "token_rate_limit" || key === "model_rate_limit";
                const unlimited = canDisable && values[key] === "0";
                const invalid = !validValue(key, values[key]);
                return (
                  <div className="gateway-field" key={key}>
                    <div className="gateway-field-heading">
                      <label htmlFor={`gateway-${key}`}>{labels[key]}</label>
                      {canDisable && (
                        <label className="gateway-toggle">
                          <input
                            type="checkbox"
                            role="switch"
                            checked={unlimited}
                            aria-label={`${labels[key]} · ${text("不限制", "Unlimited", "بلا حدود")}`}
                            onChange={(event) => {
                              if (event.target.checked) {
                                setUnlimitedPrevious((current) => ({
                                  ...current,
                                  [key]: values[key],
                                }));
                                edit(key, "0");
                              } else
                                edit(
                                  key,
                                  unlimitedPrevious[key] &&
                                    Number(unlimitedPrevious[key]) > 0
                                    ? unlimitedPrevious[key]!
                                    : "",
                                );
                            }}
                          />
                          {text("不限制", "Unlimited", "بلا حدود")}
                        </label>
                      )}
                    </div>
                    <div className="gateway-number">
                      <input
                        id={`gateway-${key}`}
                        type="number"
                        inputMode="numeric"
                        min={canDisable ? 0 : 1}
                        max={1000000}
                        step={1}
                        required
                        disabled={unlimited}
                        value={unlimited ? "" : values[key]}
                        placeholder={
                          unlimited
                            ? text("不限制", "Unlimited", "بلا حدود")
                            : ""
                        }
                        aria-invalid={invalid}
                        aria-describedby={`gateway-${key}-help${invalid ? ` gateway-${key}-error` : ""}`}
                        onChange={(event) => edit(key, event.target.value)}
                      />
                      <span>
                        {text("次 / 分钟", "req / min", "طلب / دقيقة")}
                      </span>
                    </div>
                    <p id={`gateway-${key}-help`}>{descriptions[key]}</p>
                    {invalid && (
                      <p
                        className="gateway-error-text"
                        id={`gateway-${key}-error`}
                      >
                        {text(
                          `请输入 ${canDisable ? 0 : 1}～1000000 的整数。`,
                          `Enter an integer from ${canDisable ? 0 : 1} to 1000000.`,
                          `أدخل عددًا صحيحًا من ${canDisable ? 0 : 1} إلى 1000000.`,
                        )}
                      </p>
                    )}
                  </div>
                );
              })}
            </fieldset>
            <p className="gateway-rule">
              {text(
                "按整分钟重新计数，请求需依次通过各层限制。保存不会清空本分钟的累计请求数。",
                "Counts reset at each minute boundary. Requests must pass every applicable limit. Saving does not reset current counts.",
                "تُصفّر الأعداد عند بداية كل دقيقة ويجب اجتياز كل الحدود المطبقة. لا يؤدي الحفظ إلى تصفير الأعداد الحالية.",
              )}
            </p>
            {isConflict && (
              <p className="gateway-error" role="alert">
                {text(
                  "配置已有新版本，草稿已保留。请取消更改以读取最新配置后重新编辑。",
                  "Configuration changed. Your draft is retained. Cancel changes to load the latest settings before editing again.",
                  "تغيرت الإعدادات واحتُفظ بالمسودة. ألغِ التغييرات لتحميل أحدث إعدادات ثم عدّل مجددًا.",
                )}
              </p>
            )}
            {save.isError && !isConflict && (
              <p className="gateway-error" role="alert">
                {text(
                  "未能确认保存结果，草稿已保留。可取消更改并重新读取以确认当前配置。",
                  "Could not confirm the save result. Your draft is retained. Cancel changes and reload to check the current settings.",
                  "تعذر تأكيد الحفظ واحتُفظ بالمسودة. ألغِ التغييرات وأعد التحميل للتحقق من الإعدادات الحالية.",
                )}
              </p>
            )}
            <footer className="gateway-actions">
              <span>
                {dirty
                  ? text(
                      "有未保存的更改",
                      "Unsaved changes",
                      "تغييرات غير محفوظة",
                    )
                  : text(
                      "当前已保存的配置",
                      "Saved configuration",
                      "الإعدادات المحفوظة",
                    )}
              </span>
              <Button
                disabled={
                  save.isPending ||
                  query.isFetching ||
                  (!draft && !save.isError)
                }
                onClick={() => void reset()}
              >
                {text("取消更改", "Cancel changes", "إلغاء التغييرات")}
              </Button>
              <Button
                type="submit"
                variant="primary"
                loading={save.isPending}
                disabled={
                  !dirty ||
                  !valid ||
                  isConflict ||
                  query.isError ||
                  query.isFetching
                }
              >
                {text("保存配置", "Save settings", "حفظ الإعدادات")}
              </Button>
            </footer>
          </form>
          <ChannelTrafficSettings />
          <section className="gateway-panel">
            <h2>{text("最近变更", "Recent changes", "التغييرات الأخيرة")}</h2>
            <p className="gateway-description">
              {text(
                "仅展示最近 20 条限流变更，时间使用浏览器所在时区。",
                "Latest 20 limit changes. Times use your browser’s time zone.",
                "آخر 20 تغييرًا للحدود. الأوقات حسب المنطقة الزمنية للمتصفح.",
              )}
            </p>
            {data.history.length === 0 ? (
              <p className="gateway-empty">
                {text(
                  "暂无限流配置变更记录。",
                  "No limit configuration changes yet.",
                  "لا توجد تغييرات على الحدود بعد.",
                )}
              </p>
            ) : (
              <div
                className="gateway-history"
                tabIndex={0}
                role="region"
                aria-label={text(
                  "限流配置变更记录",
                  "Limit change history",
                  "سجل تغييرات الحدود",
                )}
              >
                <table>
                  <thead>
                    <tr>
                      {[
                        text("时间", "Time", "الوقت"),
                        text("配置项", "Setting", "الإعداد"),
                        text("变更", "Change", "التغيير"),
                        text("操作人 ID", "Actor ID", "معرّف المسؤول"),
                      ].map((label) => (
                        <th scope="col" key={label}>
                          {label}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {data.history.map((entry) => (
                      <tr key={entry.id}>
                        <td>{formatTime(entry.created_at)}</td>
                        <td>{labels[entry.key as LimitKey] ?? entry.key}</td>
                        <td dir="ltr">
                          {entry.old_value === "0"
                            ? text("不限制", "Unlimited", "بلا حدود")
                            : entry.old_value || "—"}{" "}
                          →{" "}
                          {entry.new_value === "0"
                            ? text("不限制", "Unlimited", "بلا حدود")
                            : entry.new_value}
                        </td>
                        <td>{entry.actor_id}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </>
      )}
      <details
        className="gateway-panel gateway-details"
        open={syncOpen}
        onToggle={(event) => setSyncOpen(event.currentTarget.open)}
      >
        <summary>
          {text(
            "配置同步详情",
            "Configuration synchronization details",
            "تفاصيل مزامنة الإعدادات",
          )}
        </summary>
        {syncOpen && <ConfigurationSync />}
      </details>
      <UpstreamProtectionSettings />
    </div>
  );
}
