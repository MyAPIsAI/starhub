import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { ConfirmDialog } from "../components/ui/overlay";
import { ApiError, apiGet, apiPost, apiPut } from "../lib/api/http";
import {
  ROLE_ROOT,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import "./gateway-settings.css";
import "./upstream-protection.css";

type Policy = { enabled: boolean; retry: boolean } & Record<
  string,
  number | boolean
>;
type Snapshot = {
  version: number;
  value: string;
  history: {
    id: number;
    created_at: number;
    actor_id: number;
    new_value: string;
  }[];
};
type Health = {
  version: number;
  id: string;
  channel_id: number;
  model: string;
  scope: string;
  capability: string;
  stream: boolean;
  failures: number;
  open_until: number;
  lease_until: number;
  reason: string;
  probe_result: string;
  updated_at: number;
};
type Event = {
  id: string;
  request_id: string;
  channel_id: number;
  model: string;
  kind: string;
  class: string;
  status: number;
  sent: boolean;
  retry: boolean;
  decision: string;
  open_until: number;
  created_at: number;
};
const fields = [
  [
    "max_attempts",
    "最多尝试次数（含首次）",
    "Maximum attempts (including first)",
    "الحد الأقصى للمحاولات",
    1,
    10,
  ],
  [
    "retry_window_seconds",
    "重试时间预算（秒）",
    "Retry window (seconds)",
    "مهلة إعادة المحاولة بالثواني",
    1,
    600,
  ],
  [
    "failure_threshold",
    "连续失败阈值",
    "Consecutive failure threshold",
    "حد الإخفاقات المتتالية",
    1,
    100,
  ],
  [
    "open_seconds",
    "首次熔断（秒）",
    "Initial open interval (seconds)",
    "مدة العزل الأولية بالثواني",
    1,
    3600,
  ],
  [
    "max_open_seconds",
    "最大故障退避（秒）",
    "Maximum failure backoff (seconds)",
    "أقصى تراجع للأعطال بالثواني",
    1,
    3600,
  ],
  [
    "rate_limit_seconds",
    "429 默认冷却（秒）",
    "Default 429 cooldown (seconds)",
    "تبريد 429 الافتراضي بالثواني",
    1,
    86400,
  ],
  [
    "isolation_seconds",
    "凭证与额度复查间隔（秒）",
    "Credential and quota recheck (seconds)",
    "فترة فحص الاعتماد والحصة بالثواني",
    60,
    86400,
  ],
  [
    "probe_concurrency",
    "探测并发上限",
    "Concurrent probe limit",
    "حد الفحوص المتزامنة",
    1,
    16,
  ],
  [
    "probe_timeout_seconds",
    "探测超时（秒）",
    "Probe timeout (seconds)",
    "مهلة الفحص بالثواني",
    1,
    120,
  ],
] as const;

function useProtectionLabels() {
  const { text } = useSettingsText();
  const labels: Record<string, string> = {
    manual_reset: text("人工解除", "Manually reset", "إلغاء يدوي"),
    resource_protected: text("资源保护中", "Resource protected", "المورد محمي"),
    not_sent: text(
      "跳过，未发起调用",
      "Skipped; not sent",
      "تم التخطي دون إرسال",
    ),
    success: text("成功", "Success", "نجاح"),
    failure: text("失败", "Failure", "فشل"),
    unknown: text("结果不确定", "Unknown", "غير معروف"),
    recovered: text("已恢复", "Recovered", "تم التعافي"),
    rate_limited: text("上游限流", "Upstream rate limit", "حد طلبات المنبع"),
    credential_rejected: text(
      "凭证被拒绝",
      "Credential rejected",
      "رفض الاعتماد",
    ),
    quota_exhausted: text(
      "上游额度不足",
      "Upstream quota exhausted",
      "نفاد حصة المنبع",
    ),
    model_unavailable: text(
      "模型不可用",
      "Model unavailable",
      "النموذج غير متاح",
    ),
    upstream_failure: text("上游故障", "Upstream failure", "عطل المنبع"),
    protocol_failure: text(
      "响应不完整",
      "Incomplete response",
      "استجابة غير مكتملة",
    ),
    transport_failure: text(
      "连接失败或超时",
      "Connection failure or timeout",
      "فشل الاتصال أو انتهاء المهلة",
    ),
    request_rejected: text("请求被拒绝", "Request rejected", "رفض الطلب"),
    local_error: text("本地错误", "Local error", "خطأ محلي"),
    client_disconnected: text(
      "客户端断开",
      "Client disconnected",
      "انقطاع العميل",
    ),
    completed: text("已完成", "Completed", "مكتمل"),
    retry_other_channel: text(
      "选择其他渠道重试",
      "Retry another channel",
      "إعادة المحاولة بقناة أخرى",
    ),
    response_started: text(
      "响应已开始",
      "Response already started",
      "بدأت الاستجابة",
    ),
    request_cancelled: text("请求已取消", "Request cancelled", "أُلغي الطلب"),
    already_settled: text("已结算", "Already settled", "تمت التسوية"),
    strict_affinity: text("严格亲和禁止切换", "Strict affinity", "ارتباط صارم"),
    retry_disabled_by_affinity: text(
      "亲和规则禁止重试",
      "Affinity prohibits retry",
      "الارتباط يمنع الإعادة",
    ),
    retry_disabled: text("重试已关闭", "Retry disabled", "الإعادة معطلة"),
    not_retryable: text("不可重试", "Not retryable", "غير قابل للإعادة"),
    unsafe_replay: text("无法安全重放", "Unsafe to replay", "إعادة غير آمنة"),
    attempt_budget_exhausted: text(
      "尝试次数耗尽",
      "Attempt budget exhausted",
      "نفدت المحاولات",
    ),
    retry_window_exhausted: text(
      "重试时间耗尽",
      "Retry window exhausted",
      "انتهت مهلة الإعادة",
    ),
    probe_completed: text("探测完成", "Probe completed", "اكتمل الفحص"),
    probe_not_applicable: text(
      "需人工检查或专用探测",
      "Manual review or dedicated probe required",
      "يتطلب مراجعة أو فحصاً مخصصاً",
    ),
    probe_cancelled: text("探测已取消", "Probe cancelled", "أُلغي الفحص"),
    probe_blocked: text(
      "等待其他保护解除",
      "Waiting for other protection",
      "بانتظار رفع الحماية الأخرى",
    ),
    interface: text(
      "接口与流式模式",
      "Interface and streaming mode",
      "الواجهة ووضع البث",
    ),
    handshake_completed: text(
      "握手完成",
      "Handshake completed",
      "اكتملت المصافحة",
    ),
    model: text("渠道与模型", "Channel and model", "القناة والنموذج"),
    credential: text("凭证", "Credential", "الاعتماد"),
    credential_model: text(
      "凭证与模型",
      "Credential and model",
      "الاعتماد والنموذج",
    ),
  };
  return (value: string) => labels[value] ?? value ?? "—";
}

export function UpstreamProtectionSettings() {
  const { text } = useSettingsText();
  const identity = useSessionQueryIdentity();
  const query = useQuery({
    queryKey: ["upstream-policy", ...identity],
    queryFn: ({ signal }) => apiGet<Snapshot>("/api/option/upstream", signal),
    retry: false,
  });
  const client = useQueryClient();
  const [draft, setDraft] = useState<{
    version: number;
    values: Policy;
  } | null>(null);
  const [notice, setNotice] = useState("");
  const data = query.data;
  const values: Policy | null =
    draft?.values ?? (data ? (JSON.parse(data.value) as Policy) : null);
  const save = useMutation({
    mutationFn: (input: NonNullable<typeof draft>) =>
      apiPut<{ applied: boolean }>("/api/option/upstream", {
        expected_version: input.version,
        value: JSON.stringify(input.values),
      }),
    onSuccess: async (result) => {
      setDraft(null);
      setNotice(
        result.applied
          ? text(
              "配置已保存并加载。",
              "Settings saved and loaded.",
              "تم حفظ الإعدادات وتحميلها.",
            )
          : text(
              "配置已保存，等待同步。",
              "Saved; synchronization pending.",
              "حُفظت؛ المزامنة معلقة.",
            ),
      );
      await Promise.all([
        query.refetch(),
        client.invalidateQueries({ queryKey: ["option-status"] }),
        client.invalidateQueries({ queryKey: ["gateway-options"] }),
      ]);
    },
  });
  useEffect(() => {
    if (!draft) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [draft]);
  const edit = (key: string, value: number | boolean) => {
    if (!data || !values) return;
    setNotice("");
    save.reset();
    setDraft({
      version: draft?.version ?? data.version,
      values: { ...values, [key]: value },
    });
  };
  const conflict =
    (!!draft && draft.version !== data?.version) ||
    (save.error instanceof ApiError && save.error.status === 409);
  const valid =
    values &&
    fields.every(
      ([key, , , , min, max]) =>
        Number.isInteger(values[key]) &&
        Number(values[key]) >= min &&
        Number(values[key]) <= max,
    ) &&
    Number(values.max_open_seconds) >= Number(values.open_seconds);
  return (
    <section className="gateway-panel upstream-panel">
      <h2>
        {text(
          "上游重试与资源熔断",
          "Upstream retries and circuit protection",
          "إعادة المحاولة وحماية الموارد",
        )}
      </h2>
      <p className="gateway-description">
        {text(
          "保护普通渠道和资源路由。熔断不移除模型能力或持久绑定；恢复探测会产生少量上游用量，不向用户扣费。",
          "Protects channel and resource routes. Model capabilities and bindings remain available. Recovery probes use a small amount of upstream capacity without charging users.",
          "تحمي القنوات والموارد دون إزالة القدرات أو الارتباطات. تستهلك الفحوص قدراً صغيراً من المنبع دون رسوم للمستخدمين.",
        )}
      </p>
      {query.isPending && <p>{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>}
      {query.isError && (
        <p role="alert">
          {text(
            "无法读取配置，请确认后端已更新。",
            "Could not load settings. Ensure the backend is updated.",
            "تعذر تحميل الإعدادات. تحقق من تحديث الخادم.",
          )}
        </p>
      )}
      {values && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (draft && valid && !conflict) save.mutate(draft);
          }}
        >
          <fieldset
            className="gateway-fields"
            disabled={save.isPending || query.isError}
          >
            <legend className="sr-only">
              {text("保护策略", "Protection policy", "سياسة الحماية")}
            </legend>
            {(
              [
                [
                  "retry",
                  text(
                    "启用跨渠道重试",
                    "Enable channel retry",
                    "تمكين إعادة المحاولة",
                  ),
                ],
                [
                  "enabled",
                  text(
                    "启用资源熔断与恢复探测",
                    "Enable protection and recovery probes",
                    "تمكين الحماية وفحوص التعافي",
                  ),
                ],
              ] as const
            ).map(([key, label]) => (
              <label className="gateway-toggle" key={key}>
                <input
                  type="checkbox"
                  checked={Boolean(values[key])}
                  onChange={(event) => edit(key, event.target.checked)}
                />
                {label}
              </label>
            ))}
            {fields.map(([key, zh, en, ar, min, max]) => (
              <div className="gateway-field" key={key}>
                <label htmlFor={`upstream-${key}`}>{text(zh, en, ar)}</label>
                <div className="gateway-number">
                  <input
                    id={`upstream-${key}`}
                    type="number"
                    min={min}
                    max={max}
                    step={1}
                    required
                    value={
                      Number.isFinite(values[key]) ? String(values[key]) : ""
                    }
                    onChange={(event) =>
                      edit(
                        key,
                        event.target.value === ""
                          ? NaN
                          : Number(event.target.value),
                      )
                    }
                  />
                </div>
              </div>
            ))}
          </fieldset>
          <p className="gateway-rule">
            {text(
              "重试预算仅限制新尝试，不截断已开始的响应。优先遵循上游冷却时间；凭证与额度隔离单独复查。配置由共享数据库同步。",
              "The retry window limits new attempts without truncating active responses. Upstream cooldown hints take precedence; credential and quota isolation use a separate recheck interval. Settings synchronize through the shared database.",
              "تحد المهلة المحاولات الجديدة دون قطع الاستجابة. تُراعى مهلة المنبع وتُفحص الاعتمادات والحصص بشكل منفصل. تتزامن الإعدادات عبر قاعدة البيانات المشتركة.",
            )}
          </p>
          {conflict && (
            <p role="alert" className="gateway-error">
              {text(
                "配置已变化，草稿已保留。请重新读取后编辑。",
                "Settings changed; your draft is retained. Reload before editing.",
                "تغيرت الإعدادات وحُفظت المسودة. أعد التحميل قبل التعديل.",
              )}
            </p>
          )}
          {save.isError && !conflict && (
            <p role="alert" className="gateway-error">
              {text(
                "未能确认保存结果，请重新读取。",
                "Save could not be confirmed. Reload settings.",
                "تعذر تأكيد الحفظ. أعد التحميل.",
              )}
            </p>
          )}
          {notice && <p role="status">{notice}</p>}
          <footer className="gateway-actions">
            <Button
              disabled={save.isPending || query.isFetching}
              onClick={async () => {
                const result = await query.refetch();
                if (result.isSuccess) {
                  setDraft(null);
                  save.reset();
                }
              }}
            >
              {text("重新读取", "Reload", "إعادة التحميل")}
            </Button>
            <Button
              type="submit"
              variant="primary"
              loading={save.isPending}
              disabled={
                !draft ||
                !valid ||
                conflict ||
                query.isFetching ||
                query.isError
              }
            >
              {text("保存策略", "Save policy", "حفظ السياسة")}
            </Button>
          </footer>
        </form>
      )}
      {!!data?.history.length && (
        <details>
          <summary>
            {text("策略变更记录", "Policy history", "سجل السياسة")}
          </summary>
          <ul>
            {data.history.map((entry) => (
              <li key={entry.id}>
                <time>
                  {new Date(entry.created_at * 1000).toLocaleString()}
                </time>{" "}
                · {text("操作人", "Actor", "المنفذ")} {entry.actor_id}
                <pre className="upstream-history">
                  {JSON.stringify(JSON.parse(entry.new_value), null, 2)}
                </pre>
              </li>
            ))}
          </ul>
        </details>
      )}
    </section>
  );
}

