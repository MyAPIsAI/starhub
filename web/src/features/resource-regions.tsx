import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useResourceCatalog, useRegionMutation } from "../lib/api/resource";
import { ROLE_ADMIN, ROLE_ROOT, useSystemAccess } from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import "./resource-regions.css";

type Draft = {
  revision: number;
  id: string;
  name: string;
  description: string;
  locked: boolean;
  remove: boolean;
};

export function ResourceRegions() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const query = useResourceCatalog();
  const mutation = useRegionMutation();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const { hash } = useLocation();
  const section = useRef<HTMLElement>(null);
  useEffect(() => {
    if (hash !== "#regions" || (role !== ROLE_ADMIN && role !== ROLE_ROOT))
      return;
    const frame = requestAnimationFrame(() => {
      section.current?.scrollIntoView({ block: "start" });
      section.current?.focus({ preventScroll: true });
    });
    return () => cancelAnimationFrame(frame);
  }, [hash, role, query.isLoading]);
  if (role !== ROLE_ADMIN && role !== ROLE_ROOT) return null;
  const data = query.error ? undefined : query.data;
  const counts = new Map<string, number>();
  for (const resource of data?.snapshot?.instances ?? [])
    counts.set(resource.region, (counts.get(resource.region) ?? 0) + 1);
  const rows = [
    ...new Set([...Object.keys(data?.regions ?? {}), ...counts.keys()]),
  ]
    .sort()
    .filter((id) =>
      `${id} ${data?.regions[id]?.name ?? ""}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    );
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const conflict = Boolean(draft && data && draft.revision !== data.revision);
  const begin = (id: string, remove = false) => {
    if (!data) return;
    const region = data.regions[id];
    mutation.reset();
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      revision: data.revision,
      id,
      name: region?.name ?? "",
      description: region?.description ?? "",
      locked: Boolean(id),
      remove,
    });
  };
  const close = () => {
    if (!mutation.isPending) {
      setDraft(null);
      mutation.reset();
      setError("");
    }
  };
  const save = async () => {
    if (!draft || !data || conflict || !review) return;
    try {
      const result = await mutation.mutateAsync(
        draft.remove
          ? {
              operation: "delete",
              id: draft.id,
              expected_revision: draft.revision,
            }
          : {
              operation: "save",
              id: draft.id.trim(),
              expected_revision: draft.revision,
              name: draft.name.trim(),
              description: draft.description.trim(),
            },
      );
      setDraft(null);
      setNotice(
        text(
          `区域目录已保存，配置版本 v${result.revision}。`,
          `Region directory saved at v${result.revision}.`,
          `حُفظ دليل المناطق بالإصدار v${result.revision}.`,
        ),
      );
    } catch {
      setError(
        text(
          "无法确认保存结果。请刷新核对；版本变化后须重新打开编辑。被资源引用的区域不能删除。",
          "Save outcome is unconfirmed. Refresh to check and reopen after a revision change. Referenced regions cannot be deleted.",
          "تعذر تأكيد الحفظ. حدّث للتحقق وأعد فتح المحرر عند تغير الإصدار. لا يمكن حذف منطقة مستخدمة.",
        ),
      );
    }
  };
  return (
    <section
      className="resource-regions"
      id="regions"
      ref={section}
      tabIndex={-1}
      aria-labelledby="region-heading"
    >
      <header>
        <div>
          <h2 id="region-heading">
            {text(
              "资源区域目录",
              "Resource region directory",
              "دليل مناطق الموارد",
            )}
          </h2>
          <p>
            {text(
              "维护区域名称与说明，引用数量仅统计当前资源快照。此目录不代表网关部署位置，也不启停流量。",
              "Manage region names and descriptions. Reference counts cover the current inventory snapshot. This directory neither represents gateway deployment locations nor switches traffic.",
              "أدر أسماء المناطق وأوصافها. الأعداد تخص لقطة المخزون الحالية؛ لا يمثل الدليل مواقع نشر البوابة ولا يتحكم بحركة الطلبات.",
            )}
          </p>
        </div>
        <div className="rr-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text("刷新区域", "Refresh regions", "تحديث المناطق")}
          </Button>
          <Button disabled={!data} onClick={() => begin("")}>
            {text("新增区域", "Add region", "إضافة منطقة")}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text("正在读取区域…", "Loading regions…", "جارٍ تحميل المناطق…")}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            "无法读取区域目录，请刷新重试。",
            "Region directory is unavailable. Refresh to retry.",
            "دليل المناطق غير متاح. حدّث لإعادة المحاولة.",
          )}
        </p>
      )}
      {notice && (
        <p role="status" className="rr-notice">
          {notice}
        </p>
      )}
      {data && (
        <>
          <label className="rr-search">
            {text("搜索区域", "Search regions", "بحث المناطق")}
            <input
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(0);
              }}
            />
          </label>
          <p>
            v{data.revision} ·{" "}
            {text(
              "区域定义与来源观测分别保留，未登记标签不会自动创建定义。",
              "Definitions and source observations stay separate; observed labels never create definitions automatically.",
              "تبقى التعريفات منفصلة عن رصد المصدر؛ لا تُنشأ تعريفات تلقائيًا للتسميات المرصودة.",
            )}
          </p>
          {!rows.length ? (
            <p>{text("暂无区域。", "No regions yet.", "لا توجد مناطق بعد.")}</p>
          ) : (
            <>
              <div
                className="rr-table"
                role="region"
                tabIndex={0}
                aria-label={text(
                  "区域目录列表",
                  "Region directory table",
                  "جدول دليل المناطق",
                )}
              >
                <table>
                  <thead>
                    <tr>
                      {[
                        text(
                          "区域标识 / 名称",
                          "Region ID / name",
                          "معرّف المنطقة / الاسم",
                        ),
                        text("说明", "Description", "الوصف"),
                        text(
                          "资源引用",
                          "Inventory references",
                          "مراجع المخزون",
                        ),
                        text("操作", "Actions", "الإجراءات"),
                      ].map((label) => (
                        <th key={label}>{label}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows
                      .slice(currentPage * 20, currentPage * 20 + 20)
                      .map((id) => {
                        const region = data.regions[id];
                        const count = counts.get(id) ?? 0;
                        return (
                          <tr key={id} data-region-id={id}>
                            <td>
                              <strong>
                                {region?.name ??
                                  (id
                                    ? text(
                                        "未登记区域",
                                        "Unregistered region",
                                        "منطقة غير مسجلة",
                                      )
                                    : text(
                                        "区域未知",
                                        "Unknown region",
                                        "منطقة غير معروفة",
                                      ))}
                              </strong>
                              <code>{id || "—"}</code>
                            </td>
                            <td>{region?.description || "—"}</td>
                            <td>{count}</td>
                            <td>
                              {id && (
                                <div className="rr-actions">
                                  <Button onClick={() => begin(id)}>
                                    {region
                                      ? text(
                                          "编辑区域",
                                          "Edit region",
                                          "تحرير المنطقة",
                                        )
                                      : text(
                                          "登记区域",
                                          "Register region",
                                          "تسجيل المنطقة",
                                        )}
                                  </Button>
                                  {region && (
                                    <Button
                                      disabled={count > 0}
                                      title={
                                        count > 0
                                          ? text(
                                              "当前资源仍引用此区域",
                                              "Current inventory references this region",
                                              "يستخدم المخزون الحالي هذه المنطقة",
                                            )
                                          : undefined
                                      }
                                      onClick={() => begin(id, true)}
                                    >
                                      {text(
                                        "删除区域",
                                        "Delete region",
                                        "حذف المنطقة",
                                      )}
                                    </Button>
                                  )}
                                </div>
                              )}
                            </td>
                          </tr>
                        );
                      })}
                  </tbody>
                </table>
              </div>
              <nav
                className="rr-pagination"
                aria-label={text(
                  "区域分页",
                  "Region pagination",
                  "صفحات المناطق",
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
      <Modal
        open={Boolean(draft)}
        onOpenChange={(open) => !open && close()}
        preventOutsideClose
        title={
          draft?.remove
            ? text(
                "确认删除区域定义",
                "Confirm region deletion",
                "تأكيد حذف المنطقة",
              )
            : review
              ? text(
                  "确认区域信息",
                  "Confirm region details",
                  "تأكيد تفاصيل المنطقة",
                )
              : text(
                  "维护区域定义",
                  "Edit region definition",
                  "تحرير تعريف المنطقة",
                )
        }
        description={text(
          "仅修改资源区域目录，实际网关路由与调用授权不变。",
          "Only the inventory region directory changes. Gateway routing and API authorization remain unchanged.",
          "يتغير دليل مناطق المخزون فقط، دون تغيير توجيه البوابة أو صلاحيات الواجهة.",
        )}
        footer={
          <>
            <Button disabled={mutation.isPending} onClick={close}>
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            {review && !draft?.remove && (
              <Button
                disabled={mutation.isPending}
                onClick={() => setReview(false)}
              >
                {text("返回编辑", "Back to editing", "العودة للتحرير")}
              </Button>
            )}
            <Button
              disabled={!data || conflict || mutation.isPending}
              onClick={() => {
                if (review) {
                  void save();
                  return;
                }
                if (!draft?.id.trim() || !draft.name.trim()) {
                  setError(
                    text(
                      "请填写区域标识和名称。",
                      "Enter a region identifier and name.",
                      "أدخل معرّف المنطقة واسمها.",
                    ),
                  );
                  return;
                }
                setError("");
                setReview(true);
              }}
            >
              {mutation.isPending
                ? text("正在保存…", "Saving…", "جارٍ الحفظ…")
                : review
                  ? draft?.remove
                    ? text("确认删除", "Confirm delete", "تأكيد الحذف")
                    : text("确认保存", "Confirm save", "تأكيد الحفظ")
                  : text("预览变更", "Review changes", "مراجعة التغييرات")}
            </Button>
          </>
        }
      >
        <div className="rr-editor">
          {conflict && (
            <p role="alert">
              {text(
                "目录版本已变化，请取消并重新打开。",
                "The directory revision changed. Cancel and reopen.",
                "تغير إصدار الدليل. ألغِ التحرير وأعد فتحه.",
              )}
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {draft &&
            (review ? (
              <>
                <p>
                  <strong>{draft.name}</strong> · <code>{draft.id}</code> · v
                  {draft.revision}
                </p>
                <p>{draft.description || "—"}</p>
                <p>
                  {text(
                    `当前快照引用 ${counts.get(draft.id) ?? 0} 个资源。`,
                    `Referenced by ${counts.get(draft.id) ?? 0} resources in the current snapshot.`,
                    `تستخدمها ${counts.get(draft.id) ?? 0} موارد في اللقطة الحالية.`,
                  )}
                </p>
              </>
            ) : (
              <>
                <label>
                  {text("区域标识", "Region identifier", "معرّف المنطقة")}
                  <input
                    value={draft.id}
                    readOnly={draft.locked}
                    onChange={(e) => setDraft({ ...draft, id: e.target.value })}
                  />
                </label>
                <label>
                  {text("显示名称", "Display name", "الاسم المعروض")}
                  <input
                    value={draft.name}
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                </label>
                <label>
                  {text("区域说明", "Region description", "وصف المنطقة")}
                  <textarea
                    rows={3}
                    value={draft.description}
                    onChange={(e) =>
                      setDraft({ ...draft, description: e.target.value })
                    }
                  />
                </label>
              </>
            ))}
        </div>
      </Modal>
    </section>
  );
}
