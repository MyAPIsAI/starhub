import { useState } from "react";
import { useModelPricing } from "../lib/model-pricing";
import {
  applyResourceSnapshot,
  modelCatalog,
  materializeAvailability,
} from "../lib/platform-catalog";
import type {
  PricingCatalogConfig,
  ResourcePlacement,
  ResourceSnapshot,
} from "../config/models";
import { useProductPreview } from "../lib/product-preview";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { DemoControlBar } from "../components/demo-control-bar";
import "../components/catalog-management.css";
import { ChannelWorkspace } from "./channel-workspace";
import {
  ChannelSourceControls,
  RemovedChannelResources,
} from "./channel-source-controls";

type Edit = { id: string; placement: ResourcePlacement; revision: string };
export function Channels({ role }: { role: number }) {
  const config = useModelPricing((s) => s.config),
    save = useModelPricing((s) => s.save),
    keys = useProductPreview((s) => s.keys);
  const snapshot = config.resources!;
  const [notice, setNotice] = useState("");
  const [removedOpen, setRemovedOpen] = useState(false);
  const [candidate, setCandidate] = useState<ResourceSnapshot | null>(null),
    [syncRevision, setSyncRevision] = useState("");
  const [editing, setEditing] = useState<Edit | null>(null),
    [review, setReview] = useState(false),
    [importOpen, setImportOpen] = useState(false),
    [json, setJson] = useState("");
  const publish = (next: PricingCatalogConfig, revision: string) => {
    const result = save(materializeAvailability(next), role, revision);
    setNotice(
      result.ok
        ? "已保存到当前演示。新请求使用最新资源配置。"
        : result.reason === "conflict"
          ? "配置版本已变化，请重新载入后再保存。"
          : "未保存，字段或权限无效。",
    );
    return result.ok;
  };
  const preview = (source: ResourceSnapshot) => {
    try {
      applyResourceSnapshot(config, source);
      setCandidate(source);
      setSyncRevision(config.revision);
      setNotice("");
      setImportOpen(false);
    } catch (e) {
      setNotice((e as Error).message);
    }
  };
  const open = (id: string) => {
    setNotice("");
    setReview(false);
    setEditing({
      id,
      placement: structuredClone(
        config.placements?.[id] ?? {
          modelId: null,
          groupIds: [],
          enabled: false,
        },
      ),
      revision: config.revision,
    });
  };
  const patch = (data: Partial<ResourcePlacement>) =>
    setEditing((e) => e && { ...e, placement: { ...e.placement, ...data } });
  const validate = () => {
    if (editing?.placement.enabled && !editing.placement.modelId) {
      setNotice("请先映射标准模型，再启用投放。");
      return false;
    }
    setNotice("");
    return true;
  };
  const resource = snapshot.instances.find((r) => r.id === editing?.id);
  const current = editing && config.placements?.[editing.id];
  const diff = candidate
    ? {
        added: candidate.instances.filter(
          (r) => !snapshot.instances.some((old) => old.id === r.id),
        ),
        removed: snapshot.instances.filter(
          (r) => !candidate.instances.some((next) => next.id === r.id),
        ),
        changed: candidate.instances.filter((r) =>
          snapshot.instances.some(
            (old) =>
              old.id === r.id && JSON.stringify(old) !== JSON.stringify(r),
          ),
        ),
      }
    : null;
  const missing = Object.keys(config.placements ?? {}).filter(
    (id) => !snapshot.instances.some((r) => r.id === id),
  );
  return (
    <div className="catalog-management channel-management">
      <ChannelSourceControls
        config={config}
        onPreview={preview}
        onFailure={setNotice}
        onImport={() => {
          setJson(
            JSON.stringify(
              {
                ...snapshot,
                sequence: snapshot.sequence + 1,
                id: `resource-import-${snapshot.sequence + 1}`,
                publishedAt: new Date().toISOString(),
              },
              null,
              2,
            ),
          );
          setImportOpen(true);
        }}
      />
      {notice && (
        <p role="status" className="pricing-notice">
          {notice}
        </p>
      )}
      <ChannelWorkspace
        config={config}
        onEdit={open}
        onRemoved={() => setRemovedOpen(true)}
        removedCount={missing.length}
      />
      <div className="channel-demo-footer">
        <DemoControlBar inline />
      </div>
      <RemovedChannelResources
        config={config}
        open={removedOpen}
        onOpenChange={setRemovedOpen}
        onEdit={open}
      />
      <Modal
        open={!!candidate}
        onOpenChange={(open) => !open && setCandidate(null)}
        title="确认资源快照同步"
        description="同步只更新来源数据，不自动改变已有模型映射和投放权限。新资源默认未映射、未投放。同一实例的上游模型身份变化会暂停投放，需重新确认。"
        size="wide"
        footer={
          <>
            <Button onClick={() => setCandidate(null)}>取消</Button>
            <Button
              variant="primary"
              onClick={() => {
                try {
                  if (
                    candidate &&
                    publish(
                      applyResourceSnapshot(config, candidate),
                      syncRevision,
                    )
                  )
                    setCandidate(null);
                } catch (e) {
                  setNotice((e as Error).message);
                }
              }}
            >
              确认同步快照
            </Button>
          </>
        }
      >
        {candidate && diff && (
          <div className="cm-stack">
            <p>
              <code>{snapshot.id}</code> → <code>{candidate.id}</code>
            </p>
            <p>
              新增 {diff.added.length} 个实例，变化 {diff.changed.length}{" "}
              个，移除 {diff.removed.length} 个；渠道 {snapshot.channels.length}{" "}
              → {candidate.channels.length} 个。
            </p>
            {diff.added.map((r) => (
              <p key={r.id}>
                新增：{r.upstreamModel} / {r.id} · 待手动映射与投放
              </p>
            ))}
            {diff.changed.map((r) => (
              <p key={r.id}>
                更新：{r.id} · 来源状态{" "}
                {
                  { available: "可用", unavailable: "不可用", retired: "下架" }[
                    r.status
                  ]
                }
              </p>
            ))}
            {diff.removed.map((r) => (
              <p key={r.id}>移除：{r.id} · 后续请求不会使用</p>
            ))}
            <p>
              来源不可用或移除时，只能在同一标准模型的其他合格实例中选择；没有可用实例则拒绝请求且不计费。
            </p>
          </div>
        )}
        {notice && <p role="alert">{notice}</p>}
      </Modal>
      <Modal
        open={!!editing}
        onOpenChange={(open) => !open && setEditing(null)}
        title={review ? "确认映射与投放影响" : "资源实例 · 映射与投放"}
        description="上游来源字段只读。本地投放可选择多个分组，不修改上游资源。"
        size="wide"
        footer={
          review ? (
            <>
              <Button onClick={() => setReview(false)}>返回编辑</Button>
              <Button
                variant="primary"
                onClick={() => {
                  if (!editing || !validate()) return;
                  const next = structuredClone(config);
                  if (resource)
                    next.placements![editing.id] = editing.placement;
                  else delete next.placements![editing.id];
                  if (publish(next, editing.revision)) setEditing(null);
                }}
              >
                确认发布投放
              </Button>
            </>
          ) : (
            <>
              <Button onClick={() => setEditing(null)}>取消</Button>
              <Button
                variant="primary"
                onClick={() => validate() && setReview(true)}
              >
                预览投放影响
              </Button>
            </>
          )
        }
      >
        {editing &&
          (review ? (
            <div className="cm-stack">
              <p>
                资源 <code>{editing.id}</code>
              </p>
              <p>
                标准模型：{current?.modelId ?? "未映射"} →{" "}
                {resource
                  ? (editing.placement.modelId ?? "未映射")
                  : "解除映射"}
              </p>
              <p>
                投放分组：
                {current?.groupIds
                  .map((id) => config.groups[id]?.name)
                  .join("、") || "无"}{" "}
                →{" "}
                {resource
                  ? editing.placement.groupIds
                      .map((id) => config.groups[id]?.name)
                      .join("、") || "无"
                  : "清除全部投放"}
              </p>
              <p>
                新状态：
                {resource && editing.placement.enabled
                  ? "启用"
                  : "停用"}；关联{" "}
                {
                  keys.filter((k) =>
                    [
                      ...(current?.groupIds ?? []),
                      ...editing.placement.groupIds,
                    ].includes(k.tier),
                  ).length
                }{" "}
                把密钥所在分组。密钥白名单和额度仍然生效；历史请求保留原资源和原费率。
              </p>
            </div>
          ) : (
            <div className="cm-stack">
              <dl className="cm-details">
                <div>
                  <dt>外部资源标识</dt>
                  <dd>{editing.id}</dd>
                </div>
                <div>
                  <dt>上游模型标识</dt>
                  <dd>{resource?.upstreamModel ?? "已不在来源快照"}</dd>
                </div>
                <div>
                  <dt>来源标准模型提示</dt>
                  <dd>{resource?.canonicalModel ?? "—"}</dd>
                </div>
                <div>
                  <dt>来源协议 / 上下文</dt>
                  <dd>
                    {resource?.protocols.join(" / ") || "媒体计费"} ·{" "}
                    {resource?.contextLength ?? 0} 词元
                  </dd>
                </div>
              </dl>
              {resource ? (
                <div className="cm-form">
                  <label className="cm-wide">
                    映射标准模型
                    <select
                      aria-label="映射标准模型"
                      value={editing.placement.modelId ?? ""}
                      onChange={(e) =>
                        patch({ modelId: e.target.value || null })
                      }
                    >
                      <option value="">暂不映射</option>
                      {modelCatalog(config).map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.name} · {m.id}
                        </option>
                      ))}
                    </select>
                  </label>
                  <p className="pricing-help cm-wide">
                    请确保映射的是同一基础模型。来源提示不自动创建身份；没有对应标准模型时，先在“标准模型”页创建。调用时进一步校验协议、能力与上下文。
                  </p>
                  <fieldset className="cm-wide">
                    <legend>投放到分组（可多选）</legend>
                    {Object.entries(config.groups).map(([id, g]) => (
                      <label className="cm-check" key={id}>
                        <input
                          type="checkbox"
                          checked={editing.placement.groupIds.includes(id)}
                          onChange={(e) =>
                            patch({
                              groupIds: e.target.checked
                                ? [...editing.placement.groupIds, id]
                                : editing.placement.groupIds.filter(
                                    (old) => old !== id,
                                  ),
                            })
                          }
                        />
                        {g.name} · {g.visibility === "custom" ? "定制" : "通用"}{" "}
                        · {config.multipliers[id]}× {!g.enabled && "（已停用）"}
                      </label>
                    ))}
                  </fieldset>
                  <label className="cm-check">
                    <input
                      type="checkbox"
                      checked={editing.placement.enabled}
                      onChange={(e) => patch({ enabled: e.target.checked })}
                    />
                    启用本地投放
                  </label>
                </div>
              ) : (
                <p>
                  实例已移出快照。确认后清除本地映射和全部投放，历史请求不变。
                </p>
              )}
            </div>
          ))}
        {notice && <p role="alert">{notice}</p>}
      </Modal>
      <Modal
        open={importOpen}
        onOpenChange={setImportOpen}
        title="导入资源快照"
        description="用于演示外部数据契约；不连接真实资源平台。序号须递增，标识、协议、能力和引用均经过校验。"
        size="wide"
        footer={
          <>
            <Button onClick={() => setImportOpen(false)}>取消</Button>
            <Button
              variant="primary"
              onClick={() => {
                try {
                  preview(JSON.parse(json));
                } catch (e) {
                  setNotice("无法解析快照：" + (e as Error).message);
                }
              }}
            >
              校验并预览同步
            </Button>
          </>
        }
      >
        <div className="cm-form">
          <label className="cm-wide">
            快照 JSON（结构化数据）
            <textarea
              aria-label="资源快照数据"
              style={{ minHeight: 320, fontFamily: "monospace" }}
              value={json}
              onChange={(e) => setJson(e.target.value)}
            />
          </label>
        </div>
        {notice && <p role="alert">{notice}</p>}
      </Modal>
    </div>
  );
}
