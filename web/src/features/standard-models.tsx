import { useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { Plus, ArrowLeft, Pencil, Search, Boxes } from "lucide-react";
import { useModelPricing } from "../lib/model-pricing";
import { modelCatalog } from "../lib/platform-catalog";
import type { ModelItem } from "../config/models";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ModelPricingAdmin } from "./model-pricing-admin";
import { useSettingsText } from "./pricing-settings-shared";
import {
  usePlazaModelMeta,
  useUpsertModelMeta,
  useDeleteModelMeta,
  metaToModelItem,
  modelItemToMeta,
} from "../lib/api/plaza-meta";
import { ApiError } from "../lib/api/http";
import "../components/catalog-management.css";

export function StandardModels({ role }: { role: number }) {
  const { text } = useSettingsText();
  const config = useModelPricing((s) => s.config);
  const [params, setParams] = useSearchParams(),
    selected = params.get("model");
  const [query, setQuery] = useState(""),
    [notice, setNotice] = useState("");
  const [editing, setEditing] = useState<{
    model: ModelItem;
    fresh: boolean;
    revision: string;
    price: string;
  } | null>(null);
  const [remove, setRemove] = useState<string | null>(null);

  // Source from backend /api/plaza/meta (AdminAuth); fall back to
  // local catalog when the API is unavailable.
  const metaQuery = usePlazaModelMeta();
  const upsertMeta = useUpsertModelMeta();
  const deleteMeta = useDeleteModelMeta();
  const localModels = modelCatalog(config);
  const models: ModelItem[] =
    metaQuery.data?.length
      ? metaQuery.data.map(metaToModelItem)
      : localModels;
  const current = models.find((m) => m.id === selected);
  const edit = (model?: ModelItem) => {
    setNotice("");
    setEditing({
      fresh: !model,
      revision: config.revision,
      price: "0",
      model: model
        ? structuredClone(model)
        : {
            id: "",
            name: "",
            vendor: "",
            capabilities: ["text"],
            protocols: ["OpenAI"],
            contextLength: 128000,
            contextDisplay: "128K",
            online: true,
            description: { zh: "", en: "", ar: "" },
            tierAvailability: {},
            pricing: {
              input_per_million_usd: "0",
              output_per_million_usd: "0",
            },
          },
    });
  };
  const setModel = (patch: Partial<ModelItem>) =>
    setEditing(
      (value) => value && { ...value, model: { ...value.model, ...patch } },
    );
  const submit = () => {
    if (!editing) return;
    setNotice("");
    upsertMeta.mutate(modelItemToMeta(editing.model), {
      onSuccess: () => {
        setParams({ model: editing.model.id });
        setEditing(null);
      },
      onError: (err) => {
        setNotice(
          err instanceof ApiError
            ? err.message
            : "保存失败，请检查字段和权限后重试。",
        );
      },
    });
  };
  return (
    <div className="catalog-management">
      <header className="cm-heading">
        <div>
          <span className="pricing-eyebrow">
            <Boxes size={16} />
            {text("系统设置", "System settings", "إعدادات النظام")}
          </span>
          <h1>{text("标准模型", "Standard models", "النماذج القياسية")}</h1>
          <p>
            {text(
              "在这里创建对客模型身份并维护定价。资源接入与投放在渠道中管理。",
              "Create customer-facing model identities and pricing. Manage resource placement in Channels.",
              "أنشئ هوية النموذج وأسعاره وأدر الموارد ضمن القنوات.",
            )}
          </p>
        </div>
        <Button variant="primary" onClick={() => edit()}>
          <Plus size={16} />
          {text("创建标准模型", "Create standard model", "إنشاء نموذج قياسي")}
        </Button>
      </header>
      {notice && (
        <p className="pricing-notice" role="status">
          {notice}
        </p>
      )}
      {selected && !current ? (
        <section className="pricing-panel">
          <h2>标准模型不存在</h2>
          <Button onClick={() => setParams({})}>返回模型列表</Button>
        </section>
      ) : current ? (
        <>
          <div className="cm-actions">
            <Button onClick={() => setParams({})}>
              <ArrowLeft size={16} />
              返回标准模型
            </Button>
            <strong>{current.name}</strong>
            <code>{current.id}</code>
            <Button onClick={() => edit(current)}>
              <Pencil size={14} />
              编辑模型信息
            </Button>
            <Link to="/console/channels">查看渠道与资源映射</Link>
          </div>
          <div className="pricing-policy">
            <p>
              标准模型是本平台的调用与定价对象；存在可用、已映射且已投放的资源实例后才能接入。当前已映射{" "}
              {
                Object.values(config.placements ?? {}).filter(
                  (p) => p.modelId === current.id,
                ).length
              }{" "}
              个资源实例。
            </p>
          </div>
          <ModelPricingAdmin
            key={current.id}
            role={role}
            modelId={current.id}
            embedded
          />
        </>
      ) : (
        <>
          <label className="cm-search">
            <Search size={17} />
            <input
              aria-label="搜索标准模型"
              placeholder="搜索模型名称、调用标识或厂商"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <div className="cm-table">
            <table>
              <thead>
                <tr>
                  <th>模型名称 / 调用标识</th>
                  <th>厂商 / 能力</th>
                  <th>计价方式</th>
                  <th>资源映射</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {models
                  .filter((m) =>
                    `${m.id} ${m.name} ${m.vendor}`
                      .toLowerCase()
                      .includes(query.toLowerCase()),
                  )
                  .map((model) => {
                    const count = Object.values(config.placements ?? {}).filter(
                      (p) => p.modelId === model.id,
                    ).length;
                    const rule = config.models[model.id].rule;
                    return (
                      <tr key={model.id}>
                        <td>
                          <button
                            className="cm-text-button"
                            onClick={() => setParams({ model: model.id })}
                          >
                            {model.name}
                          </button>
                          <code>{model.id}</code>
                        </td>
                        <td>
                          {model.vendor}
                          <small>
                            {model.capabilities
                              .map(
                                (c) =>
                                  ({
                                    text: "文本",
                                    code: "编程",
                                    vision: "视觉",
                                    reasoning: "推理",
                                    image: "图片",
                                    video: "视频",
                                  })[c],
                              )
                              .join(" / ")}
                          </small>
                        </td>
                        <td>
                          {rule.mode === "tokens"
                            ? "按词元"
                            : rule.mode === "expression"
                              ? "表达式"
                              : {
                                  image: "按张",
                                  second: "按秒",
                                  request: "按次",
                                }[rule.unit]}
                        </td>
                        <td>{count} 个实例</td>
                        <td>
                          {!model.online
                            ? "已停用"
                            : count
                              ? "已配置"
                              : "待映射资源"}
                        </td>
                        <td>
                          <div className="cm-actions">
                            <Button
                              size="small"
                              onClick={() => setParams({ model: model.id })}
                            >
                              管理定价
                            </Button>
                            <Button size="small" onClick={() => edit(model)}>
                              编辑
                            </Button>
                            <Button
                              size="small"
                              onClick={() => setRemove(model.id)}
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
            {!models.length && <p>暂无标准模型，请先创建。</p>}
          </div>
          <p className="pricing-help">
            {text(
              "已有模型来自后端 /api/plaza/meta。新增模型保存到后端数据库。",
              "Models are loaded from the backend /api/plaza/meta. New models are saved to the backend database.",
              "تُحمّل النماذج من الخادم. تُحفظ النماذج الجديدة في قاعدة بيانات الخادم.",
            )}
          </p>
        </>
      )}
      <Modal
        open={!!editing}
        onOpenChange={(open) => !open && setEditing(null)}
        title={editing?.fresh ? "创建标准模型" : "编辑标准模型"}
        description="标准模型定义客户请求的身份，不包含供应商凭证。所有价格为演示配置。"
        size="wide"
        footer={
          <>
            <Button onClick={() => setEditing(null)}>取消</Button>
            <Button variant="primary" onClick={submit}>
              保存标准模型
            </Button>
          </>
        }
      >
        {editing && (
          <div className="cm-form">
            <label>
              模型名称
              <input
                aria-label="模型名称"
                value={editing.model.name}
                onChange={(e) => setModel({ name: e.target.value })}
                placeholder="例如：GPT 模型 / 通用文本模型"
              />
            </label>
            <label>
              调用标识
              <input
                aria-label="调用标识"
                disabled={!editing.fresh}
                value={editing.model.id}
                onChange={(e) => setModel({ id: e.target.value })}
                placeholder="例如：demo-text-next"
              />
            </label>
            <label>
              模型厂商
              <input
                aria-label="模型厂商"
                value={editing.model.vendor}
                onChange={(e) => setModel({ vendor: e.target.value })}
              />
            </label>
            <label>
              模型类型
              <select
                aria-label="模型类型"
                value={
                  editing.model.capabilities.includes("image")
                    ? "image"
                    : editing.model.capabilities.includes("video")
                      ? "video"
                      : "text"
                }
                onChange={(e) => {
                  const type = e.target.value as "text" | "image" | "video";
                  setModel({
                    capabilities: [type],
                    protocols: type === "text" ? ["OpenAI"] : [],
                    contextLength: type === "text" ? 128000 : 0,
                    contextDisplay: type === "text" ? "128K" : "—",
                  });
                }}
              >
                <option value="text">文本语言模型</option>
                <option value="image">图片计费模型</option>
                <option value="video">视频计费模型</option>
              </select>
            </label>
            {editing.model.capabilities.includes("text") && (
              <>
                <label>
                  上下文上限（词元）
                  <input
                    type="number"
                    min="1"
                    value={editing.model.contextLength}
                    onChange={(e) =>
                      setModel({
                        contextLength: Number(e.target.value),
                        contextDisplay: `${e.target.value} tokens（词元）`,
                      })
                    }
                  />
                </label>
                <fieldset>
                  <legend>适配协议</legend>
                  {(["OpenAI", "Anthropic", "Gemini"] as const).map(
                    (protocol) => (
                      <label className="cm-check" key={protocol}>
                        <input
                          type="checkbox"
                          checked={editing.model.protocols.includes(protocol)}
                          onChange={(e) =>
                            setModel({
                              protocols: e.target.checked
                                ? [...editing.model.protocols, protocol]
                                : editing.model.protocols.filter(
                                    (p) => p !== protocol,
                                  ),
                            })
                          }
                        />
                        {protocol}（协议）
                      </label>
                    ),
                  )}
                </fieldset>
                <fieldset>
                  <legend>能力声明</legend>
                  {(["text", "code", "vision", "reasoning"] as const).map(
                    (cap) => (
                      <label key={cap} className="cm-check">
                        <input
                          type="checkbox"
                          disabled={cap === "text"}
                          checked={editing.model.capabilities.includes(cap)}
                          onChange={(e) =>
                            setModel({
                              capabilities: e.target.checked
                                ? [...editing.model.capabilities, cap]
                                : editing.model.capabilities.filter(
                                    (c) => c !== cap,
                                  ),
                            })
                          }
                        />
                        {
                          {
                            text: "文本",
                            code: "编程",
                            vision: "视觉理解",
                            reasoning: "推理",
                          }[cap]
                        }
                      </label>
                    ),
                  )}
                </fieldset>
              </>
            )}
            {editing.fresh &&
              (editing.model.capabilities.includes("text") ? (
                <>
                  {(
                    ["input_per_million_usd", "output_per_million_usd"] as const
                  ).map((field, i) => (
                    <label key={field}>
                      {i
                        ? "输出基准价（美元 / 百万词元）"
                        : "输入基准价（美元 / 百万词元）"}
                      <input
                        aria-label={i ? "输出基准价" : "输入基准价"}
                        inputMode="decimal"
                        value={editing.model.pricing[field]}
                        onChange={(e) =>
                          setModel({
                            pricing: {
                              ...editing.model.pricing,
                              [field]: e.target.value,
                            },
                          })
                        }
                      />
                    </label>
                  ))}
                </>
              ) : (
                <label>
                  基准价（美元 /{" "}
                  {editing.model.capabilities.includes("image") ? "张" : "秒"}）
                  <input
                    aria-label="单位基准价"
                    inputMode="decimal"
                    value={editing.price}
                    onChange={(e) =>
                      setEditing({ ...editing, price: e.target.value })
                    }
                  />
                </label>
              ))}
            <label className="cm-wide">
              模型说明
              <textarea
                value={editing.model.description.zh}
                onChange={(e) =>
                  setModel({
                    description: {
                      zh: e.target.value,
                      en: e.target.value,
                      ar: e.target.value,
                    },
                  })
                }
              />
            </label>
            <label className="cm-check">
              <input
                type="checkbox"
                checked={editing.model.online}
                onChange={(e) => setModel({ online: e.target.checked })}
              />
              启用此标准模型
            </label>
            <p className="pricing-help cm-wide">
              新建仅配置基础演示价格。保存后可维护缓存、条件价格、分时和来源，不会自动映射或开放上游资源。图片和视频当前仅验证计费。
            </p>
            {notice && (
              <p role="alert" className="settings-error cm-wide">
                {notice}
              </p>
            )}
          </div>
        )}
      </Modal>
      <Modal
        open={!!remove}
        onOpenChange={(open) => !open && setRemove(null)}
        title="删除标准模型"
        description="存在资源、密钥或历史记录引用时将拒绝删除，可改为停用。"
        footer={
          <>
            <Button onClick={() => setRemove(null)}>取消</Button>
            <Button
              variant="primary"
              onClick={() => {
                if (!remove) return;
                setNotice("");
                deleteMeta.mutate(remove, {
                  onSuccess: () => {
                    setRemove(null);
                    setParams({});
                  },
                  onError: (err) => {
                    setNotice(
                      err instanceof ApiError
                        ? err.message
                        : "删除失败，模型可能存在引用。",
                    );
                  },
                });
              }}
            >
              确认删除
            </Button>
          </>
        }
      >
        <p>{remove}</p>
        {notice && <p role="alert">{notice}</p>}
      </Modal>
    </div>
  );
}
