import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useResourceCatalog, useResourceMutation } from "../lib/api/resource";
import {
  ROLE_ADMIN,
  ROLE_ROOT,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import {
  parseResourceSnapshot,
  resourceImportDiff,
} from "../lib/resource-catalog";
import type { ResourceSnapshot } from "../config/models";
import { useSettingsText } from "./settings-text";
import "./resource-inventory.css";
import { ResourcePublication } from "./resource-publication";

type Draft =
  | {
      kind: "import";
      revision: number;
      json: string;
      snapshot?: ResourceSnapshot;
    }
  | {
      kind: "placement";
      revision: number;
      id: string;
      model: string;
      groups: string;
      enabled: boolean;
    };

export function ResourceInventory() {
  const identity = useSessionQueryIdentity();
  return <ResourceInventoryView key={JSON.stringify(identity)} />;
}

function ResourceInventoryView() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const query = useResourceCatalog();
  const mutation = useResourceMutation();
  const [search, setSearch] = useState("");
  const [channel, setChannel] = useState("");
  const [region, setRegion] = useState("");
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(0);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  if (role !== ROLE_ADMIN && role !== ROLE_ROOT) return null;
  const data = query.error ? undefined : query.data;
  const snapshot = data?.snapshot;
  const rows = (snapshot?.instances ?? []).filter(
    (row) =>
      (!channel || row.channelId === channel) &&
      (!region || JSON.stringify(row.region) === region) &&
      (!status || row.status === status) &&
      [
        row.id,
        row.upstreamModel,
        row.canonicalModel,
        data?.placements[row.id]?.modelId ?? "",
      ].some((value) =>
        value.toLowerCase().includes(search.trim().toLowerCase()),
      ),
  );
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const conflict = Boolean(draft && data && draft.revision !== data.revision);
  const diff =
    draft?.kind === "import" && draft.snapshot
      ? resourceImportDiff(snapshot ?? null, draft.snapshot)
      : null;
  const unknown = text("未知区域", "Unknown region", "منطقة غير معروفة");
  const statusLabel = (value: string) =>
    value === "available"
      ? text("来源可用", "Available", "متاح")
      : value === "unavailable"
        ? text("来源不可用", "Unavailable", "غير متاح")
        : text("来源已下架", "Retired", "مسحوب");
  const begin = (next: Draft) => {
    mutation.reset();
    setDraft(next);
    setReview(false);
    setError("");
    setNotice("");
  };
  const close = () => {
    if (!mutation.isPending) {
      setDraft(null);
      setError("");
      mutation.reset();
    }
  };
  const preview = () => {
    if (!draft || !data) return;
    try {
      if (draft.kind === "import") {
        const next = parseResourceSnapshot(JSON.parse(draft.json));
        if (next.sequence <= (snapshot?.sequence ?? 0))
          throw new Error("sequence");
        setDraft({ ...draft, snapshot: next });
      } else {
        if (
          draft.enabled &&
          (!draft.model.trim() ||
            !draft.groups.split(",").some((group) => group.trim()))
        )
          throw new Error("placement");
      }
      setError("");
      setReview(true);
    } catch {
      setError(
        text(
          "请检查字段；快照版本须递增，启用投放须填写模型和分组。",
          "Check fields. Snapshot sequence must increase; enabled placements need a model and groups.",
          "تحقق من الحقول. يجب زيادة تسلسل اللقطة وتحديد النموذج والمجموعات للنشر.",
        ),
      );
    }
  };
  const save = async () => {
    if (!draft || !review || conflict || !data) return;
    try {
      const result = await mutation.mutateAsync(
        draft.kind === "import"
          ? { expected_revision: draft.revision, snapshot: draft.snapshot! }
          : {
              expected_revision: draft.revision,
              id: draft.id,
              placement: {
                modelId: draft.model.trim() || null,
                groupIds: draft.groups
                  .split(",")
                  .map((group) => group.trim())
                  .filter(Boolean),
                enabled: draft.enabled,
              },
            },
      );
      setNotice(
        text(
          `已保存资源配置 v${result.revision}。`,
          `Inventory saved as v${result.revision}.`,
          `حُفظ المخزون بالإصدار v${result.revision}.`,
        ),
      );
      setDraft(null);
      setError("");
    } catch {
      setError(
        text(
          "无法确认保存结果。请刷新核对；若版本已变化，请重新打开编辑后再保存。",
          "Save outcome is unconfirmed. Refresh to check. If the revision changed, reopen the editor before saving.",
          "تعذر تأكيد الحفظ. حدّث للتحقق، ثم أعد فتح المحرر إذا تغير الإصدار.",
        ),
      );
    }
  };
  return (
    <div className="resource-inventory">
      <header>
        <div>
          <h1>
            {text(
              "渠道资源与投放",
              "Channel inventory and placements",
              "مخزون القنوات والنشر",
            )}
          </h1>
          <p>
            {text(
              "集中保存供应商资源、区域和投放清单。",
              "Store supplier inventory, regions and placements centrally.",
              "احفظ مخزون الموردين والمناطق وإعدادات النشر مركزيًا.",
            )}
          </p>
        </div>
        <div className="ri-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text("刷新", "Refresh", "تحديث")}
          </Button>
          <Button
            disabled={!data}
            onClick={() =>
              data &&
              begin({
                kind: "import",
                revision: data.revision,
                json: JSON.stringify(
                  snapshot
                    ? {
                        ...snapshot,
                        sequence: snapshot.sequence + 1,
                        publishedAt: new Date().toISOString(),
                      }
                    : {
                        id: "inventory-1",
                        sequence: 1,
                        publishedAt: new Date().toISOString(),
                        source: "",
                        channels: [],
                        instances: [],
                      },
                  null,
                  2,
                ),
              })
            }
          >
            {text("导入快照", "Import snapshot", "استيراد لقطة")}
          </Button>
          <Button asChild>
            <a href="#gateway-publication">
              {text(
                "网关发布预览",
                "Gateway publication preview",
                "معاينة نشر البوابة",
              )}
            </a>
          </Button>
          <Button asChild>
            <Link to="/admin?tab=resources#regions">
              {text("区域目录", "Region directory", "دليل المناطق")}
            </Link>
          </Button>
        </div>
      </header>
      <aside className="ri-boundary">
        {text(
          "此清单暂不改变网关实际路由或调用授权。上游密钥与真实渠道在后台单独管理；区域及延迟来自导入数据，并非实测或服务承诺。",
          "This inventory does not yet change gateway routing or API authorization. Manage credentials and live channels separately in Admin. Imported regions and latency are metadata, not measurements or service commitments.",
          "لا يغيّر هذا المخزون توجيه البوابة أو صلاحيات الواجهة بعد. تُدار بيانات الاعتماد والقنوات الفعلية في الإدارة. المناطق وزمن الاستجابة بيانات مستوردة وليست قياسات أو ضمانات.",
        )}{" "}
        <Link to="/admin?tab=resources">
          {text("打开后台渠道", "Open live channels", "فتح القنوات الفعلية")}
        </Link>
      </aside>
      {query.isLoading && (
        <p role="status">
          {text("正在读取资源…", "Loading inventory…", "جارٍ تحميل المخزون…")}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            "无法读取服务器资源，刷新后重试。",
            "Server inventory is unavailable. Refresh to retry.",
            "مخزون الخادم غير متاح. حدّث لإعادة المحاولة.",
          )}
        </p>
      )}
      {notice && (
        <p role="status" className="ri-notice">
          {notice}
        </p>
      )}
      {data && (
        <>
          <p className="ri-summary">
            <strong>v{data.revision}</strong> ·{" "}
            {snapshot?.id ??
              text(
                "尚未导入快照",
                "No snapshot imported",
                "لم تُستورد لقطة",
              )}{" "}
            · {snapshot?.instances.length ?? 0}{" "}
            {text("个资源", "resources", "موارد")}
          </p>
          {!snapshot ? (
            <p className="ri-empty">
              {text(
                "服务器暂无资源快照。导入真实清单后可编辑投放；不会自动发布浏览器内的演示资源。",
                "The server has no inventory snapshot. Import your inventory to edit placements. Browser demo resources are never published automatically.",
                "لا توجد لقطة مخزون في الخادم. استورد المخزون لتحرير النشر؛ لا تُنشر موارد العرض المحلية تلقائيًا.",
              )}
            </p>
          ) : (
            <>
              <p className="ri-source">
                {text("来源", "Source", "المصدر")}: {snapshot.source || "—"} ·{" "}
                {text("来源发布时间", "Source publication", "نشر المصدر")}:{" "}
                <time dateTime={snapshot.publishedAt}>
                  {snapshot.publishedAt}
                </time>
              </p>
              <div className="ri-filters">
                <label>
                  {text(
                    "搜索模型或资源",
                    "Search model or resource",
                    "بحث النموذج أو المورد",
                  )}
                  <input
                    value={search}
                    onChange={(e) => {
                      setSearch(e.target.value);
                      setPage(0);
                    }}
                  />
                </label>
                <label>
                  {text("渠道", "Channel", "القناة")}
                  <select
                    aria-label={text("渠道", "Channel", "القناة")}
                    value={channel}
                    onChange={(e) => {
                      setChannel(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">
                      {text("全部渠道", "All channels", "كل القنوات")}
                    </option>
                    {snapshot.channels.map((row) => (
                      <option key={row.id} value={row.id}>
                        {row.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {text("区域", "Region", "المنطقة")}
                  <select
                    aria-label={text("区域", "Region", "المنطقة")}
                    value={region}
                    onChange={(e) => {
                      setRegion(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">
                      {text("全部区域", "All regions", "كل المناطق")}
                    </option>
                    {[...new Set(snapshot.instances.map((row) => row.region))]
                      .sort()
                      .map((value) => (
                        <option key={value} value={JSON.stringify(value)}>
                          {data.regions[value]?.name ?? (value || unknown)}
                        </option>
                      ))}
                  </select>
                </label>
                <label>
                  {text("来源状态", "Source state", "حالة المصدر")}
                  <select
                    aria-label={text("来源状态", "Source state", "حالة المصدر")}
                    value={status}
                    onChange={(e) => {
                      setStatus(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">
                      {text("全部状态", "All states", "كل الحالات")}
                    </option>
                    {["available", "unavailable", "retired"].map((value) => (
                      <option key={value} value={value}>
                        {statusLabel(value)}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <div
                className="ri-table"
                role="region"
                aria-label={text(
                  "服务器资源列表",
                  "Server inventory",
                  "مخزون الخادم",
                )}
                tabIndex={0}
              >
                <table>
                  <thead>
                    <tr>
                      {[
                        text(
                          "资源 / 上游模型",
                          "Resource / upstream model",
                          "المورد / النموذج",
                        ),
                        text(
                          "区域 / 来源状态",
                          "Region / source state",
                          "المنطقة / الحالة",
                        ),
                        text("保存的投放", "Stored placement", "النشر المحفوظ"),
                        text("操作", "Actions", "الإجراءات"),
                      ].map((label) => (
                        <th key={label}>{label}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows
                      .slice(currentPage * 20, currentPage * 20 + 20)
                      .map((row) => {
                        const placement = data.placements[row.id];
                        return (
                          <tr key={row.id} data-resource-id={row.id}>
                            <td>
                              <strong>{row.id}</strong>
                              <code>{row.upstreamModel}</code>
                              <small>
                                {row.protocols.join(" / ")} ·{" "}
                                {row.contextLength.toLocaleString()}
                              </small>
                            </td>
                            <td>
                              <span>
                                {data.regions[row.region]?.name ??
                                  (row.region || unknown)}
                              </span>
                              <span>{statusLabel(row.status)}</span>
                              <small>
                                {row.latencyMs} ms ·{" "}
                                {text("来源值", "Source value", "قيمة المصدر")}
                              </small>
                            </td>
                            <td>
                              <span>
                                {placement?.modelId ??
                                  text("未映射", "Unmapped", "غير مرتبط")}
                              </span>
                              <small>
                                {placement?.groupIds.join(", ") || "—"}
                              </small>
                              <small>
                                {placement?.enabled
                                  ? text(
                                      "清单投放启用",
                                      "Inventory placement enabled",
                                      "النشر مفعّل في المخزون",
                                    )
                                  : text(
                                      "清单投放停用",
                                      "Inventory placement disabled",
                                      "النشر معطّل في المخزون",
                                    )}
                              </small>
                            </td>
                            <td>
                              <Button
                                onClick={() =>
                                  begin({
                                    kind: "placement",
                                    revision: data.revision,
                                    id: row.id,
                                    model: placement?.modelId ?? "",
                                    groups:
                                      placement?.groupIds.join(", ") ?? "",
                                    enabled: placement?.enabled ?? false,
                                  })
                                }
                              >
                                {text(
                                  "编辑投放",
                                  "Edit placement",
                                  "تحرير النشر",
                                )}
                              </Button>
                            </td>
                          </tr>
                        );
                      })}
                  </tbody>
                </table>
              </div>
              {!rows.length && (
                <p className="ri-empty">
                  {text(
                    "没有匹配资源。",
                    "No matching resources.",
                    "لا توجد موارد مطابقة.",
                  )}
                </p>
              )}
              <nav
                className="ri-pagination"
                aria-label={text(
                  "资源分页",
                  "Inventory pagination",
                  "صفحات المخزون",
                )}
              >
                <Button
                  disabled={currentPage === 0}
                  onClick={() => setPage(currentPage - 1)}
                >
                  {text("上一页", "Previous", "السابق")}
                </Button>
                <span>
                  {currentPage + 1} / {pages} · {rows.length}
                </span>
                <Button
                  disabled={currentPage >= pages - 1}
                  onClick={() => setPage(currentPage + 1)}
                >
                  {text("下一页", "Next", "التالي")}
                </Button>
              </nav>
            </>
          )}
        </>
      )}
      <ResourcePublication
        catalog={data}
        refreshCatalog={() => query.refetch()}
      />
      <Modal
        open={Boolean(draft)}
        onOpenChange={(open) => !open && close()}
        preventOutsideClose
        size="wide"
        title={
          review
            ? text(
                "确认保存资源配置",
                "Confirm inventory save",
                "تأكيد حفظ المخزون",
              )
            : draft?.kind === "placement"
              ? text(
                  "编辑清单投放",
                  "Edit inventory placement",
                  "تحرير نشر المخزون",
                )
              : text(
                  "导入资源快照",
                  "Import inventory snapshot",
                  "استيراد لقطة المخزون",
                )
        }
        description={text(
          "保存到服务器清单。此操作暂不改变实际网关路由。",
          "Save to server inventory. Live gateway routing is unchanged by this operation.",
          "الحفظ في مخزون الخادم؛ لا يغيّر هذا الإجراء توجيه البوابة الفعلي.",
        )}
        footer={
          <>
            <Button disabled={mutation.isPending} onClick={close}>
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            {review && (
              <Button
                disabled={mutation.isPending}
                onClick={() => setReview(false)}
              >
                {text("返回编辑", "Back to editing", "العودة للتحرير")}
              </Button>
            )}
            <Button
              disabled={mutation.isPending || conflict || !data}
              onClick={() => (review ? void save() : preview())}
            >
              {mutation.isPending
                ? text("正在保存…", "Saving…", "جارٍ الحفظ…")
                : review
                  ? text("确认保存", "Confirm save", "تأكيد الحفظ")
                  : text("预览变更", "Review changes", "مراجعة التغييرات")}
            </Button>
          </>
        }
      >
        <div className="ri-editor">
          {draft && (
            <p>
              {text("基于版本", "Base revision", "الإصدار الأساسي")} v
              {draft.revision}
            </p>
          )}
          {conflict && (
            <p role="alert">
              {text(
                "服务器版本已变化，请取消并重新打开编辑。",
                "The server revision changed. Cancel and reopen the editor.",
                "تغير إصدار الخادم. ألغِ التحرير وأعد فتحه.",
              )}
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {draft?.kind === "import" &&
            (!review ? (
              <label>
                JSON
                <textarea
                  dir="ltr"
                  aria-label="Snapshot JSON"
                  value={draft.json}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      json: e.target.value,
                      snapshot: undefined,
                    })
                  }
                />
              </label>
            ) : (
              <>
                <p>
                  {draft.snapshot?.id} ·{" "}
                  {text("来源序号", "Source sequence", "تسلسل المصدر")}{" "}
                  {draft.snapshot?.sequence}
                </p>
                <p>
                  {text(
                    `新增 ${diff?.added}，变化 ${diff?.changed}，移除 ${diff?.removed}。`,
                    `Added ${diff?.added}, changed ${diff?.changed}, removed ${diff?.removed}.`,
                    `إضافة ${diff?.added}، تغيير ${diff?.changed}، إزالة ${diff?.removed}.`,
                  )}
                </p>
                <p>
                  {text(
                    "新资源默认未投放；改绑渠道或模型、下架将暂停原投放；移除会清除投放。",
                    "New resources are unplaced. Channel/model identity changes or retirement disable placement; removal clears it.",
                    "تبدأ الموارد الجديدة بلا نشر. يؤدي تغيير القناة أو النموذج أو سحب المورد إلى تعطيل النشر؛ وتزيله إزالة المورد.",
                  )}
                </p>
              </>
            ))}
          {draft?.kind === "placement" && (
            <>
              <p>
                <code>{draft.id}</code>
              </p>
              {review ? (
                <dl>
                  <dt>
                    {text("标准模型", "Standard model", "النموذج القياسي")}
                  </dt>
                  <dd>{draft.model.trim() || "—"}</dd>
                  <dt>
                    {text("分组标识", "Group identifiers", "معرّفات المجموعات")}
                  </dt>
                  <dd>{draft.groups || "—"}</dd>
                  <dt>{text("清单状态", "Inventory state", "حالة المخزون")}</dt>
                  <dd>{String(draft.enabled)}</dd>
                </dl>
              ) : (
                <>
                  <label>
                    {text(
                      "标准模型标识",
                      "Standard model identifier",
                      "معرّف النموذج القياسي",
                    )}
                    <input
                      value={draft.model}
                      onChange={(e) =>
                        setDraft({ ...draft, model: e.target.value })
                      }
                    />
                  </label>
                  <label>
                    {text(
                      "分组标识（英文逗号分隔）",
                      "Group identifiers (comma-separated)",
                      "معرّفات المجموعات (بفواصل إنجليزية)",
                    )}
                    <input
                      dir="ltr"
                      value={draft.groups}
                      onChange={(e) =>
                        setDraft({ ...draft, groups: e.target.value })
                      }
                    />
                  </label>
                  <label className="ri-checkbox">
                    <input
                      type="checkbox"
                      checked={draft.enabled}
                      onChange={(e) =>
                        setDraft({ ...draft, enabled: e.target.checked })
                      }
                    />
                    {text(
                      "启用清单投放",
                      "Enable inventory placement",
                      "تفعيل النشر في المخزون",
                    )}
                  </label>
                </>
              )}
            </>
          )}
        </div>
      </Modal>
    </div>
  );
}
