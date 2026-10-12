import { useState } from "react";
import { Link } from "react-router-dom";
import { Route } from "lucide-react";
import { useModelPricing } from "../lib/model-pricing";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useBackendRequests } from "../lib/api/usage";
import { getKeyModels, resolveKeyModelResult } from "../lib/key-model-policy";
import { groupResources } from "../lib/platform-catalog";
import { useSystemAccess, isAdmin } from "../lib/system-access";
import { Button } from "../components/ui/button";
import "../components/catalog-management.css";
export default function ResourceRouting() {
  const config = useModelPricing((s) => s.config),
    keys = useBackendKeys().keys,
    requests = useBackendRequests(20).rows as any[],
    role = useSystemAccess((s) => s.role);
  const [keyId, setKeyId] = useState("");
  const key = keys.find((k) => k.id === keyId) ?? keys.find((k) => k.active);
  const models = key ? getKeyModels(key, config) : [];
  const resolved = key
    ? resolveKeyModelResult(key, "veyra-auto", config, {
        protocol: "OpenAI",
        capability: "text",
        inputTokens: 1000,
        maxTokens: 500,
      })
    : null;
  return (
    <div className="catalog-management">
      <header className="cm-heading">
        <div>
          <span className="pricing-eyebrow">
            <Route size={16} />
            智能路由
          </span>
          <h1>路由候选与调用追踪</h1>
          <p>
            请求模型为
            veyra-auto（智能路由）时，在分组、账号、密钥白名单与能力约束内选取实际模型；再选择该标准模型的可用资源实例。
          </p>
        </div>
        <Button asChild>
          <Link to="/console/playground">验证一次请求</Link>
        </Button>
      </header>
      <section className="cm-callout">
        <label className="cm-actions">
          查看密钥候选
          <select
            aria-label="查看路由密钥"
            value={key?.id ?? ""}
            onChange={(e) => setKeyId(e.target.value)}
          >
            {keys.map((k) => (
              <option key={k.id} value={k.id}>
                {k.name}
                {!k.active ? "（已停用）" : ""}
              </option>
            ))}
          </select>
        </label>
        <p>
          估算条件：兼容 OpenAI（开放接口）文本协议、输入 1,000 词元、最大输出
          500 词元。优先成本策略按当前计价规则估算，实际用量决定最终费用。
        </p>
        <p>
          {!key
            ? "请先创建密钥。"
            : resolved?.model
              ? `当前候选选择：${resolved.model.name}；仅为只读估算，没有发起请求或扣费。`
              : `不可路由：${resolved?.error ?? "无候选"}。请检查分组授权、智能入口权限或资源状态。`}
        </p>
        <small>
          未启用的密钥、余额或额度不足仍会在请求执行时拒绝；此预览不代表接通上游。
        </small>
      </section>
      <div className="cm-table">
        <table>
          <thead>
            <tr>
              <th>已授权标准模型</th>
              <th>支持协议</th>
              <th>可用实例数</th>
              <th>状态</th>
            </tr>
          </thead>
          <tbody>
            {models.map((m) => (
              <tr key={m.id}>
                <td>
                  {m.name}
                  <code>{m.id}</code>
                </td>
                <td>{m.protocols.join(" / ") || "媒体计费"}</td>
                <td>{key && groupResources(config, key.tier, m.id).length}</td>
                <td>
                  {resolved?.model?.id === m.id
                    ? "当前估算选择"
                    : "待满足请求条件"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!models.length && (
          <p className="cm-empty">
            此密钥暂无可调用模型；权限或实例恢复后可继续验证。
          </p>
        )}
      </div>
      <section className="cm-callout">
        <h3>真实操作产生的演示调用</h3>
        {requests
          .filter((r) => r.requested_model === "veyra-auto")
          .slice(0, 8)
          .map((r) => (
            <p key={r.trace_id}>
              <Link
                to={`/console/requests?trace=${encodeURIComponent(r.trace_id)}`}
              >
                {r.trace_id}
              </Link>{" "}
              · {r.actual_model ?? r.error_message} ·{" "}
              {r.billing_status === "charged" ? "已计费" : "未计费"}
            </p>
          ))}
        {!requests.some((r) => r.requested_model === "veyra-auto") && (
          <p>
            还没有智能路由请求。可在客户端配置或可选调试台使用同一把密钥验证。
          </p>
        )}
      </section>
      {isAdmin(role) && (
        <details className="cm-callout">
          <summary>高级视图 · 资源快照</summary>
          <p>
            与渠道管理共用快照 <code>{config.resources?.id}</code>；当前{" "}
            {config.resources?.instances.length}{" "}
            个实例。此处不维护另一份资源或路由记录。
          </p>
          <p>
            资源来自本地模拟的资源管理平台。实例状态和演示延迟不代表真实服务质量。
          </p>
          <Button asChild>
            <Link to="/console/channels">检查渠道与资源投放</Link>
          </Button>
        </details>
      )}
    </div>
  );
}
