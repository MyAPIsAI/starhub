import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { ApiError, apiGet, apiPut } from "../lib/api/http";
import { useSessionQueryIdentity } from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import "./channel-traffic.css";

type ChannelWeight = {
  channel_id: string;
  weight: number;
  priority?: number | null;
};
type Rule = {
  group: string;
  model: string;
  enabled: boolean;
  channels: ChannelWeight[];
};
type Candidate = ChannelWeight & {
  group: string;
  model: string;
  name: string;
  priority: number;
  eligible: boolean;
};
type Snapshot = {
  version: number;
  channel_revision: number;
  mode: "local" | "commercial";
  policy: { rules: Rule[] };
  candidates: Candidate[];
  history: { version: number; created_at: number; actor_id: number }[];
};
type Draft = {
  version: number;
  channelRevision: number;
  group: string;
  model: string;
  enabled: boolean;
  weights: Record<string, string>;
  priorities: Record<string, string>;
  remove: boolean;
};

function readSnapshot(data: Snapshot) {
  if (
    !data ||
    !Number.isSafeInteger(data.version) ||
    data.version < 0 ||
    !Number.isSafeInteger(data.channel_revision) ||
    data.channel_revision < 0 ||
    !["local", "commercial"].includes(data.mode) ||
    !Array.isArray(data.policy?.rules) ||
    !Array.isArray(data.candidates) ||
    !Array.isArray(data.history)
  )
    throw Error("Invalid traffic configuration");
  return data;
}

