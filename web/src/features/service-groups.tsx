import { useState } from "react";
import { Link } from "react-router-dom";
import { Layers3, Plus, Search } from "lucide-react";
import { useModelPricing, isValidMultiplier } from "../lib/model-pricing";
import {
  demoAccounts as seedAccounts,
  accountCanUseGroup,
  groupResources,
  materializeAvailability,
} from "../lib/platform-catalog";
import { removeServiceGroup } from "../lib/catalog-mutations";
import { accountDisplayName } from "../lib/account-identity";
import { useProductPreview } from "../lib/product-preview";
import type {
  ServiceGroupConfig,
  PricingCatalogConfig,
} from "../config/models";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import "../components/catalog-management.css";

type Draft = {
  id: string;
  group: ServiceGroupConfig;
  multiplier: string;
  fresh: boolean;
  revision: string;
};
export function ServiceGroups({ role }: { role: number }) {
  const user = useProductPreview((s) => s.user);
  const demoAccounts = seedAccounts.some((account) => account.id === user.id)
    ? seedAccounts.map((account) =>
        account.id === user.id
          ? { ...account, name: accountDisplayName(user) }
          : account,
      )
    : [...seedAccounts, { id: user.id, name: accountDisplayName(user) }];
  const config = useModelPricing((s) => s.config),
    save = useModelPricing((s) => s.save);
  const keys = useProductPreview((s) => s.keys),
    requests = useProductPreview((s) => s.requests);
  const [query, setQuery] = useState(""),
    [filter, setFilter] = useState("all"),
    [account, setAccount] = useState("account-demo");
  const [editing, setEditing] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [removing, setRemoving] = useState<string | null>(null),
    [notice, setNotice] = useState("");
  const open = (id?: string) => {
    setNotice("");
    setReview(false);
    setEditing({
      id: id ?? "",
      group: id
        ? structuredClone(config.groups[id])
        : {
            name: "",
            description: "",
            enabled: true,
            visibility: "common",
            accountIds: [],
          },
      multiplier: id ? config.multipliers[id] : "1",
      fresh: !id,
      revision: config.revision,
    });
  };
  const patch = (update: Partial<ServiceGroupConfig>) =>
    setEditing((e) => e && { ...e, group: { ...e.group, ...update } });
  const publish = (next: PricingCatalogConfig, revision = config.revision) => {
    const result = save(materializeAvailability(next), role, revision);
    setNotice(
      result.ok
        ? "分组已更新，后续请求使用新设置。"
        : result.reason === "conflict"
          ? "配置已被更新，请关闭编辑后重试。"
          : "未保存，请检查字段、权限或浏览器存储。",
    );
    return result.ok;
  };
  const validate = () => {
    if (!editing) return false;
    if (
      !/^[a-z0-9][a-z0-9_-]{1,59}$/.test(editing.id) ||
      !editing.group.name.trim() ||
      !isValidMultiplier(editing.multiplier)
    ) {
      setNotice("请填写有效的名称、分组标识和倍率（大于 0，不超过 100）。");
      return false;
    }
    if (editing.fresh && config.groups[editing.id]) {
      setNotice("分组标识已存在。");
      return false;
    }
    if (
      editing.group.visibility === "custom" &&
      !editing.group.accountIds?.length
    ) {
      setNotice("定制分组至少指定一个账号。");
      return false;
    }
    setNotice("");
    return true;
  };
  const submit = () => {
    if (!editing || !validate()) return;
    const next = structuredClone(config);
    next.groups[editing.id] = editing.group;
    next.multipliers[editing.id] = editing.multiplier;
    if (publish(next, editing.revision)) {
      setEditing(null);
      setReview(false);
    }
  };
  const rows = Object.entries(config.groups).filter(
    ([id, g]) =>
      `${id} ${g.name}`.toLowerCase().includes(query.toLowerCase()) &&
      (filter === "all" || g.visibility === filter),
  );
  return (
    <div className="catalog-management">
      <header className="cm-heading">
        <div>
          <span className="pricing-eyebrow">
            <Layers3 size={16} />
            系统设置
          </span>
          <h1>服务分组</h1>
          <p>
            管理销售范围和计费倍率。资源实例在哪些分组销售，请到渠道管理中设置。
          </p>
        </div>
        <Button variant="primary" onClick={() => open()}>
          <Plus size={16} />
          创建分组
        </Button>
      </header>
      <div className="cm-callout">
        <strong>标准模型基准价 × 分组倍率 = 对客价格</strong>
        <p>
          通用分组对所有账号可见可选；定制分组仅对指定账号可见可选。既有密钥也会校验当前账号的分组权限。停用或撤销授权后，后续请求明确失败且不计费。
        </p>
      </div>
      {notice && (
        <p role="status" className="pricing-notice">
          {notice}
        </p>
      )}
      <div className="cm-toolbar">
        <label className="cm-search">
          <Search size={17} />
          <input
            aria-label="搜索服务分组"
            placeholder="搜索分组名称或标识"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <select
          aria-label="筛选分组类型"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        >
          <option value="all">全部类型</option>
          <option value="common">通用分组</option>
          <option value="custom">定制分组</option>
        </select>
      </div>
      <div className="cm-table">
        <table>
          <thead>
            <tr>
              <th>分组</th>
              <th>类型 / 账号范围</th>
              <th>倍率</th>
              <th>资源投放概况</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(([id, g]) => {
              const resources = groupResources(config, id, undefined, false);
              const models = new Set(
                resources.map((r) => config.placements?.[r.id]?.modelId),
              );
              return (
                <tr key={id}>
                  <td>
                    <strong>{g.name}</strong>
                    <code>{id}</code>
                    <small>{g.description}</small>
                  </td>
                  <td>
                    {g.visibility === "custom" ? "定制分组" : "通用分组"}
                    <small>
                      {g.visibility === "custom"
                        ? g.accountIds
                            ?.map(
                              (a) =>
                                demoAccounts.find((item) => item.id === a)
                                  ?.name ?? a,
                            )
                            .join("、")
                        : "所有账号"}
                    </small>
                  </td>
                  <td>{config.multipliers[id]}×</td>
                  <td>
                    <Link
                      to={`/console/channels?group=${encodeURIComponent(id)}`}
                    >
                      {resources.length} 个实例 · {models.size} 个模型
                    </Link>
                    <small>由实例投放自动汇总</small>
                  </td>
                  <td>
                    {g.enabled ? "启用中" : "已停用"}
                    <small>
                      {keys.filter((k) => k.tier === id).length} 把密钥
                    </small>
                  </td>
                  <td>
                    <div className="cm-actions">
                      <Button size="small" onClick={() => open(id)}>
                        编辑
                      </Button>
                      <Button
                        size="small"
                        onClick={() => {
                          setNotice("");
                          setRemoving(id);
                        }}
                      >
                        删除
                      </Button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {!rows.length && <p className="cm-empty">未找到分组。</p>}
      </div>
      <section className="cm-callout">
        <div className="cm-actions">
          <strong>账号可见范围预览</strong>
          <select
            aria-label="预览账号"
            value={account}
            onChange={(e) => setAccount(e.target.value)}
          >
            {demoAccounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </div>
        <p>
          {Object.entries(config.groups)
            .filter(([id]) => accountCanUseGroup(config, id, account))
            .map(([, g]) => g.name)
            .join("、") || "此账号没有可用分组"}
        </p>
        <small>
          此处仅预览授权目录，不切换当前账号、钱包或密钥；账号清单为本地演示数据。
        </small>
      </section>
      <Modal
        open={!!editing}
        onOpenChange={(open) => {
          if (!open) {
            setEditing(null);
            setReview(false);
          }
        }}
        title={
          review
            ? "确认分组影响"
            : editing?.fresh
              ? "创建服务分组"
              : "编辑服务分组"
        }
        description={
          review
            ? "确认后立即应用于新请求，历史费率保持不变。"
            : "分组不直接维护模型列表；资源由渠道实例投放。"
        }
        size="wide"
        footer={
          review ? (
            <>
              <Button onClick={() => setReview(false)}>返回编辑</Button>
              <Button variant="primary" onClick={submit}>
                确认发布分组
              </Button>
            </>
          ) : (
            <>
              <Button onClick={() => setEditing(null)}>取消</Button>
              <Button
                variant="primary"
                onClick={() => validate() && setReview(true)}
              >
                预览影响
              </Button>
            </>
          )
        }
      >
        {editing &&
          (review ? (
            <div className="cm-stack">
              <h3>
                {editing.group.name} · {editing.id}
              </h3>
              <p>
                倍率：
                {editing.fresh
                  ? "未创建"
                  : config.multipliers[editing.id] + "×"}{" "}
                → {editing.multiplier}×；状态：
                {editing.group.enabled ? "启用" : "停用"}
              </p>
              <p>
                账号范围：
                {editing.group.visibility === "common"
                  ? "所有账号"
                  : editing.group.accountIds
                      ?.map(
                        (a) =>
                          demoAccounts.find((item) => item.id === a)?.name ?? a,
                      )
                      .join("、")}
              </p>
              <p>
                影响{" "}
                {keys.filter((k) => k.tier === editing.id && k.active).length}{" "}
                把启用密钥；新增分组没有实例投放时不能调用模型。历史调用和密钥自身白名单保持原值。
              </p>
              <p>
                价格示例：模型基准费用为 $1 时，当前分组费用为 $
                {editing.multiplier}
                。分时和条件计价先计算模型基础费用，再应用一次分组倍率。
              </p>
            </div>
          ) : (
            <div className="cm-form">
              <label>
                分组名称
                <input
                  aria-label="分组名称"
                  value={editing.group.name}
                  maxLength={60}
                  onChange={(e) => patch({ name: e.target.value })}
                />
              </label>
              <label>
                分组标识
                <input
                  aria-label="分组标识"
                  disabled={!editing.fresh}
                  value={editing.id}
                  onChange={(e) =>
                    setEditing({ ...editing, id: e.target.value })
                  }
                  placeholder="例如：account-a"
                />
              </label>
              <label>
                分组倍率
                <input
                  aria-label="分组倍率"
                  inputMode="decimal"
                  value={editing.multiplier}
                  onChange={(e) =>
                    setEditing({ ...editing, multiplier: e.target.value })
                  }
                />
              </label>
              <label>
                分组类型
                <select
                  aria-label="分组类型"
                  value={editing.group.visibility}
                  onChange={(e) =>
                    patch({ visibility: e.target.value as "common" | "custom" })
                  }
                >
                  <option value="common">通用分组 · 所有账号</option>
                  <option value="custom">定制分组 · 指定账号</option>
                </select>
              </label>
              {editing.group.visibility === "custom" && (
                <fieldset className="cm-wide">
                  <legend>指定账号</legend>
                  {demoAccounts.map((a) => (
                    <label className="cm-check" key={a.id}>
                      <input
                        type="checkbox"
                        checked={
                          editing.group.accountIds?.includes(a.id) ?? false
                        }
                        onChange={(e) =>
                          patch({
                            accountIds: e.target.checked
                              ? [...(editing.group.accountIds ?? []), a.id]
                              : editing.group.accountIds?.filter(
                                  (id) => id !== a.id,
                                ),
                          })
                        }
                      />
                      {a.name}
                      <code>{a.id}</code>
                    </label>
                  ))}
                </fieldset>
              )}
              <label className="cm-wide">
                分组说明
                <textarea
                  aria-label="分组说明"
                  maxLength={500}
                  value={editing.group.description}
                  onChange={(e) => patch({ description: e.target.value })}
                />
              </label>
              <label className="cm-check">
                <input
                  type="checkbox"
                  checked={editing.group.enabled}
                  onChange={(e) => patch({ enabled: e.target.checked })}
                />
                启用分组
              </label>
            </div>
          ))}
        {notice && (
          <p role="alert" className="settings-error">
            {notice}
          </p>
        )}
      </Modal>
      <Modal
        open={!!removing}
        onOpenChange={(open) => !open && setRemoving(null)}
        title="删除服务分组"
        description="存在密钥、历史调用或资源投放引用时拒绝删除；可以先停用分组。"
        footer={
          <>
            <Button onClick={() => setRemoving(null)}>取消</Button>
            <Button
              variant="primary"
              onClick={() => {
                try {
                  if (
                    removing &&
                    publish(
                      removeServiceGroup(config, removing, keys, requests),
                    )
                  )
                    setRemoving(null);
                } catch (e) {
                  setNotice((e as Error).message);
                }
              }}
            >
              确认删除
            </Button>
          </>
        }
      >
        <p>{removing && config.groups[removing]?.name}</p>
        {notice && <p role="alert">{notice}</p>}
      </Modal>
    </div>
  );
}
