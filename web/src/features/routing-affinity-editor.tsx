import { useEffect, useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import { useUpdateOption } from "../lib/api/option";
import { readAffinityDraft } from "../lib/api/routing-affinity";
import { useSettingsText } from "./settings-text";

type Kind = "global" | "rules";
type Field =
  | "enabled"
  | "mode"
  | "ttl_seconds"
  | "cooldown_seconds"
  | "prefix_cache_enabled";
type Source = { type: "request_header" | "gjson"; key?: string; path?: string };
type Rule = {
  name: string;
  model_regex: string[];
  path_regex?: string[];
  user_agent_include?: string[];
  value_regex?: string;
  key_sources: Source[];
  ttl_seconds?: number;
  session_mode?: string;
  skip_retry_on_failure?: boolean;
};
type Draft = Awaited<ReturnType<typeof readAffinityDraft>>;
type Text = ReturnType<typeof useSettingsText>["text"];
const fields: Record<Field, [string, string, string]> = {
  enabled: ["亲和总开关", "Affinity enabled", "تفعيل التقارب"],
  mode: ["选道模式", "Routing mode", "وضع التوجيه"],
  ttl_seconds: [
    "默认有效期（秒）",
    "Default TTL (seconds)",
    "مدة الصلاحية الافتراضية (ثوانٍ)",
  ],
  cooldown_seconds: [
    "失败冷却（秒）",
    "Failure cooldown (seconds)",
    "مهلة الفشل (ثوانٍ)",
  ],
  prefix_cache_enabled: [
    "提示词前缀亲和",
    "Prompt-prefix affinity",
    "تقارب بادئة الطلب",
  ],
};
const title = (kind: Kind, t: Text) =>
  kind === "global"
    ? t("编辑全局配置", "Edit global configuration", "تحرير الإعدادات العامة")
    : t("管理亲和规则", "Manage affinity rules", "إدارة قواعد التقارب");

export function AffinityEditorActions() {
  const { text } = useSettingsText();
  const [kind, setKind] = useState<Kind>();
  const [notice, setNotice] = useState(false);
  return (
    <>
      {(["global", "rules"] as const).map((k) => (
        <Button
          key={k}
          onClick={() => {
            setNotice(false);
            setKind(k);
          }}
        >
          {title(k, text)}
        </Button>
      ))}
      {kind && (
        <AffinityEditor
          kind={kind}
          close={() => setKind(undefined)}
          saved={() => {
            setKind(undefined);
            setNotice(true);
          }}
        />
      )}
      {notice && (
        <span role="status">
          {text(
            "亲和配置已保存。",
            "Affinity configuration saved.",
            "حُفظت إعدادات التقارب.",
          )}
        </span>
      )}
    </>
  );
}

function AffinityEditor({
  kind,
  close,
  saved,
}: {
  kind: Kind;
  close: () => void;
  saved: () => void;
}) {
  const { text } = useSettingsText();
  const update = useUpdateOption();
  const [draft, setDraft] = useState<Draft>();
  const [rules, setRules] = useState<Rule[]>([]);
  const [field, setField] = useState<Field>("enabled");
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [blocked, setBlocked] = useState(false);
  useEffect(() => {
    const c = new AbortController();
    setDraft(undefined);
    setError("");
    setBlocked(false);
    readAffinityDraft(c.signal)
      .then((d) => {
        if (c.signal.aborted) return;
        if (kind === "rules") {
          if (d.values.rules === undefined) throw Error();
          const parsed = JSON.parse(d.values.rules) ?? [];
          if (!Array.isArray(parsed)) throw Error();
          setRules(parsed);
        } else {
          if (Object.keys(fields).some((k) => d.values[k] === undefined))
            throw Error();
          setField("enabled");
          setValue(d.values.enabled);
        }
        setDraft(d);
      })
      .catch(() => {
        if (!c.signal.aborted) setError("load");
      });
    return () => c.abort();
  }, [kind, reload]);
  const save = async () => {
    if (!draft || blocked || update.isPending) return;
    const key = kind === "rules" ? "rules" : field;
    try {
      await update.mutateAsync({
        key: "routing_affinity_" + key,
        value: kind === "rules" ? JSON.stringify(rules) : value,
        expected_version: draft.version,
      });
      saved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 400) {
        setError(e.message);
      } else {
        setBlocked(true);
        setError(
          e instanceof ApiError && e.status === 409 ? "conflict" : "unknown",
        );
      }
    }
  };
  const isNumber = field === "ttl_seconds" || field === "cooldown_seconds";
  const valid =
    kind === "rules"
      ? rules.every(
          (r) =>
            r.name.trim() &&
            r.model_regex?.some((s) => s.trim()) &&
            r.key_sources?.length &&
            r.key_sources.every((s) =>
              (s.type === "gjson" ? s.path : s.key)?.trim(),
            ),
        )
      : !isNumber ||
        (/^\d+$/.test(value) &&
          Number(value) > 0 &&
          Number(value) <= (field === "ttl_seconds" ? 86400 : 600));
  return (
    <Modal
      open
      title={title(kind, text)}
      description={text(
        "影响平台 HTTP 请求选道，不修改持久绑定授权。哈希模式不执行会话规则。",
        "Applies to platform HTTP routing, not binding grants. Hash mode does not evaluate session rules.",
        "تسري على توجيه HTTP للمنصة ولا تغيّر صلاحيات الروابط. لا يقيّم وضع التجزئة قواعد الجلسة.",
      )}
      size={kind === "rules" ? "wide" : "compact"}
      preventOutsideClose
      onOpenChange={(open) => {
        if (!open && !update.isPending) close();
      }}
      footer={
        <>
          <Button onClick={close} disabled={update.isPending}>
            {text("取消", "Cancel", "إلغاء")}
          </Button>
          <Button
            variant="primary"
            disabled={
              !draft ||
              !valid ||
              blocked ||
              update.isPending ||
              (kind === "global" && value === draft.values[field])
            }
            onClick={() => void save()}
          >
            {text("保存配置", "Save configuration", "حفظ الإعدادات")}
          </Button>
        </>
      }
    >
      <div className="ri-editor">
        {error && (
          <p role="alert">
            {error === "conflict"
              ? text(
                  "配置版本已变化，草稿未覆盖服务器。重新载入将放弃当前草稿。",
                  "Configuration changed. Your draft did not overwrite it. Reload to discard the draft.",
                  "تغيّر إصدار الإعدادات. أعد التحميل لتجاهل المسودة.",
                )
              : error === "load"
                ? text(
                    "无法读取配置。",
                    "Could not read configuration.",
                    "تعذرت قراءة الإعدادات.",
                  )
                : error === "unknown"
                  ? text(
                      "未确认保存结果，请重新读取。",
                      "Save outcome unconfirmed. Reload to verify.",
                      "تعذر تأكيد الحفظ. أعد التحميل للتحقق.",
                    )
                  : error}
          </p>
        )}
        {(blocked || error === "load") && (
          <Button onClick={() => setReload((v) => v + 1)}>
            {text(
              "重新载入配置",
              "Reload configuration",
              "إعادة تحميل الإعدادات",
            )}
          </Button>
        )}
        {!draft && !error && (
          <p role="status">
            {text(
              "正在读取配置…",
              "Loading configuration…",
              "جارٍ تحميل الإعدادات…",
            )}
          </p>
        )}
        {draft &&
          (kind === "rules" ? (
            <RulesEditor rules={rules} setRules={setRules} text={text} />
          ) : (
            <>
              <label>
                {text(
                  "选择要修改的配置项",
                  "Setting to change",
                  "الإعداد المراد تعديله",
                )}
                <select
                  value={field}
                  onChange={(e) => {
                    const f = e.target.value as Field;
                    setField(f);
                    setValue(draft.values[f]);
                    setError("");
                  }}
                >
                  {(Object.keys(fields) as Field[]).map((f) => (
                    <option key={f} value={f}>
                      {text(...fields[f])}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {text(...fields[field])}
                {isNumber ? (
                  <input
                    type="number"
                    min={1}
                    max={field === "ttl_seconds" ? 86400 : 600}
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                  />
                ) : (
                  <select
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                  >
                    {(field === "mode"
                      ? [
                          [
                            "sticky",
                            text("会话粘滞", "Sticky", "تثبيت الجلسة"),
                          ],
                          ["hash", text("哈希分配", "Hash", "التجزئة")],
                        ]
                      : [
                          ["true", text("启用", "Enabled", "مفعّل")],
                          ["false", text("停用", "Disabled", "غير مفعّل")],
                        ]
                    ).map(([v, l]) => (
                      <option key={v} value={v}>
                        {l}
                      </option>
                    ))}
                  </select>
                )}
              </label>
            </>
          ))}
      </div>
    </Modal>
  );
}

function RulesEditor({
  rules,
  setRules,
  text,
}: {
  rules: Rule[];
  setRules: (r: Rule[]) => void;
  text: Text;
}) {
  const [index, setIndex] = useState(0);
  const r = rules[index];
  const change = (patch: Partial<Rule>) =>
    setRules(rules.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  const move = (offset: number) => {
    const next = [...rules];
    [next[index], next[index + offset]] = [next[index + offset], next[index]];
    setRules(next);
    setIndex(index + offset);
  };
  const add = () => {
    setRules([
      ...rules,
      {
        name: "",
        model_regex: [],
        key_sources: [{ type: "request_header", key: "" }],
        session_mode: "prefer",
      },
    ]);
    setIndex(rules.length);
  };
  return (
    <>
      <p>
        {text(
          "按列表顺序匹配；保存会一次替换整组规则。",
          "Rules match in order. Saving replaces the entire rule list.",
          "تُطابق القواعد بالترتيب. يستبدل الحفظ القائمة كاملة.",
        )}
      </p>
      {!!rules.length && (
        <label>
          {text("规则顺序", "Rule order", "ترتيب القواعد")}
          <select
            value={index}
            onChange={(e) => setIndex(Number(e.target.value))}
          >
            {rules.map((row, i) => (
              <option key={i} value={i}>
                {i + 1}. {row.name || "—"}
              </option>
            ))}
          </select>
        </label>
      )}
      <div className="ri-actions">
        <Button onClick={add} disabled={rules.length >= 64}>
          {text("新增规则", "Add rule", "إضافة قاعدة")}
        </Button>
        {r && (
          <>
            <Button disabled={!index} onClick={() => move(-1)}>
              {text("上移", "Move up", "نقل لأعلى")}
            </Button>
            <Button
              disabled={index + 1 === rules.length}
              onClick={() => move(1)}
            >
              {text("下移", "Move down", "نقل لأسفل")}
            </Button>
            <Button
              onClick={() => {
                setRules(rules.filter((_, i) => i !== index));
                setIndex(Math.max(0, index - 1));
              }}
            >
              {text("删除规则", "Remove rule", "حذف القاعدة")}
            </Button>
          </>
        )}
      </div>
      {r && (
        <>
          <label>
            {text("规则名称", "Rule name", "اسم القاعدة")}
            <input
              value={r.name}
              onChange={(e) => change({ name: e.target.value })}
            />
          </label>
          {(
            [
              [
                "model_regex",
                text(
                  "模型正则（每行一项）",
                  "Model patterns (one per line)",
                  "أنماط النموذج (واحد لكل سطر)",
                ),
              ],
              [
                "path_regex",
                text(
                  "路径正则（每行一项）",
                  "Path patterns (one per line)",
                  "أنماط المسار (واحد لكل سطر)",
                ),
              ],
              [
                "user_agent_include",
                text(
                  "用户代理包含（每行一项）",
                  "User-agent includes (one per line)",
                  "مطابقة وكيل المستخدم (واحد لكل سطر)",
                ),
              ],
            ] as const
          ).map(([k, label]) => (
            <label key={k}>
              {label}
              <textarea
                dir="ltr"
                style={{ minHeight: 64 }}
                rows={2}
                value={r[k]?.join("\n") ?? ""}
                onChange={(e) =>
                  change({
                    [k]: e.target.value ? e.target.value.split("\n") : [],
                  })
                }
              />
            </label>
          ))}
          <p>
            {text(
              "会话标识来源：按顺序取第一个非空值。",
              "Session key sources: first nonempty value wins.",
              "مصادر معرّف الجلسة: تُستخدم أول قيمة غير فارغة.",
            )}
          </p>
          {r.key_sources.map((s, i) => {
            const replace = (src: Source) =>
              change({
                key_sources: r.key_sources.map((v, j) => (j === i ? src : v)),
              });
            return (
              <div className="ri-editor" key={i}>
                <label>
                  {i + 1}. {text("来源类型", "Source type", "نوع المصدر")}
                  <select
                    value={s.type}
                    onChange={(e) =>
                      replace(
                        e.target.value === "gjson"
                          ? { type: "gjson", path: "" }
                          : { type: "request_header", key: "" },
                      )
                    }
                  >
                    <option value="request_header">
                      {text("请求头", "Request header", "ترويسة الطلب")}
                    </option>
                    <option value="gjson">
                      {text(
                        "JSON 字段路径",
                        "JSON field path",
                        "مسار حقل JSON",
                      )}
                    </option>
                  </select>
                </label>
                <label>
                  {s.type === "gjson"
                    ? text("字段路径", "Field path", "مسار الحقل")
                    : text("请求头名称", "Header name", "اسم الترويسة")}
                  <input
                    dir="ltr"
                    value={(s.type === "gjson" ? s.path : s.key) ?? ""}
                    onChange={(e) =>
                      replace(
                        s.type === "gjson"
                          ? { ...s, path: e.target.value }
                          : { ...s, key: e.target.value },
                      )
                    }
                  />
                </label>
                <div className="ri-actions">
                  <Button
                    disabled={!i}
                    onClick={() => {
                      const a = [...r.key_sources];
                      [a[i - 1], a[i]] = [a[i], a[i - 1]];
                      change({ key_sources: a });
                    }}
                  >
                    {text("来源上移", "Move source up", "نقل المصدر لأعلى")}
                  </Button>
                  <Button
                    disabled={r.key_sources.length === 1}
                    onClick={() =>
                      change({
                        key_sources: r.key_sources.filter((_, j) => j !== i),
                      })
                    }
                  >
                    {text("移除来源", "Remove source", "إزالة المصدر")}
                  </Button>
                </div>
              </div>
            );
          })}
          <Button
            onClick={() =>
              change({
                key_sources: [...r.key_sources, { type: "gjson", path: "" }],
              })
            }
          >
            {text("增加来源", "Add source", "إضافة مصدر")}
          </Button>
          <label>
            {text("值校验正则", "Value pattern", "نمط القيمة")}
            <input
              value={r.value_regex ?? ""}
              onChange={(e) => change({ value_regex: e.target.value })}
            />
          </label>
          <label>
            {text(
              "规则有效期（秒，0 使用默认值）",
              "Rule TTL (seconds, 0 uses default)",
              "مدة الصلاحية (ثوانٍ، 0 للقيمة الافتراضية)",
            )}
            <input
              type="number"
              min={0}
              max={86400}
              value={r.ttl_seconds ?? 0}
              onChange={(e) => change({ ttl_seconds: Number(e.target.value) })}
            />
          </label>
          <label>
            {text("会话模式", "Session mode", "وضع الجلسة")}
            <select
              value={r.session_mode || "inherit"}
              onChange={(e) => change({ session_mode: e.target.value })}
            >
              {[
                [
                  "inherit",
                  text("默认（优先）", "Default (prefer)", "افتراضي (تفضيل)"),
                ],
                ["off", text("关闭", "Off", "إيقاف")],
                [
                  "prefer",
                  text(
                    "优先，允许回退",
                    "Prefer, allow fallback",
                    "تفضيل مع بديل",
                  ),
                ],
                [
                  "strict",
                  text(
                    "严格，不切换渠道",
                    "Strict, no channel switch",
                    "صارم دون تبديل القناة",
                  ),
                ],
              ].map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
          </label>
          <label className="ri-checkbox">
            <input
              type="checkbox"
              checked={r.skip_retry_on_failure ?? false}
              onChange={(e) =>
                change({ skip_retry_on_failure: e.target.checked })
              }
            />
            {text(
              "已绑定渠道失败时停止重试",
              "Stop retries when the bound channel fails",
              "إيقاف المحاولات عند فشل القناة المرتبطة",
            )}
          </label>
        </>
      )}
    </>
  );
}
