import { PricingHistory, ChangeSummary } from "./admin-operations";
import { useState } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Drawer, ConfirmDialog } from "../components/ui/overlay";
import { apiGet, apiPut, apiRequest } from "../lib/api/http";
import { useSessionQueryIdentity } from "../lib/system-access";
import type { ModelMeta } from "../lib/api/plaza-meta";
import { emptyModelMetadata } from "../lib/model-metadata";
import {
  readServerPricing,
  replaceServerModelPricing,
  serverPricingMode,
  type ServerPricing,
} from "../lib/server-pricing";
import {
  AdminFeedback,
  AdminPagination,
  AdminTable,
  adminDate,
  useAdminText,
} from "./admin-shared";
import { ServerTokenTimePricingEditor } from "./server-time-token-pricing";
import { ServerTimePricingEditor } from "./server-time-pricing";
import { ServerDetailPricing } from "./server-detail-pricing";

interface ManagedModel {
  model: string;
  name: string;
  vendor: string;
  kind: string;
  protocols: string[];
  capabilities: string[];
  context_length: number;
  description: string;
  listed: boolean;
  sort_weight: number;
  revision: number;
  created_at: number;
  updated_at: number;
}
interface ModelHistory {
  id: number;
  model: string;
  before: string;
  after: string;
  actor_id: number;
  created_at: number;
}
interface ModelCatalog {
  models: ManagedModel[];
  metadata: ModelMeta[];
  catalog_revision: number;
  inventory: Record<
    string,
    {
      corresponding: number;
      mapped: number;
      available: number;
      groups: string[];
    }
  >;
  history: ModelHistory[];
  pricing: { version: number; options: { key: string; value: string }[] };
  loaded_version: number;
  advanced_pricing: Record<string, unknown>;
}
type Draft = {
  existing: boolean;
  row: ManagedModel;
  metadata: ModelMeta;
  catalogVersion: number;
  pricing: ServerPricing;
  mode: "per_token" | "per_request" | "expression" | "independent";
  input: string;
  output: string;
  request: string;
  expression: string;
  priceChanged: boolean;
  original: string;
  tab: string;
};
const protocolNames = [
  ["OpenAI", "OpenAI Chat Completions"],
  ["OpenAIResponses", "OpenAI Responses"],
  ["Anthropic", "Anthropic Messages"],
  ["Gemini", "Gemini"],
  ["OpenAIEmbeddings", "OpenAI Embeddings"],
  ["OpenAIImages", "OpenAI Images"],
  ["OpenAIAudio", "OpenAI Audio"],
  ["OpenAIVideo", "OpenAI Video"],
  ["OpenAIRerank", "Rerank"],
];
export function AdminModels() {
  const t = useAdminText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const query = useQuery({
    queryKey: ["admin-models", ...identity],
    queryFn: () => apiGet<ModelCatalog>("/api/admin/models"),
    retry: false,
  });
  const [search, setSearch] = useState(""),
    [kind, setKind] = useState(""),
    [page, setPage] = useState(1);
  const [draft, setDraft] = useState<Draft | null>(null),
    [discard, setDiscard] = useState(false);
  const [removing, setRemoving] = useState<ManagedModel | null>(null),
    [history, setHistory] = useState<string | null>(null);
  const [localError, setLocalError] = useState<unknown>(null);
  const data = query.data;
  const prices = data
    ? readServerPricing({
        ...data.pricing,
        loaded_version: data.loaded_version,
        atomic_batch: true,
      })
    : undefined;
  const kinds = [
    ["chat", t("文本对话", "Chat")],
    ["embedding", t("向量", "Embeddings")],
    ["image", t("图片", "Images")],
    ["audio", t("音频", "Audio")],
    ["video", t("视频", "Video")],
    ["rerank", "Rerank"],
  ];
  const refresh = async () => {
    await cache.invalidateQueries({ queryKey: ["admin-models"] });
    for (const key of [
      "server-pricing",
      "plaza-meta",
      "plaza-models",
      "plaza-model",
      "plaza-filters",
      "option-history",
      "admin-audit",
    ])
      void cache.invalidateQueries({ queryKey: [key] });
  };
  const mutation = useMutation({
    retry: false,
    mutationFn: async () => {
      if (!draft) return;
      let options: ReturnType<typeof replaceServerModelPricing> = [];
      if (draft.priceChanged && draft.mode !== "independent") {
        if (draft.mode === "per_token") {
          const input = Number(draft.input),
            output = Number(draft.output);
          if (
            draft.input.trim() === "" ||
            draft.output.trim() === "" ||
            !Number.isFinite(input) ||
            !Number.isFinite(output) ||
            input < 0 ||
            output < 0 ||
            (input === 0 && output > 0)
          )
            throw Error(
              t(
                "请检查词元单价；输入价为零但输出收费时，请使用独立词元价格。",
                "Check token prices. Use independent token pricing for zero input with paid output.",
              ),
            );
          options = replaceServerModelPricing(draft.pricing, draft.row.model, {
            mode: "per_token",
            input: String(input / 2),
            completion: String(input === 0 ? 0 : output / input),
          });
        } else if (draft.mode === "per_request")
          options = replaceServerModelPricing(draft.pricing, draft.row.model, {
            mode: "per_request",
            price: draft.request,
          });
        else
          options = replaceServerModelPricing(draft.pricing, draft.row.model, {
            mode: "expression",
            expression: draft.expression,
          });
      }
      return apiPut("/api/admin/models", {
        ...draft.row,
        metadata: draft.metadata,
        expected_revision: draft.row.revision,
        expected_catalog_revision: draft.catalogVersion,
        expected_pricing_version: draft.pricing.version,
        pricing_options: options,
      });
    },
    onSuccess: async () => {
      setDraft(null);
      await refresh();
    },
  });
  const deletion = useMutation({
    retry: false,
    mutationFn: () =>
      apiRequest(`/api/admin/models/${encodeURIComponent(removing!.model)}`, {
        method: "DELETE",
        body: JSON.stringify({
          expected_catalog_revision: data!.catalog_revision,
        }),
      }),
    onSuccess: async () => {
      setRemoving(null);
      await refresh();
    },
  });
  const begin = (row?: ManagedModel) => {
    if (!data || !prices) return;
    mutation.reset();
    setLocalError(null);
    const current = row
      ? structuredClone(row)
      : {
          model: "",
          name: "",
          vendor: "",
          kind: "chat",
          protocols: ["OpenAI"],
          capabilities: [],
          context_length: 0,
          description: "",
          listed: false,
          sort_weight: 0,
          revision: 0,
          created_at: 0,
          updated_at: 0,
        };
    const meta = structuredClone(
      data.metadata.find((m) => m.model === current.model) ??
        emptyModelMetadata(),
    );
    const mode = data.advanced_pricing[current.model]
      ? "independent"
      : serverPricingMode(prices, current.model);
    const input = prices.input[current.model];
    setDraft({
      existing: !!row,
      row: current,
      metadata: meta,
      catalogVersion: data.catalog_revision,
      pricing: prices,
      mode,
      input: input === undefined ? "" : String(input * 2),
      output:
        input === undefined
          ? ""
          : String(input * 2 * (prices.completion[current.model] ?? 1)),
      request: prices.perRequest[current.model]?.toString() ?? "",
      expression: prices.expressions[current.model] ?? "",
      priceChanged: !row,
      original: JSON.stringify([current, meta]),
      tab: "basic",
    });
  };
  const patch = (value: Partial<ManagedModel>) =>
    setDraft((d) => (d ? { ...d, row: { ...d.row, ...value } } : d));
  const patchMeta = (value: Partial<ModelMeta>) =>
    setDraft((d) => (d ? { ...d, metadata: { ...d.metadata, ...value } } : d));
  const close = () => {
    if (!draft || mutation.isPending) return;
    if (
      draft.priceChanged ||
      JSON.stringify([draft.row, draft.metadata]) !== draft.original
    )
      setDiscard(true);
    else setDraft(null);
  };
  const filtered =
    data?.models.filter(
      (m) =>
        (!kind || m.kind === kind) &&
        `${m.name} ${m.model} ${m.vendor}`
          .toLowerCase()
          .includes(search.trim().toLowerCase()),
    ) ?? [];
  const summary = (name: string) => {
    if (!prices) return "—";
    if (data?.advanced_pricing[name])
      return t("独立词元价格", "Independent token prices");
    const mode = serverPricingMode(prices, name);
    if (mode === "expression") return t("按表达式计费", "Expression pricing");
    if (mode === "per_request")
      return `$${prices.perRequest[name]} / ${t("次", "request")}`;
    if (prices.input[name] === undefined)
      return t("尚未配置", "Not configured");
    return `$${prices.input[name] * 2} / $${prices.input[name] * 2 * (prices.completion[name] ?? 1)}`;
  };
  return (
    <section className="adm-page">
      <header className="adm-heading">
        <div>
          <h1>{t("标准模型与价格", "Models and pricing")}</h1>
          <p>
            {t(
              "对客的调用身份与计费基准。可用性由资源映射与投放决定。",
              "Customer-facing identities and base prices. Availability follows resource mapping and placement.",
            )}
          </p>
        </div>
        <Button variant="primary" disabled={!data} onClick={() => begin()}>
          <Plus size={15} />
          {t("新建标准模型", "New standard model")}
        </Button>
      </header>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {data && (
        <div>
          <div className="adm-filters">
            <Input
              aria-label={t("搜索模型", "Search models")}
              placeholder={t("搜索模型", "Search models")}
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(1);
              }}
            />
            <select
              aria-label={t("模型类型", "Model type")}
              value={kind}
              onChange={(e) => {
                setKind(e.target.value);
                setPage(1);
              }}
            >
              <option value="">{t("全部类型", "All types")}</option>
              {kinds.map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
          </div>
          <AdminTable label={t("标准模型列表", "Standard model list")}>
            <thead>
              <tr>
                {[
                  t("模型", "Model"),
                  t("类型", "Type"),
                  t("平台基准价", "Platform base price"),
                  t("实例投放汇总", "Resource placement"),
                  t("可用 / 映射实例", "Available / mapped"),
                  t("官方参考", "Official reference"),
                  t("上架", "Listed"),
                  t("操作", "Actions"),
                ].map((s) => (
                  <th key={s}>{s}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.slice((page - 1) * 20, page * 20).map((row) => {
                const inventory = data.inventory[row.model],
                  meta = data.metadata.find((m) => m.model === row.model);
                return (
                  <tr key={row.model}>
                    <td>
                      <button className="adm-link" onClick={() => begin(row)}>
                        {row.name}
                      </button>
                      <bdi className="adm-meta">{row.model}</bdi>
                      {row.revision === 0 && (
                        <span className="adm-meta">
                          {t(
                            "历史渠道模型 · 待登记",
                            "Legacy channel model · pending registration",
                          )}
                        </span>
                      )}
                    </td>
                    <td>{kinds.find(([id]) => id === row.kind)?.[1]}</td>
                    <td>
                      <bdi>{summary(row.model)}</bdi>
                    </td>
                    <td>
                      <div className="adm-model-availability">
                        {inventory?.groups?.map((g) => (
                          <span key={g}>{g}</span>
                        ))}
                      </div>
                      {!inventory?.groups.length &&
                        t("暂无实例投放", "No resource placement")}
                    </td>
                    <td>
                      <bdi>
                        {inventory?.available ?? 0} / {inventory?.mapped ?? 0}
                      </bdi>
                    </td>
                    <td>
                      {meta?.official_verified_date ? (
                        <>
                          <span>
                            {t("核验于", "Checked")}{" "}
                            {meta.official_verified_date}
                          </span>
                          {meta.official_historical && (
                            <small className="adm-meta">
                              {t("历史参考", "Historical reference")}
                            </small>
                          )}
                        </>
                      ) : (
                        t("未填写", "Not supplied")
                      )}
                    </td>
                    <td>
                      <span
                        className={`adm-status ${row.listed ? "is-active" : "is-disabled"}`}
                      >
                        {row.listed
                          ? t("已上架", "Listed")
                          : t("未上架", "Unlisted")}
                      </span>
                    </td>
                    <td>
                      <div className="adm-actions">
                        <button className="adm-link" onClick={() => begin(row)}>
                          {t("编辑", "Edit")}
                        </button>
                        <button
                          className="adm-link"
                          onClick={() => setHistory(row.model)}
                        >
                          {t("历史", "History")}
                        </button>
                        {row.revision > 0 && (
                          <button
                            className="adm-link"
                            onClick={() => {
                              deletion.reset();
                              setRemoving(row);
                            }}
                          >
                            {t("删除", "Delete")}
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </AdminTable>
          {!filtered.length && (
            <p>
              {t(
                "暂无标准模型。请先创建调用身份，再映射并投放资源。",
                "No standard models. Create an identity, then map and place resources.",
              )}
            </p>
          )}
          <AdminPagination
            page={page}
            total={filtered.length}
            onChange={setPage}
          />
        </div>
      )}
      {prices && (
        <details className="adm-card">
          <summary>
            {t("高级计费与分时设置", "Advanced billing and time schedules")}
          </summary>
          <p className="adm-meta">
            {t(
              "独立词元单价、分时系数与条件单位价使用同一套服务器结算规则。",
              "Independent token rates, time multipliers and conditional unit prices use the server settlement rules.",
            )}
          </p>
          <ServerTokenTimePricingEditor />
          <ServerTimePricingEditor />
          <ServerDetailPricing data={prices} />
        </details>
      )}
      <Drawer
        open={!!draft}
        onOpenChange={(open) => {
          if (!open) close();
        }}
        title={
          draft?.row.revision
            ? t("编辑标准模型", "Edit standard model")
            : t("新建标准模型", "New standard model")
        }
        description={t(
          "保存后立即生效，操作与价格变更保留历史。",
          "Changes take effect on save and retain an audit and pricing history.",
        )}
        preventOutsideClose
        footer={
          <div className="adm-actions">
            <Button disabled={mutation.isPending} onClick={close}>
              {t("取消", "Cancel")}
            </Button>
            <Button
              variant="primary"
              disabled={!draft?.row.model || mutation.isPending}
              onClick={() => mutation.mutate()}
            >
              {t("保存", "Save")}
            </Button>
          </div>
        }
      >
        {draft && (
          <div className="adm-drawer-content">
            <Tabs.Root
              value={draft.tab}
              onValueChange={(tab) => setDraft((d) => (d ? { ...d, tab } : d))}
            >
              <Tabs.List className="adm-tabs">
                {[
                  ["basic", t("基本信息", "Basic information")],
                  ["price", t("平台基准价", "Platform base price")],
                  ["official", t("官方参考价", "Official reference")],
                ].map(([id, label]) => (
                  <Tabs.Trigger className="adm-tab" key={id} value={id}>
                    {label}
                  </Tabs.Trigger>
                ))}
              </Tabs.List>
              <Tabs.Content value="basic">
                <div className="adm-form">
                  <label>
                    {t("显示名称", "Display name")}
                    <Input
                      value={draft.row.name}
                      maxLength={128}
                      onChange={(e) => patch({ name: e.target.value })}
                    />
                  </label>
                  <label>
                    {t("稳定调用标识", "Stable model ID")}
                    <Input
                      dir="ltr"
                      readOnly={draft.existing}
                      value={draft.row.model}
                      maxLength={128}
                      onChange={(e) => patch({ model: e.target.value })}
                    />
                    <span className="adm-meta">
                      {t(
                        "创建后不可修改，客户在请求中填写。",
                        "Immutable after creation; customers send this value in requests.",
                      )}
                    </span>
                  </label>
                  <label>
                    {t("厂商", "Vendor")}
                    <Input
                      value={draft.row.vendor}
                      maxLength={64}
                      onChange={(e) => patch({ vendor: e.target.value })}
                    />
                  </label>
                  <label>
                    {t("能力类型", "Capability type")}
                    <select
                      value={draft.row.kind}
                      onChange={(e) => patch({ kind: e.target.value })}
                    >
                      {kinds.map(([id, label]) => (
                        <option key={id} value={id}>
                          {label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    {t("上下文长度（词元）", "Context length (tokens)")}
                    <Input
                      type="number"
                      min={0}
                      step={1}
                      value={draft.row.context_length}
                      onChange={(e) =>
                        patch({ context_length: Number(e.target.value) })
                      }
                    />
                  </label>
                  <label>
                    {t(
                      "能力标签（逗号分隔）",
                      "Capabilities (comma separated)",
                    )}
                    <Input
                      defaultValue={draft.row.capabilities.join(",")}
                      onChange={(e) =>
                        patch({
                          capabilities: e.target.value
                            .split(",")
                            .map((x) => x.trim())
                            .filter(Boolean),
                        })
                      }
                    />
                  </label>
                  <div className="adm-wide">
                    <p>{t("支持协议", "Supported protocols")}</p>
                    <div className="adm-checks">
                      {protocolNames.map(([id, label]) => (
                        <label className="adm-check" key={id}>
                          <input
                            type="checkbox"
                            checked={draft.row.protocols.includes(id)}
                            onChange={(e) =>
                              patch({
                                protocols: e.target.checked
                                  ? [...draft.row.protocols, id]
                                  : draft.row.protocols.filter((p) => p !== id),
                              })
                            }
                          />
                          {label}
                        </label>
                      ))}
                    </div>
                  </div>
                  <label className="adm-wide">
                    {t("简介", "Description")}
                    <textarea
                      rows={3}
                      maxLength={8192}
                      value={draft.row.description}
                      onChange={(e) => patch({ description: e.target.value })}
                    />
                  </label>
                  <label>
                    {t("显示顺序", "Display order")}
                    <Input
                      type="number"
                      step={1}
                      value={draft.row.sort_weight}
                      onChange={(e) =>
                        patch({ sort_weight: Number(e.target.value) })
                      }
                    />
                  </label>
                  <label className="adm-check">
                    <input
                      type="checkbox"
                      checked={draft.row.listed}
                      onChange={(e) => patch({ listed: e.target.checked })}
                    />
                    {t("上架", "Listed")}
                  </label>
                  <p className="adm-wide adm-meta">
                    {t(
                      "上架须有符合地区和厂商限制的对应资源。下架后从模型广场隐藏，新调用返回 model_not_found。",
                      "Listing requires corresponding resources in the permitted region/vendor scope. Unlisting hides the model and rejects new calls with model_not_found.",
                    )}
                  </p>
                </div>
              </Tabs.Content>
              <Tabs.Content value="price">
                <div className="adm-form">
                  <label className="adm-wide">
                    {t("计费方式", "Billing mode")}
                    <select
                      disabled={!!data?.advanced_pricing[draft.row.model]}
                      value={draft.mode}
                      onChange={(e) =>
                        setDraft((d) =>
                          d
                            ? {
                                ...d,
                                mode: e.target.value as Draft["mode"],
                                priceChanged: true,
                              }
                            : d,
                        )
                      }
                    >
                      <option value="per_token">
                        {t("按词元", "Per token")}
                      </option>
                      <option value="per_request">
                        {t(
                          "按次 / 图片基础价",
                          "Per request / base image price",
                        )}
                      </option>
                      <option value="expression">
                        {t(
                          "安全算术表达式（含阶梯）",
                          "Safe expression (including tiers)",
                        )}
                      </option>
                      <option value="independent">
                        {t(
                          "保留独立词元价格",
                          "Keep independent token pricing",
                        )}
                      </option>
                    </select>
                  </label>
                  {draft.mode === "per_token" && (
                    <>
                      <label>
                        {t(
                          "输入价（USD / 百万词元）",
                          "Input USD / million tokens",
                        )}
                        <Input
                          inputMode="decimal"
                          value={draft.input}
                          onChange={(e) =>
                            setDraft((d) =>
                              d
                                ? {
                                    ...d,
                                    input: e.target.value,
                                    priceChanged: true,
                                  }
                                : d,
                            )
                          }
                        />
                      </label>
                      <label>
                        {t(
                          "输出价（USD / 百万词元）",
                          "Output USD / million tokens",
                        )}
                        <Input
                          inputMode="decimal"
                          value={draft.output}
                          onChange={(e) =>
                            setDraft((d) =>
                              d
                                ? {
                                    ...d,
                                    output: e.target.value,
                                    priceChanged: true,
                                  }
                                : d,
                            )
                          }
                        />
                      </label>
                    </>
                  )}
                  {draft.mode === "per_request" && (
                    <label>
                      {t("基础单价（USD）", "Base unit price (USD)")}
                      <Input
                        inputMode="decimal"
                        value={draft.request}
                        onChange={(e) =>
                          setDraft((d) =>
                            d
                              ? {
                                  ...d,
                                  request: e.target.value,
                                  priceChanged: true,
                                }
                              : d,
                          )
                        }
                      />
                    </label>
                  )}
                  {draft.mode === "expression" && (
                    <label className="adm-wide">
                      {t("计费表达式", "Billing expression")}
                      <textarea
                        rows={5}
                        dir="ltr"
                        value={draft.expression}
                        onChange={(e) =>
                          setDraft((d) =>
                            d
                              ? {
                                  ...d,
                                  expression: e.target.value,
                                  priceChanged: true,
                                }
                              : d,
                          )
                        }
                      />
                      <span className="adm-meta">
                        {t(
                          "使用 prompt_tokens、completion_tokens 等计量变量，结果单位为美元。保存时由服务器校验。",
                          "Use metering variables such as prompt_tokens and completion_tokens; the result is USD. The server validates the expression.",
                        )}
                      </span>
                    </label>
                  )}
                  {draft.mode === "independent" && (
                    <p className="adm-wide">
                      {t(
                        "独立词元价格在页面下方“高级计费与分时设置”中维护。此处保留已经保存的规则。",
                        "Maintain independent token rates in Advanced billing and time schedules below the model list. This form preserves the saved rules.",
                      )}
                    </p>
                  )}
                  <p className="adm-wide adm-meta">
                    {t(
                      "这里只修改平台基准价；服务分组倍率另行维护，已受理调用和历史账单保留原价。",
                      "This changes platform base prices. Group multipliers are managed separately; accepted calls and historical charges keep their original rates.",
                    )}
                  </p>
                </div>
              </Tabs.Content>
              <Tabs.Content value="official">
                <div className="adm-form">
                  <p className="adm-wide adm-meta">
                    {t(
                      "人工维护的参考值，与平台基准价、采购成本分开。系统不自动核验官网价格。",
                      "Manually maintained references, separate from platform prices and procurement costs. The system does not automatically verify vendor prices.",
                    )}
                  </p>
                  {(
                    [
                      [
                        "official_input_per_m",
                        t(
                          "官方输入价（USD / 百万词元）",
                          "Reference input USD / million",
                        ),
                      ],
                      [
                        "official_output_per_m",
                        t(
                          "官方输出价（USD / 百万词元）",
                          "Reference output USD / million",
                        ),
                      ],
                      [
                        "official_cache_read_per_m",
                        t("官方缓存读取价", "Reference cache-read price"),
                      ],
                      [
                        "official_cache_write_per_m",
                        t("官方缓存写入价", "Reference cache-write price"),
                      ],
                      [
                        "official_per_request",
                        t(
                          "官方按次 / 单位价（USD）",
                          "Reference unit price (USD)",
                        ),
                      ],
                    ] as const
                  ).map(([field, label]) => (
                    <label key={field}>
                      {label}
                      <Input
                        type="number"
                        min={0}
                        step="any"
                        value={draft.metadata[field] ?? ""}
                        onChange={(e) =>
                          patchMeta({
                            [field]:
                              e.target.value === ""
                                ? null
                                : Number(e.target.value),
                          })
                        }
                      />
                    </label>
                  ))}
                  <label className="adm-wide">
                    {t("来源地址", "Source URL")}
                    <Input
                      type="url"
                      dir="ltr"
                      value={draft.metadata.official_source_url ?? ""}
                      onChange={(e) =>
                        patchMeta({ official_source_url: e.target.value })
                      }
                    />
                  </label>
                  <label>
                    {t("核验日期", "Verification date")}
                    <Input
                      type="date"
                      value={draft.metadata.official_verified_date ?? ""}
                      onChange={(e) =>
                        patchMeta({ official_verified_date: e.target.value })
                      }
                    />
                  </label>
                  <label className="adm-check">
                    <input
                      type="checkbox"
                      checked={draft.metadata.official_historical ?? false}
                      onChange={(e) =>
                        patchMeta({ official_historical: e.target.checked })
                      }
                    />
                    {t("历史参考（退役模型等）", "Historical reference")}
                  </label>
                  <label className="adm-wide">
                    {t("适用条件", "Applicable conditions")}
                    <textarea
                      rows={3}
                      value={draft.metadata.official_conditions ?? ""}
                      onChange={(e) =>
                        patchMeta({ official_conditions: e.target.value })
                      }
                    />
                  </label>
                </div>
              </Tabs.Content>
            </Tabs.Root>
            <AdminFeedback error={mutation.error ?? localError} />
          </div>
        )}
      </Drawer>
      <ConfirmDialog
        open={discard}
        onOpenChange={setDiscard}
        onConfirm={() => {
          setDiscard(false);
          setDraft(null);
        }}
        title={t("放弃未保存的修改？", "Discard unsaved changes?")}
        description={t(
          "关闭后，本次修改不会保存。",
          "Closing discards the current changes.",
        )}
      />
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(open) => {
          if (!open) setRemoving(null);
        }}
        onConfirm={() => deletion.mutate()}
        title={t("删除标准模型？", "Delete standard model?")}
        description={t(
          "存在资源映射、Key 或历史引用时，服务器会拒绝删除。可使用下架保留追溯。",
          "The server rejects deletion while resource, Key or historical references exist. Unlist the model to preserve traceability.",
        )}
      />
      <AdminFeedback error={deletion.error} />
      <Drawer
        open={!!history}
        onOpenChange={(open) => {
          if (!open) setHistory(null);
        }}
        title={t("模型变更历史", "Model change history")}
        description={history ?? ""}
      >
        <div className="adm-page">
          {history && <PricingHistory kind="models" id={history} />}{" "}
          {data?.history
            .filter((h) => h.model === history)
            .map((h) => (
              <article className="adm-card" key={h.id}>
                <strong>
                  {adminDate(h.created_at)} · {t("操作人", "Actor")}{" "}
                  {h.actor_id}
                </strong>
                <p>{t("修改前", "Before")}</p>
                <ChangeSummary value={h.before} />
                <p>{t("修改后", "After")}</p>
                <ChangeSummary value={h.after} />
              </article>
            ))}
          {!data?.history.some((h) => h.model === history) && (
            <p>
              {t(
                "暂无该模型的变更记录。",
                "No recorded changes for this model.",
              )}
            </p>
          )}
        </div>
      </Drawer>
    </section>
  );
}