export function ChannelTrafficSettings() {
  const { text, lang } = useSettingsText();
  const identity = useSessionQueryIdentity();
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["channel-traffic", ...identity],
    retry: false,
    queryFn: async ({ signal }) =>
      readSnapshot(
        await apiGet<Snapshot>("/api/option/channel-traffic", signal),
      ),
  });
  const [modelChoice, setModelChoice] = useState("");
  const [groupChoice, setGroupChoice] = useState("");
  const [draft, setDraft] = useState<Draft>();
  const [notice, setNotice] = useState("");
  const data = query.data;
  const scopes = [...(data?.candidates ?? []), ...(data?.policy.rules ?? [])];
  const models = [...new Set(scopes.map((row) => row.model))].sort();
  const model = models.includes(modelChoice) ? modelChoice : (models[0] ?? "");
  const groups = [
    ...new Set(
      scopes.filter((row) => row.model === model).map((row) => row.group),
    ),
  ].sort();
  const group = groups.includes(groupChoice)
    ? groupChoice
    : groups.includes("default")
      ? "default"
      : (groups[0] ?? "");
  const rows = (data?.candidates ?? []).filter(
    (row) => row.model === model && row.group === group,
  );
  const saved = data?.policy.rules.find(
    (rule) => rule.model === model && rule.group === group,
  );
  const defaults = Object.fromEntries(
    rows.map((row) => [
      row.channel_id,
      String(Math.min(1000000, Math.max(1, row.weight))),
    ]),
  );
  const baseWeights = saved
    ? Object.fromEntries(
        rows.map((row) => [
          row.channel_id,
          String(
            saved.channels.find((ch) => ch.channel_id === row.channel_id)
              ?.weight ?? 0,
          ),
        ]),
      )
    : defaults;
  const weights = draft?.weights ?? baseWeights;
  const defaultPriorities = Object.fromEntries(
    rows.map((row) => [row.channel_id, String(row.priority)]),
  );
  const basePriorities = Object.fromEntries(
    rows.map((row) => [
      row.channel_id,
      String(
        saved?.channels.find((ch) => ch.channel_id === row.channel_id)
          ?.priority ?? row.priority,
      ),
    ]),
  );
  const priorities = draft?.priorities ?? basePriorities;
  const enabled = draft?.enabled ?? saved?.enabled ?? false;
  const local = data?.mode === "local";
  const conflict =
    !!draft &&
    !!data &&
    (draft.version !== data.version ||
      draft.channelRevision !== data.channel_revision);
  const validWeight = (value: string) =>
    /^\d+$/.test(value) && Number(value) <= 1000000;
  const validPriority = (value: string) =>
    /^-?\d+$/.test(value) && Math.abs(Number(value)) <= 1000000;
  const valid =
    rows.length <= 500 &&
    rows.every((row) => validWeight(weights[row.channel_id] ?? "0")) &&
    rows.every((row) => validPriority(priorities[row.channel_id] ?? "")) &&
    (!enabled || rows.some((row) => Number(weights[row.channel_id]) > 0));
  const candidates = rows.filter(
    (row) =>
      row.eligible &&
      Number(weights[row.channel_id]) > 0 &&
      validPriority(priorities[row.channel_id] ?? ""),
  );
  const priority = Math.max(
    ...candidates.map((row) => Number(priorities[row.channel_id])),
  );
  const total = candidates
    .filter((row) => Number(priorities[row.channel_id]) === priority)
    .reduce((sum, row) => sum + Number(weights[row.channel_id]), 0);
  const removed =
    saved?.channels.filter(
      (ch) => !rows.some((row) => row.channel_id === ch.channel_id),
    ).length ?? 0;
  const mutation = useMutation({
    retry: false,
    mutationFn: (input: Draft) =>
      apiPut<{ version: number; applied: boolean }>(
        "/api/option/channel-traffic",
        {
          expected_version: input.version,
          channel_revision: input.channelRevision,
          remove: input.remove,
          rule: {
            group: input.group,
            model: input.model,
            enabled: input.enabled,
            channels: Object.entries(input.weights).map(
              ([channel_id, weight]) => ({
                channel_id,
                weight: Number(weight),
                priority: Number(input.priorities[channel_id]),
              }),
            ),
          },
        },
      ),
    onSuccess: async (result) => {
      setDraft(undefined);
      setNotice(
        result.applied
          ? text(
              "已保存并加载；新请求使用更新后的策略。",
              "Saved and loaded. New requests use the updated policy.",
              "تم الحفظ والتحميل. تستخدم الطلبات الجديدة السياسة المحدّثة.",
            )
          : text(
              "已保存，当前进程尚未加载；请查看配置同步详情。",
              "Saved, but not loaded on this process. Check configuration synchronization.",
              "تم الحفظ ولم يُحمّل على هذه العملية. راجع مزامنة الإعدادات.",
            ),
      );
      await Promise.all(
        [
          "channel-traffic",
          "options",
          "option-status",
          "option-history",
          "gateway-options",
          "routing-affinity-cache",
        ].map((key) => client.invalidateQueries({ queryKey: [key] })),
      );
    },
  });
  const edit = (change: Partial<Draft>) => {
    if (!data) return;
    setNotice("");
    mutation.reset();
    setDraft((current) => ({
      ...(current ?? {
        version: data.version,
        channelRevision: data.channel_revision,
        group,
        model,
        enabled,
        weights: baseWeights,
        priorities: basePriorities,
        remove: false,
      }),
      ...change,
    }));
  };
  useEffect(() => {
    if (!draft) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [draft]);
  const busy = mutation.isPending || query.isFetching;
  const stale =
    conflict ||
    (mutation.error instanceof ApiError && mutation.error.status === 409);
  return (
    <section
      className="gateway-panel channel-traffic"
      aria-labelledby="channel-traffic-title"
    >
      <div className="traffic-heading">
        <h2 id="channel-traffic-title">
          {text(
            "渠道流量分配",
            "Channel traffic allocation",
            "توزيع حركة القنوات",
          )}
        </h2>
        <div className="traffic-toolbar">
          {data && models.length > 0 && (
            <label className="traffic-toggle">
              <input
                type="checkbox"
                checked={enabled}
                disabled={!local || busy || query.isError || stale}
                onChange={(e) =>
                  edit({ enabled: e.target.checked, remove: false })
                }
              />
              {text(
                "启用比例分流",
                "Enable weighted allocation",
                "تفعيل التوزيع بالأوزان",
              )}
            </label>
          )}
          <Button
            disabled={busy || !!draft}
            onClick={() => void query.refetch()}
          >
            {text("刷新", "Refresh", "تحديث")}
          </Button>
        </div>
      </div>
      <p className="gateway-description">
        {text(
          "按模型和分组设置渠道优先级与权重，百分比自动计算。仅影响普通渠道路由。",
          "Set channel priorities and weights per model and group; percentages are calculated automatically. Applies to ordinary channel routing.",
          "اضبط أولويات القنوات وأوزانها لكل نموذج ومجموعة؛ تُحسب النسب تلقائيًا. يسري على توجيه القنوات العادي.",
        )}
      </p>
      {query.isLoading && (
        <p role="status">
          {text("正在读取配置…", "Loading settings…", "جارٍ تحميل الإعدادات…")}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "配置读取失败，请刷新后重试。",
            "Could not load settings. Refresh and retry.",
            "تعذر تحميل الإعدادات. حدّث وأعد المحاولة.",
          )}
        </p>
      )}
      {data && (
        <>
          {!local && (
            <p className="gateway-rule" role="status">
              {text(
                "当前为商业评分路由，此配置不生效。可恢复默认以移除已保存规则。",
                "Commercial scoring routing is active; these allocations do not apply. Restore defaults to remove saved rules.",
                "توجيه التقييم التجاري نشط؛ لا تسري هذه الأوزان. استعد الافتراضي لحذف القواعد المحفوظة.",
              )}
            </p>
          )}
          {!models.length ? (
            <p>
              {text(
                "暂无可配置的渠道模型，请先在渠道管理录入。",
                "No channel models available. Add them in channel management first.",
                "لا توجد نماذج قنوات. أضفها أولًا في إدارة القنوات.",
              )}
            </p>
          ) : (
            <>
              <div className="traffic-selectors">
                <label>
                  {text("模型", "Model", "النموذج")}
                  <select
                    value={model}
                    disabled={!!draft || busy}
                    onChange={(e) => {
                      setModelChoice(e.target.value);
                      setGroupChoice("");
                      setNotice("");
                      mutation.reset();
                    }}
                  >
                    {models.map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {text("分组", "Group", "المجموعة")}
                  <select
                    value={group}
                    disabled={!!draft || busy}
                    onChange={(e) => {
                      setGroupChoice(e.target.value);
                      setNotice("");
                      mutation.reset();
                    }}
                  >
                    {groups.map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <p className="gateway-rule">
                {text(
                  "启用后按请求分配渠道，不保持会话粘滞。比例为长期预计值，实际分布受渠道可用性和重试影响。",
                  "When enabled, requests are allocated without session affinity. Percentages are long-term estimates affected by availability and retries.",
                  "عند التفعيل توزّع الطلبات دون تثبيت الجلسات. النسب تقديرات طويلة الأجل تتأثر بالتوافر وإعادة المحاولة.",
                )}
              </p>
              <div
                className="traffic-table-wrap"
                tabIndex={0}
                role="region"
                aria-label={text(
                  "渠道权重",
                  "Channel weights",
                  "أوزان القنوات",
                )}
              >
                <table className="traffic-table">
                  <thead>
                    <tr>
                      <th>{text("渠道", "Channel", "القناة")}</th>
                      <th>{text("优先级", "Priority", "الأولوية")}</th>
                      <th>{text("权重", "Weight", "الوزن")}</th>
                      <th>
                        {text("预计占比", "Estimated share", "النسبة المقدّرة")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row) => {
                      const weight = Number(weights[row.channel_id] ?? 0);
                      const share =
                        row.eligible &&
                        weight > 0 &&
                        Number(priorities[row.channel_id]) === priority &&
                        total > 0
                          ? weight / total
                          : 0;
                      return (
                        <tr key={row.channel_id}>
                          <td>
                            <strong>
                              {row.channel_id} · {row.name}
                            </strong>
                            {!row.eligible && (
                              <small>
                                {text(
                                  "当前配置不可路由",
                                  "Not currently route eligible",
                                  "غير مؤهلة للتوجيه حاليًا",
                                )}
                              </small>
                            )}
                          </td>
                          <td>
                            <input
                              type="number"
                              min={-1000000}
                              max={1000000}
                              step={1}
                              value={priorities[row.channel_id] ?? ""}
                              aria-label={text(
                                `渠道 ${row.channel_id} 优先级`,
                                `Channel ${row.channel_id} priority`,
                                `أولوية القناة ${row.channel_id}`,
                              )}
                              aria-invalid={
                                !validPriority(priorities[row.channel_id] ?? "")
                              }
                              disabled={
                                !local ||
                                !enabled ||
                                busy ||
                                query.isError ||
                                stale
                              }
                              onChange={(e) =>
                                edit({
                                  priorities: {
                                    ...priorities,
                                    [row.channel_id]: e.target.value,
                                  },
                                  remove: false,
                                })
                              }
                            />
                          </td>
                          <td>
                            <input
                              type="number"
                              min={0}
                              max={1000000}
                              step={1}
                              value={weights[row.channel_id] ?? "0"}
                              aria-label={text(
                                `渠道 ${row.channel_id} 权重`,
                                `Channel ${row.channel_id} weight`,
                                `وزن القناة ${row.channel_id}`,
                              )}
                              aria-invalid={
                                !validWeight(weights[row.channel_id] ?? "0")
                              }
                              disabled={
                                !local ||
                                !enabled ||
                                busy ||
                                query.isError ||
                                stale
                              }
                              onChange={(e) =>
                                edit({
                                  weights: {
                                    ...weights,
                                    [row.channel_id]: e.target.value,
                                  },
                                  remove: false,
                                })
                              }
                            />
                          </td>
                          <td>
                            {!enabled || !local ? (
                              "—"
                            ) : row.eligible &&
                              weight > 0 &&
                              Number(priorities[row.channel_id]) < priority ? (
                              text(
                                "备用优先级",
                                "Fallback tier",
                                "أولوية احتياطية",
                              )
                            ) : (
                              <span className="traffic-share">
                                {new Intl.NumberFormat(lang, {
                                  style: "percent",
                                  maximumFractionDigits: 1,
                                }).format(share)}
                              </span>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              <p className="gateway-description">
                {text(
                  "优先级越大越优先，同优先级按权重分流；权重 0 不参与常规分流。启用普通渠道兜底时可突破本规则。预估未计入实时熔断与凭证状态，新增渠道默认不加入已保存规则。",
                  "Higher priorities are preferred; equal priorities share traffic by weight. Zero weight excludes normal traffic; enabled ordinary fallback may bypass this rule. Estimates exclude live protection and credential state; new channels are not added automatically.",
                  "تُفضّل الأولوية الأعلى وتوزّع الحركة بالأوزان عند تساويها. الوزن صفر يستبعد الحركة العادية، ويمكن للقنوات الاحتياطية تجاوز القاعدة عند تمكينها. لا تشمل التقديرات الحماية وحالة الاعتماد الآنية ولا تضاف القنوات الجديدة تلقائيًا.",
                )}
              </p>
              {!!removed && (
                <p role="status">
                  {text(
                    `有 ${removed} 个已保存渠道不再提供此模型或分组，保存时将移除这些条目。`,
                    `${removed} saved channels no longer offer this model/group; saving removes those entries.`,
                    `${removed} قنوات محفوظة لم تعد توفر هذا النموذج أو المجموعة؛ سيزيل الحفظ تلك العناصر.`,
                  )}
                </p>
              )}
              {!valid && (
                <p role="alert">
                  {text(
                    "优先级须为 -1000000～1000000 的整数，权重须为 0～1000000 的整数；启用时至少一个权重大于 0，每个范围最多 500 个渠道。",
                    "Use integer priorities from -1000000 to 1000000 and weights from 0 to 1000000; at least one weight must be positive when enabled, with at most 500 channels per scope.",
                    "الأولوية عدد صحيح بين -1000000 و1000000 والوزن بين 0 و1000000؛ يلزم وزن موجب عند التفعيل وبحد أقصى 500 قناة.",
                  )}
                </p>
              )}
              {stale && (
                <p role="alert">
                  {text(
                    "配置或渠道已变化。请取消草稿并刷新后重新编辑。",
                    "Settings or channels changed. Cancel the draft and refresh before editing again.",
                    "تغيّرت الإعدادات أو القنوات. ألغِ المسودة وحدّث ثم عدّل مجددًا.",
                  )}
                </p>
              )}
              {mutation.isError && !stale && (
                <p role="alert">
                  {text(
                    "保存失败，配置未确认。请刷新核对后重试。",
                    "Save was not confirmed. Refresh to check before retrying.",
                    "لم يُؤكد الحفظ. حدّث للتحقق قبل إعادة المحاولة.",
                  )}
                </p>
              )}
              {notice && <p role="status">{notice}</p>}
              {draft?.remove && (
                <p role="status">
                  {text(
                    "保存后移除此范围的比例配置，恢复原路由与亲和策略。",
                    "Saving removes this allocation and restores the original routing and affinity policy.",
                    "سيزيل الحفظ هذا التوزيع ويستعيد سياسة التوجيه والتقارب الأصلية.",
                  )}
                </p>
              )}
              <div className="gateway-actions">
                <Button
                  disabled={!saved || busy || query.isError || stale}
                  onClick={() =>
                    edit({
                      enabled: false,
                      weights: defaults,
                      priorities: defaultPriorities,
                      remove: true,
                    })
                  }
                >
                  {text("恢复默认", "Restore defaults", "استعادة الافتراضي")}
                </Button>
                {draft && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      setDraft(undefined);
                      mutation.reset();
                      setNotice("");
                      void query.refetch();
                    }}
                  >
                    {text("取消更改", "Cancel changes", "إلغاء التغييرات")}
                  </Button>
                )}
                <Button
                  variant="primary"
                  loading={mutation.isPending}
                  disabled={
                    !draft ||
                    busy ||
                    stale ||
                    query.isError ||
                    (!draft.remove && (!local || !valid))
                  }
                  onClick={() => draft && mutation.mutate(draft)}
                >
                  {text("保存", "Save", "حفظ")}
                </Button>
              </div>
            </>
          )}
          {!!data.history.length && (
            <details className="traffic-history">
              <summary>
                {text(
                  "最近分流配置变更",
                  "Recent allocation changes",
                  "تغييرات التوزيع الأخيرة",
                )}
              </summary>
              <ul>
                {data.history.map((entry) => (
                  <li key={entry.version}>
                    v{entry.version} ·{" "}
                    {new Date(entry.created_at * 1000).toLocaleString(lang)} ·{" "}
                    {text("操作用户", "Actor", "المستخدم")}: {entry.actor_id}
                  </li>
                ))}
              </ul>
            </details>
          )}
        </>
      )}
    </section>
  );
}