export function UpstreamEvents({
  requestId,
  resourceId,
}: {
  requestId?: string;
  resourceId?: string;
}) {
  const { text } = useSettingsText();
  const identity = useSessionQueryIdentity();
  const label = useProtectionLabels();
  const query = useQuery({
    queryKey: ["upstream-events", ...identity, requestId, resourceId],
    queryFn: ({ signal }) =>
      apiGet<Event[]>(
        `/api/upstream/events?${new URLSearchParams({ ...(requestId ? { request_id: requestId } : {}), ...(resourceId ? { resource_id: resourceId } : {}) })}`,
        signal,
      ),
    retry: false,
  });
  return (
    <section className="upstream-events">
      <h3>
        {text(
          "上游尝试与恢复事件",
          "Upstream attempts and recovery events",
          "محاولات المنبع وأحداث التعافي",
        )}
      </h3>
      <p>
        {text(
          "最近 100 条，保留 7 天。重试决策表示是否继续选路；是否发出调用单独记录。",
          "Latest 100 events, retained for 7 days. Retry decisions indicate whether routing continues; actual sends are recorded separately.",
          "آخر 100 حدث محفوظة لسبعة أيام. قرار الإعادة يعني استمرار التوجيه؛ تُسجّل الطلبات المرسلة منفصلة.",
        )}
      </p>
      {query.isError && (
        <p role="alert">
          {text("事件读取失败", "Could not load events", "تعذر تحميل الأحداث")}
        </p>
      )}
      {query.isPending && <p>{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>}
      {query.data?.length === 0 && (
        <p>
          {text(
            "暂无事件（历史请求不会补写）。",
            "No events. Historical requests are not backfilled.",
            "لا أحداث. لا تُضاف الطلبات التاريخية بأثر رجعي.",
          )}
        </p>
      )}
      {!!query.data?.length && (
        <div
          className="gateway-history"
          tabIndex={0}
          role="region"
          aria-label={text("上游事件", "Upstream events", "أحداث المنبع")}
        >
          <table>
            <thead>
              <tr>
                {[
                  text("时间", "Time", "الوقت"),
                  text("渠道 / 模型", "Channel / model", "القناة / النموذج"),
                  text("结果", "Result", "النتيجة"),
                  text("决策", "Decision", "القرار"),
                ].map((s) => (
                  <th scope="col" key={s}>
                    {s}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data.map((event) => (
                <tr key={event.id}>
                  <td>
                    {new Date(event.created_at * 1000).toLocaleString()}
                    <small>
                      {event.kind === "probe"
                        ? text("恢复探测", "Recovery probe", "فحص التعافي")
                        : event.sent
                          ? text("已调用上游", "Sent upstream", "أُرسل للمنبع")
                          : text(
                              "未调用上游",
                              "Not sent upstream",
                              "لم يُرسل للمنبع",
                            )}
                    </small>
                  </td>
                  <td>
                    {event.channel_id} / {event.model}
                  </td>
                  <td>
                    {event.status || "—"} · {label(event.class)}
                  </td>
                  <td>{label(event.decision)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

export function UpstreamHealth() {
  const { text } = useSettingsText();
  const identity = useSessionQueryIdentity();
  const role = useSystemAccess((state) => state.role);
  const label = useProtectionLabels();
  const query = useQuery({
    queryKey: ["upstream-health", ...identity],
    queryFn: ({ signal }) =>
      apiGet<{
        states: Health[];
        enabled: boolean;
        diagnostics?: {
          persistence_failures: number;
          last_persistence_failure: number;
        };
      }>("/api/upstream/health", signal),
    refetchInterval: 10000,
    retry: false,
  });
  const [selected, setSelected] = useState<string | undefined>();
  const [resetTarget, setResetTarget] = useState<Health | null>(null);
  const reset = useMutation({
    mutationFn: (state: Health) =>
      apiPost("/api/upstream/reset", { id: state.id, version: state.version }),
    onSuccess: () => query.refetch(),
  });
  const [allEvents, setAllEvents] = useState(false);
  const probe = useMutation({
    mutationFn: (id: string) => apiPost("/api/upstream/probe", { id }),
    onSuccess: () => query.refetch(),
  });
  const now = Date.now() / 1000;
  return (
    <section className="gateway-panel upstream-panel">
      <h2>
        {text(
          "资源熔断与恢复",
          "Resource protection and recovery",
          "حماية الموارد والتعافي",
        )}
      </h2>
      <p>
        {text(
          "状态由共享数据库协调。展示最多 500 条有效保护及近 24 小时更新的故障或恢复记录；从未出现故障的资源不单独建档。",
          "State is coordinated through the shared database. Shows up to 500 current protections or failure/recovery records updated in the last 24 hours; resources without failures have no state row.",
          "تُنسّق الحالة عبر قاعدة البيانات المشتركة. تُعرض حتى 500 حالة حماية أو سجل فشل وتعافٍ محدث خلال 24 ساعة؛ لا يُنشأ سجل للموارد التي لم تفشل.",
        )}
      </p>
      {!!query.data?.diagnostics?.persistence_failures && (
        <p role="alert">
          {text(
            "本副本启动以来，保护状态或事件保存失败次数：",
            "Protection state or event persistence failures since this replica started: ",
            "إخفاقات حفظ الحماية أو الأحداث منذ بدء هذه النسخة: ",
          )}
          {query.data.diagnostics.persistence_failures}
          {" · "}
          {text("最近一次：", "Last failure: ", "آخر إخفاق: ")}
          {new Date(
            query.data.diagnostics.last_persistence_failure * 1000,
          ).toLocaleString()}
        </p>
      )}
      {query.data && !query.data.enabled && (
        <p role="status">
          {text(
            "保护已关闭，下列历史状态不限制选路。",
            "Protection is disabled; stored states do not restrict routing.",
            "الحماية معطلة؛ الحالات المحفوظة لا تقيد التوجيه.",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "无法读取资源健康状态，请确认后端已更新。",
            "Could not load resource health. Ensure the backend is updated.",
            "تعذر تحميل صحة الموارد. تحقق من تحديث الخادم.",
          )}
        </p>
      )}
      {query.isPending && <p>{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>}
      {query.data?.states.length === 0 && (
        <p>
          {text(
            "暂无资源保护记录。",
            "No resource protection records yet.",
            "لا توجد سجلات حماية بعد.",
          )}
        </p>
      )}
      {!!query.data?.states.length && (
        <div
          className="gateway-history"
          tabIndex={0}
          role="region"
          aria-label={text("资源健康状态", "Resource health", "صحة الموارد")}
        >
          <table>
            <thead>
              <tr>
                {[
                  text("渠道 / 模型", "Channel / model", "القناة / النموذج"),
                  text("状态 / 范围", "State / scope", "الحالة / النطاق"),
                  text(
                    "原因 / 失败次数",
                    "Cause / failures",
                    "السبب / الإخفاقات",
                  ),
                  text(
                    "下次探测 / 最近结果",
                    "Next probe / last result",
                    "الفحص التالي / آخر نتيجة",
                  ),
                  text("操作", "Actions", "الإجراءات"),
                ].map((s) => (
                  <th scope="col" key={s}>
                    {s}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data.states.map((s) => (
                <tr key={s.id}>
                  <td>
                    {s.channel_id} / {s.model}
                    <small>
                      {s.capability} ·{" "}
                      {s.stream
                        ? text("流式", "Streaming", "بث")
                        : text("非流式", "Non-streaming", "دون بث")}
                    </small>
                  </td>
                  <td>
                    <strong>
                      {!s.open_until
                        ? text("正常", "Healthy", "سليم")
                        : s.open_until > now
                          ? text(
                              "熔断 / 冷却",
                              "Open / cooling",
                              "معزول / تبريد",
                            )
                          : text("半开待探测", "Half-open", "بانتظار الفحص")}
                    </strong>
                    <small>{label(s.scope)}</small>
                  </td>
                  <td>
                    {label(s.reason) || "—"} / {s.failures}
                  </td>
                  <td>
                    {s.open_until
                      ? new Date(s.open_until * 1000).toLocaleString()
                      : "—"}
                    <small>{label(s.probe_result) || "—"}</small>
                  </td>
                  <td>
                    <div className="upstream-row-actions">
                      {role === ROLE_ROOT && s.open_until > 0 && (
                        <Button
                          disabled={reset.isPending}
                          onClick={() => setResetTarget(s)}
                        >
                          {text("人工解除", "Manual reset", "إلغاء يدوي")}
                        </Button>
                      )}
                      <Button
                        onClick={() => {
                          setSelected(s.id);
                          setAllEvents(true);
                        }}
                      >
                        {text("事件", "Events", "الأحداث")}
                      </Button>
                      {role === ROLE_ROOT && s.open_until > 0 && (
                        <Button
                          disabled={
                            !query.data.enabled ||
                            s.open_until > now ||
                            s.lease_until > now ||
                            probe.isPending
                          }
                          onClick={() => probe.mutate(s.id)}
                        >
                          {text("探测恢复", "Probe recovery", "فحص التعافي")}
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {probe.isError && (
        <p role="alert">
          {text(
            "未能执行探测，资源可能仍在冷却或已有探测运行。",
            "Probe could not run; cooldown or another probe may be active.",
            "تعذر الفحص؛ قد يكون التبريد أو فحص آخر نشطاً.",
          )}
        </p>
      )}
      {reset.isError && (
        <p role="alert">
          {text(
            "状态已变化或无法解除，请刷新后重试。",
            "State changed or reset failed. Refresh and retry.",
            "تغيرت الحالة أو تعذر الإلغاء. حدّث وأعد المحاولة.",
          )}
        </p>
      )}
      <ConfirmDialog
        open={!!resetTarget}
        onOpenChange={(open) => {
          if (!open) setResetTarget(null);
        }}
        onConfirm={() => {
          if (resetTarget) reset.mutate(resetTarget);
          setResetTarget(null);
        }}
        title={text(
          "解除临时资源保护？",
          "Clear temporary resource protection?",
          "إلغاء حماية المورد المؤقتة؟",
        )}
        description={text(
          "仅在已确认上游恢复后操作。后续请求将重新参与选路；渠道禁用和资源平台的限制仍然有效。",
          "Use after confirming upstream recovery. Routing may resume; channel disables and resource platform restrictions remain in force.",
          "استخدم بعد تأكيد تعافي المنبع. قد يُستأنف التوجيه مع بقاء تعطيل القنوات وقيود المنصة سارية.",
        )}
        confirmLabel={text("确认解除", "Clear protection", "إلغاء الحماية")}
      />
      <div className="gateway-actions">
        <Button onClick={() => void query.refetch()}>
          {text("刷新状态", "Refresh", "تحديث")}
        </Button>
        <Button
          onClick={() => {
            setSelected(undefined);
            setAllEvents(!allEvents);
          }}
        >
          {text("最近事件", "Recent events", "الأحداث الأخيرة")}
        </Button>
      </div>
      {allEvents && (
        <UpstreamEvents key={selected ?? "all"} resourceId={selected} />
      )}
    </section>
  );
}
