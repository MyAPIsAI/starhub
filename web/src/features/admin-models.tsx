import { messagePair } from "../i18n/messages";
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
    ["chat", t(...messagePair("chat_0d6200e32c"))],
    ["embedding", t(...messagePair("embeddings_39146bd60b"))],
    ["image", t(...messagePair("images_afc737e9b3"))],
    ["audio", t(...messagePair("audio_812b6825a4"))],
    ["video", t(...messagePair("video_133c2b69bf"))],
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
                ...messagePair(
                  "check_token_prices_use_independent_token_pric_674feec840",
                ),
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
      return t(...messagePair("independent_token_prices_0997c36e91"));
    const mode = serverPricingMode(prices, name);
    if (mode === "expression")
      return t(...messagePair("expression_pricing_9b06d58458"));
    if (mode === "per_request")
      return `$${prices.perRequest[name]} / ${t(...messagePair("request_3689ebc447"))}`;
    if (prices.input[name] === undefined)
      return t(...messagePair("not_configured_23f1d5464b"));
    return `$${prices.input[name] * 2} / $${prices.input[name] * 2 * (prices.completion[name] ?? 1)}`;
  };
  return (
    <section className="adm-page">
      <header className="adm-heading">
        <div>
          <h1>{t(...messagePair("models_and_pricing_4debb4201e"))}</h1>
          <p>
            {t(
              ...messagePair(
                "customer_facing_identities_and_base_prices_av_a847fe435d",
              ),
            )}
          </p>
        </div>
        <Button variant="primary" disabled={!data} onClick={() => begin()}>
          <Plus size={15} />
          {t(...messagePair("new_standard_model_8c487445ac"))}
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
              aria-label={t(...messagePair("search_models_64bd2df7c3"))}
              placeholder={t(...messagePair("search_models_64bd2df7c3"))}
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(1);
              }}
            />
            <select
              aria-label={t(...messagePair("model_type_68ecffa04b"))}
              value={kind}
              onChange={(e) => {
                setKind(e.target.value);
                setPage(1);
              }}
            >
              <option value="">
                {t(...messagePair("all_types_a5e050dcc5"))}
              </option>
              {kinds.map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
          </div>
          <AdminTable
            label={t(...messagePair("standard_model_list_9f106a2738"))}
          >
            <thead>
              <tr>
                {[
                  t(...messagePair("model_b80025508f")),
                  t(...messagePair("type_be00a90d64")),
                  t(...messagePair("platform_base_price_f2e6a77078")),
                  t(...messagePair("resource_placement_9fef55b672")),
                  t(...messagePair("available_mapped_8a8d05d364")),
                  t(...messagePair("official_reference_c3da545a24")),
                  t(...messagePair("listed_cbb84e921f")),
                  t(...messagePair("actions_2806edaa15")),
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
                            ...messagePair(
                              "legacy_channel_model_pending_registration_c2b870f137",
                            ),
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
                        t(...messagePair("no_resource_placement_3fbf3ae173"))}
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
                            {t(...messagePair("checked_8e7e4d0ec7"))}{" "}
                            {meta.official_verified_date}
                          </span>
                          {meta.official_historical && (
                            <small className="adm-meta">
                              {t(
                                ...messagePair(
                                  "historical_reference_3559ec5ca6",
                                ),
                              )}
                            </small>
                          )}
                        </>
                      ) : (
                        t(...messagePair("not_supplied_7e96d46eeb"))
                      )}
                    </td>
                    <td>
                      <span
                        className={`adm-status ${row.listed ? "is-active" : "is-disabled"}`}
                      >
                        {row.listed
                          ? t(...messagePair("listed_58b7e232c5"))
                          : t(...messagePair("unlisted_d7d5c259ae"))}
                      </span>
                    </td>
                    <td>
                      <div className="adm-actions">
                        <button className="adm-link" onClick={() => begin(row)}>
                          {t(...messagePair("edit_abd2648b80"))}
                        </button>
                        <button
                          className="adm-link"
                          onClick={() => setHistory(row.model)}
                        >
                          {t(...messagePair("history_57902a31ac"))}
                        </button>
                        {row.revision > 0 && (
                          <button
                            className="adm-link"
                            onClick={() => {
                              deletion.reset();
                              setRemoving(row);
                            }}
                          >
                            {t(...messagePair("delete_1f0f118a6e"))}
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
                ...messagePair(
                  "no_standard_models_create_an_identity_then_ma_83344058b3",
                ),
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
            {t(
              ...messagePair("advanced_billing_and_time_schedules_5a9f19e174"),
            )}
          </summary>
          <p className="adm-meta">
            {t(
              ...messagePair(
                "independent_token_rates_time_multipliers_and__a7097473d1",
              ),
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
            ? t(...messagePair("edit_standard_model_5c00e5889a"))
            : t(...messagePair("new_standard_model_8c487445ac"))
        }
        description={t(
          ...messagePair(
            "changes_take_effect_on_save_and_retain_an_aud_8d63e3bb33",
          ),
        )}
        preventOutsideClose
        footer={
          <div className="adm-actions">
            <Button disabled={mutation.isPending} onClick={close}>
              {t(...messagePair("cancel_82ac496be8"))}
            </Button>
            <Button
              variant="primary"
              disabled={!draft?.row.model || mutation.isPending}
              onClick={() => mutation.mutate()}
            >
              {t(...messagePair("save_c4fd729724"))}
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
                  ["basic", t(...messagePair("basic_information_c3acdfaec2"))],
                  [
                    "price",
                    t(...messagePair("platform_base_price_f2e6a77078")),
                  ],
                  [
                    "official",
                    t(...messagePair("official_reference_86d3f723f6")),
                  ],
                ].map(([id, label]) => (
                  <Tabs.Trigger className="adm-tab" key={id} value={id}>
                    {label}
                  </Tabs.Trigger>
                ))}
              </Tabs.List>
              <Tabs.Content value="basic">
                <div className="adm-form">
                  <label>
                    {t(...messagePair("display_name_c360ad46a1"))}
                    <Input
                      value={draft.row.name}
                      maxLength={128}
                      onChange={(e) => patch({ name: e.target.value })}
                    />
                  </label>
                  <label>
                    {t(...messagePair("stable_model_id_cb7e0f3e05"))}
                    <Input
                      dir="ltr"
                      readOnly={draft.existing}
                      value={draft.row.model}
                      maxLength={128}
                      onChange={(e) => patch({ model: e.target.value })}
                    />
                    <span className="adm-meta">
                      {t(
                        ...messagePair(
                          "immutable_after_creation_customers_send_this__9ee2be71b7",
                        ),
                      )}
                    </span>
                  </label>
                  <label>
                    {t(...messagePair("vendor_02b6ea9581"))}
                    <Input
                      value={draft.row.vendor}
                      maxLength={64}
                      onChange={(e) => patch({ vendor: e.target.value })}
                    />
                  </label>
                  <label>
                    {t(...messagePair("capability_type_91467f56e6"))}
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
                    {t(...messagePair("context_length_tokens_18a5628082"))}
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
                      ...messagePair("capabilities_comma_separated_f080821e09"),
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
                    <p>{t(...messagePair("supported_protocols_1e88846c16"))}</p>
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
                    {t(...messagePair("description_a2be8d4775"))}
                    <textarea
                      rows={3}
                      maxLength={8192}
                      value={draft.row.description}
                      onChange={(e) => patch({ description: e.target.value })}
                    />
                  </label>
                  <label>
                    {t(...messagePair("display_order_03732aba4b"))}
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
                    {t(...messagePair("listed_cbb84e921f"))}
                  </label>
                  <p className="adm-wide adm-meta">
                    {t(
                      ...messagePair(
                        "listing_requires_corresponding_resources_in_t_5f50406ad7",
                      ),
                    )}
                  </p>
                </div>
              </Tabs.Content>
              <Tabs.Content value="price">
                <div className="adm-form">
                  <label className="adm-wide">
                    {t(...messagePair("billing_mode_3109b0fd6d"))}
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
                        {t(...messagePair("per_token_e04996a530"))}
                      </option>
                      <option value="per_request">
                        {t(
                          ...messagePair(
                            "per_request_base_image_price_5fd9266603",
                          ),
                        )}
                      </option>
                      <option value="expression">
                        {t(
                          ...messagePair(
                            "safe_expression_including_tiers_8de2b5a5bc",
                          ),
                        )}
                      </option>
                      <option value="independent">
                        {t(
                          ...messagePair(
                            "keep_independent_token_pricing_d7af7e02f8",
                          ),
                        )}
                      </option>
                    </select>
                  </label>
                  {draft.mode === "per_token" && (
                    <>
                      <label>
                        {t(
                          ...messagePair("input_usd_million_tokens_51251ae8c0"),
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
                          ...messagePair(
                            "output_usd_million_tokens_8f390928fa",
                          ),
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
                      {t(...messagePair("base_unit_price_usd_133a5db3f1"))}
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
                      {t(...messagePair("billing_expression_2d2eda60b8"))}
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
                          ...messagePair(
                            "use_metering_variables_such_as_prompt_tokens__d175ec8975",
                          ),
                        )}
                      </span>
                    </label>
                  )}
                  {draft.mode === "independent" && (
                    <p className="adm-wide">
                      {t(
                        ...messagePair(
                          "maintain_independent_token_rates_in_advanced__f5de173d71",
                        ),
                      )}
                    </p>
                  )}
                  <p className="adm-wide adm-meta">
                    {t(
                      ...messagePair(
                        "this_changes_platform_base_prices_group_multi_00a1485d06",
                      ),
                    )}
                  </p>
                </div>
              </Tabs.Content>
              <Tabs.Content value="official">
                <div className="adm-form">
                  <p className="adm-wide adm-meta">
                    {t(
                      ...messagePair(
                        "manually_maintained_references_separate_from__4bc64cb9e6",
                      ),
                    )}
                  </p>
                  {(
                    [
                      [
                        "official_input_per_m",
                        t(
                          ...messagePair(
                            "reference_input_usd_million_b0d736806e",
                          ),
                        ),
                      ],
                      [
                        "official_output_per_m",
                        t(
                          ...messagePair(
                            "reference_output_usd_million_531c0cc621",
                          ),
                        ),
                      ],
                      [
                        "official_cache_read_per_m",
                        t(
                          ...messagePair(
                            "reference_cache_read_price_8b4ef400fd",
                          ),
                        ),
                      ],
                      [
                        "official_cache_write_per_m",
                        t(
                          ...messagePair(
                            "reference_cache_write_price_686d0ef315",
                          ),
                        ),
                      ],
                      [
                        "official_per_request",
                        t(
                          ...messagePair("reference_unit_price_usd_36e0a9cb87"),
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
                    {t(...messagePair("source_url_206f5408ad"))}
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
                    {t(...messagePair("verification_date_b219ec5269"))}
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
                    {t(...messagePair("historical_reference_dfcc47ed73"))}
                  </label>
                  <label className="adm-wide">
                    {t(...messagePair("applicable_conditions_48905d9727"))}
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
        title={t(...messagePair("discard_unsaved_changes_a37928adaf"))}
        description={t(
          ...messagePair("closing_discards_the_current_changes_ef13183f1a"),
        )}
      />
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(open) => {
          if (!open) setRemoving(null);
        }}
        onConfirm={() => deletion.mutate()}
        title={t(...messagePair("delete_standard_model_35abdf1cb6"))}
        description={t(
          ...messagePair(
            "the_server_rejects_deletion_while_resource_ke_87f1e682c2",
          ),
        )}
      />
      <AdminFeedback error={deletion.error} />
      <Drawer
        open={!!history}
        onOpenChange={(open) => {
          if (!open) setHistory(null);
        }}
        title={t(...messagePair("model_change_history_a65b0a06ba"))}
        description={history ?? ""}
      >
        <div className="adm-page">
          {history && <PricingHistory kind="models" id={history} />}{" "}
          {data?.history
            .filter((h) => h.model === history)
            .map((h) => (
              <article className="adm-card" key={h.id}>
                <strong>
                  {adminDate(h.created_at)} ·{" "}
                  {t(...messagePair("actor_965092921e"))} {h.actor_id}
                </strong>
                <p>{t(...messagePair("before_761653b175"))}</p>
                <ChangeSummary value={h.before} />
                <p>{t(...messagePair("after_d9e2277599"))}</p>
                <ChangeSummary value={h.after} />
              </article>
            ))}
          {!data?.history.some((h) => h.model === history) && (
            <p>
              {t(
                ...messagePair("no_recorded_changes_for_this_model_86105c529a"),
              )}
            </p>
          )}
        </div>
      </Drawer>
    </section>
  );
}
