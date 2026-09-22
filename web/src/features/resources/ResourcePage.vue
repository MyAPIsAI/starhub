<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useQuery } from "@tanstack/vue-query";
import { useDebounceFn, useClipboard } from "@vueuse/core";
import { useI18n } from "vue-i18n";
import { toast } from "vue-sonner";
import {
  Plus,
  RefreshCw,
  Copy,
  Pencil,
  Trash2,
  Activity,
  Search,
} from "lucide-vue-next";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import PageHeading from "@/components/layout/PageHeading.vue";
import FormField from "@/components/layout/FormField.vue";
import DiscardDialog from "@/components/layout/DiscardDialog.vue";
import AsyncState from "@/components/states/AsyncState.vue";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import { canEdit } from "@/router/capabilities";
import { quota, number, date } from "@/lib/format";
import { ApiError, errorMessage } from "@/lib/http";
import type { Row, PageResult } from "@/lib/types";
import { queryClient } from "@/lib/query";
import { useUnsaved } from "@/composables/useUnsaved";
import {
  resources,
  makeDraft,
  buildPayload,
  type Kind,
  type Draft,
} from "./schema";
import * as resource from "./api";
const route = useRoute();
const router = useRouter();
const auth = useAuth();
const ui = useUI();
const { t } = useI18n();
const { copy } = useClipboard();
const kind = computed(() => route.params.section as Kind);
const config = computed(() => resources[kind.value]);
const page = computed(() =>
  Math.max(1, Math.min(1000000, Math.floor(Number(route.query.p)) || 1)),
);
const size = computed(() =>
  [10, 20, 50, 100].includes(Number(route.query.page_size))
    ? Number(route.query.page_size)
    : 20,
);
const filter = computed(() =>
  String(route.query[config.value.paged ? "model" : "q"] ?? ""),
);
const scope = computed(() => (route.query.scope === "mine" ? "mine" : "all"));
const sort = computed(() => String(route.query.sort ?? "newest"));
const admin = computed(() => (auth.user?.role ?? 0) >= 10);
const editable = computed(() => canEdit(auth.user?.role ?? 0, kind.value));
const columns = computed(() =>
  config.value.columns.filter((c) => admin.value || !["user_id"].includes(c)),
);
const query = useQuery({
  queryKey: computed(() => [
    auth.user?.id,
    kind.value,
    ...(config.value.paged ? [page.value, size.value, filter.value] : []),
  ]),
  enabled: computed(() => Boolean(auth.user)),
  queryFn: ({ signal }) =>
    resource.list(
      kind.value,
      config.value.paged
        ? `?p=${page.value}&page_size=${size.value}&model=${encodeURIComponent(filter.value)}`
        : "",
      signal,
    ),
});
const allRows = computed<Row[]>(
  () =>
    (config.value.paged
      ? (query.data.value as PageResult | undefined)?.items
      : (query.data.value as Row[] | undefined)) ?? [],
);
const filteredRows = computed(() => {
  if (config.value.paged) return allRows.value;
  const term = filter.value.toLowerCase();
  return allRows.value
    .filter(
      (row) =>
        (scope.value !== "mine" || row.user_id === auth.user?.id) &&
        (!term ||
          String(row.name ?? row.username ?? row.id)
            .toLowerCase()
            .includes(term)),
    )
    .slice()
    .sort((a, b) =>
      sort.value === "name"
        ? String(a.name ?? a.username ?? a.id).localeCompare(
            String(b.name ?? b.username ?? b.id),
          )
        : sort.value === "oldest"
          ? a.id - b.id
          : b.id - a.id,
    );
});
const total = computed(() =>
  config.value.paged
    ? ((query.data.value as PageResult | undefined)?.total ?? 0)
    : filteredRows.value.length,
);
const pages = computed(() => Math.max(1, Math.ceil(total.value / size.value)));
const rows = computed(() =>
  config.value.paged
    ? filteredRows.value
    : filteredRows.value.slice(
        (page.value - 1) * size.value,
        page.value * size.value,
      ),
);
function setQuery(values: Record<string, string | number | undefined>) {
  const next = { ...route.query };
  for (const [key, value] of Object.entries(values)) {
    if (value === "" || value === undefined) delete next[key];
    else next[key] = String(value);
  }
  void router.replace({ query: next });
}
const updateFilter = useDebounceFn(
  (value: string) =>
    setQuery({ [config.value.paged ? "model" : "q"]: value, p: 1 }),
  300,
);
watch([total, pages], () => {
  if (query.isSuccess.value && page.value > pages.value)
    setQuery({ p: pages.value });
});
const editor = ref(false);
const original = ref<Row>();
const draft = ref<Draft>({});
const initial = ref("");
const busy = ref(false);
const formError = ref("");
const balanceConfirmed = ref(false);
const dirty = computed(
  () => editor.value && JSON.stringify(draft.value) !== initial.value,
);
const { discardOpen, ask, answer } = useUnsaved(dirty);
const balanceKey = computed(() =>
  kind.value === "users" ? "quota" : "remain_quota",
);
const balanceChanged = computed(
  () =>
    Boolean(original.value) &&
    ["users", "tokens"].includes(kind.value) &&
    (String(draft.value[balanceKey.value]) !==
      String(original.value?.[balanceKey.value]) ||
      (kind.value === "tokens" &&
        Boolean(draft.value.unlimited_quota) !==
          Boolean(original.value?.unlimited_quota))),
);
function openEditor(row?: Row) {
  original.value = row ? { ...row } : undefined;
  draft.value = makeDraft(kind.value, row);
  initial.value = JSON.stringify(draft.value);
  formError.value = "";
  balanceConfirmed.value = false;
  editor.value = true;
}
async function closeEditor(value: boolean) {
  if (value) return;
  if (busy.value) return;
  if (await ask()) {
    editor.value = false;
    draft.value = {};
    original.value = undefined;
  }
}
const secret = ref("");
async function refresh() {
  await query.refetch();
}
async function save() {
  formError.value = "";
  if (balanceChanged.value && !balanceConfirmed.value) return;
  let payload: ReturnType<typeof buildPayload>;
  try {
    payload = buildPayload(kind.value, draft.value, original.value);
  } catch (e) {
    formError.value = errorMessage(e);
    return;
  }
  busy.value = true;
  try {
    const result = await resource.save(
      kind.value,
      payload,
      Boolean(original.value),
    );
    if (kind.value === "tokens" && !original.value)
      secret.value = String(result.key ?? "");
    if (kind.value === "redemptions" && result.codes)
      secret.value = result.codes.join("\n");
    editor.value = false;
    draft.value = {};
    original.value = undefined;
    toast.success(t("updated"));
    await refresh();
    if (kind.value === "users") await auth.refresh();
  } catch (e) {
    formError.value =
      e instanceof ApiError && e.status === 409
        ? t("conflict")
        : errorMessage(e);
    if (e instanceof ApiError && e.status === 0)
      formError.value += ` ${t("pendingUncertain")}`;
    void refresh();
  } finally {
    busy.value = false;
  }
}
const deletion = ref<Row>();
const deleteError = ref("");
async function deleteRow() {
  if (!deletion.value) return;
  busy.value = true;
  deleteError.value = "";
  try {
    await resource.remove(kind.value, deletion.value.id);
    deletion.value = undefined;
    toast.success(t("deleted"));
    await refresh();
  } catch (e) {
    deleteError.value = errorMessage(e);
    void refresh();
  } finally {
    busy.value = false;
  }
}
async function copyText(value: string) {
  try {
    await copy(value);
    toast.success(t("copied"));
  } catch {
    toast.error(t("copyFailed"));
  }
}
const revealed = ref(new Set<number>());
const probing = ref<Row>();
const probeModel = ref("");
const probeBusy = ref(false);
const probeResults = ref<
  Record<number, { ok: boolean; message: string; elapsed_ms: number }>
>({});
function openProbe(row: Row) {
  probing.value = row;
  probeModel.value = String(row.models).split(",")[0] ?? "";
}
async function testChannel() {
  if (!probing.value) return;
  probeBusy.value = true;
  const id = probing.value.id;
  try {
    probeResults.value[id] = await resource.probe(id, probeModel.value);
    probing.value = undefined;
    await refresh();
    await queryClient.invalidateQueries({
      predicate: (q) => q.queryKey.includes("metrics"),
    });
  } catch (e) {
    probeResults.value[id] = {
      ok: false,
      message: errorMessage(e),
      elapsed_ms: 0,
    };
    probing.value = undefined;
  } finally {
    probeBusy.value = false;
  }
}
const moneyColumns = ["quota", "remain_quota", "used_quota", "unfunded_quota"];
function status(row: Row) {
  if (kind.value === "audit")
    return {
      text: String(row.status),
      class: Number(row.status) < 400 ? "success" : "danger",
    };
  if (kind.value === "redemptions")
    return Number(row.status) === 2
      ? { text: t("redeemed"), class: "" }
      : Number(row.expired_at) > 0 && Number(row.expired_at) * 1000 < Date.now()
        ? { text: t("expired"), class: "warning" }
        : { text: t("unused"), class: "success" };
  if (
    kind.value === "tokens" &&
    Number(row.expired_time) > 0 &&
    Number(row.expired_time) * 1000 < Date.now()
  )
    return { text: t("expired"), class: "warning" };
  return {
    text: t(
      Number(row.status) === 1
        ? "enabled"
        : Number(row.status) === 3
          ? "autoDisabled"
          : "disabled",
    ),
    class:
      Number(row.status) === 1
        ? "success"
        : Number(row.status) === 3
          ? "warning"
          : "",
  };
}
function value(row: Row, column: string) {
  if (moneyColumns.includes(column)) {
    if (
      kind.value === "tokens" &&
      row.unlimited_quota &&
      ["remain_quota", "used_quota"].includes(column)
    )
      return t(column === "remain_quota" ? "unlimited" : "notApplicable");
    return quota(row[column], ui.locale, ui.unit);
  }
  if (["created_at", "used_at", "expired_at", "expired_time"].includes(column))
    return ["expired_at", "expired_time"].includes(column) &&
      Number(row[column]) <= 0
      ? t("never")
      : date(row[column], ui.locale);
  if (column === "role")
    return t(
      Number(row.role) >= 100
        ? "roleRoot"
        : Number(row.role) >= 10
          ? "roleAdmin"
          : "roleUser",
    );
  if (column === "type")
    return (
      (
        { 1: "OpenAI", 14: "Claude", 15: "Bedrock", 25: "Gemini" } as Record<
          number,
          string
        >
      )[Number(row.type)] ?? String(row.type)
    );
  if (column === "actor")
    return row.username ? `${row.username} #${row.user_id}` : t("unknownActor");
  if (column === "group" && !row.group) return t("inherited");
  if (typeof row[column] === "number")
    return (
      number(row[column], ui.locale) + (column === "response_time" ? " ms" : "")
    );
  if (typeof row[column] === "object") return JSON.stringify(row[column]);
  return String(row[column] ?? "—") || "—";
}
</script>
<template>
  <PageHeading :title="t(kind)" :description="t(`${kind}Desc`)"
    ><Button
      variant="outline"
      :disabled="query.isFetching.value"
      @click="refresh"
      ><RefreshCw :size="15" />{{ t("refresh") }}</Button
    ><Button v-if="config.create && editable" @click="openEditor()"
      ><Plus :size="16" />{{ t("create") }}</Button
    ></PageHeading
  >
  <p
    v-if="kind === 'tokens' || kind === 'logs'"
    class="notice"
    style="margin-bottom: 18px"
  >
    {{ t(kind === "tokens" ? "tokenHelp" : "collectionNote") }}
  </p>
  <section class="panel" :class="ui.density">
    <div class="toolbar">
      <Search v-if="kind !== 'audit'" :size="16" class="muted" /><input
        v-if="kind !== 'audit'"
        :value="filter"
        :placeholder="t(config.paged ? 'modelFilter' : 'search')"
        :aria-label="t(config.paged ? 'modelFilter' : 'search')"
        @input="updateFilter(($event.target as HTMLInputElement).value)"
      /><select
        v-if="kind === 'tokens' && admin"
        :value="scope"
        :aria-label="t('user_id')"
        @change="
          setQuery({ scope: ($event.target as HTMLSelectElement).value, p: 1 })
        "
      >
        <option value="all">{{ t("all") }}</option>
        <option value="mine">{{ t("mine") }}</option></select
      ><select
        v-if="!config.paged"
        :value="sort"
        :aria-label="t('sort')"
        @change="
          setQuery({ sort: ($event.target as HTMLSelectElement).value, p: 1 })
        "
      >
        <option value="newest">{{ t("newest") }}</option>
        <option value="oldest">{{ t("oldest") }}</option>
        <option value="name">{{ t("nameSort") }}</option></select
      ><span class="spacer"></span
      ><select v-model="ui.unit" :aria-label="t('unit')">
        <option value="usd">USD</option>
        <option value="quota">Quota</option></select
      ><select v-model="ui.density" :aria-label="t('density')">
        <option value="compact">{{ t("compact") }}</option>
        <option value="comfortable">{{ t("comfortable") }}</option>
      </select>
    </div>
    <p v-if="query.isError.value && query.data.value" class="notice warning">
      {{ t("stale") }} {{ errorMessage(query.error.value) }}
    </p>
    <AsyncState
      :loading="query.isPending.value"
      :error="!query.data.value && query.error.value"
      :empty="rows.length === 0"
      :filtered="Boolean(filter)"
      @retry="refresh"
    >
      <template #action
        ><Button
          v-if="config.create && editable && !filter"
          @click="openEditor()"
          >{{ t("create") }}</Button
        ></template
      >
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th
                v-for="column in columns"
                :key="column"
                :class="{ numeric: moneyColumns.includes(column) }"
              >
                {{
                  t(column === "quota" && kind === "logs" ? "charged" : column)
                }}
              </th>
              <th v-if="editable || kind === 'channels'">{{ t("actions") }}</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="row in rows" :key="row.id">
              <tr>
                <td
                  v-for="column in columns"
                  :key="column"
                  :class="{ numeric: moneyColumns.includes(column) }"
                >
                  <span
                    v-if="column === 'status'"
                    class="badge"
                    :class="status(row).class"
                    ><span class="badge-dot"></span>{{ status(row).text }}</span
                  ><template v-else-if="column === 'code'"
                    ><span class="mono">{{
                      revealed.has(row.id) ? row.code : "RD-••••••••"
                    }}</span
                    ><button
                      class="text-button"
                      @click="
                        revealed.has(row.id)
                          ? revealed.delete(row.id)
                          : revealed.add(row.id)
                      "
                    >
                      {{ t(revealed.has(row.id) ? "hide" : "show") }}</button
                    ><button
                      v-if="revealed.has(row.id)"
                      class="text-button"
                      :aria-label="t('copy')"
                      @click="copyText(String(row.code))"
                    >
                      <Copy :size="13" /></button></template
                  ><template
                    v-else-if="column === 'name' || column === 'username'"
                    ><span class="record-name">{{ value(row, column) }}</span
                    ><span class="record-sub">#{{ row.id }}</span></template
                  ><span
                    v-else
                    class="truncate-cell"
                    :class="{
                      mono: column === 'key' || column === 'used_channels',
                    }"
                    :title="value(row, column)"
                    >{{ value(row, column) }}</span
                  >
                </td>
                <td v-if="editable || kind === 'channels'">
                  <div class="row-actions">
                    <button
                      v-if="config.edit && editable"
                      class="text-button"
                      :aria-label="`${t('edit')} ${row.name ?? row.username ?? row.id}`"
                      @click="openEditor(row)"
                    >
                      <Pencil :size="15" /></button
                    ><button
                      v-if="kind === 'channels'"
                      class="text-button"
                      :aria-label="`${t('probe')} ${row.name}`"
                      @click="openProbe(row)"
                    >
                      <Activity :size="15" /></button
                    ><button
                      v-if="config.remove && editable"
                      class="text-button danger"
                      :aria-label="`${t('delete')} ${row.name ?? row.id}`"
                      @click="
                        deletion = row;
                        deleteError = '';
                      "
                    >
                      <Trash2 :size="15" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="kind === 'channels' && probeResults[row.id]">
                <td :colspan="columns.length + 1">
                  <span
                    class="badge"
                    :class="probeResults[row.id]?.ok ? 'success' : 'danger'"
                    >{{
                      t(
                        probeResults[row.id]?.ok
                          ? "probeSuccess"
                          : "probeFailed",
                      )
                    }}
                    · {{ probeResults[row.id]?.elapsed_ms }} ms</span
                  >
                  <span class="help">{{ probeResults[row.id]?.message }}</span>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </AsyncState>
    <div class="pagination">
      <span>{{ t("rowCount", { count: total }) }}</span>
      <div class="pagination-controls">
        <select
          :value="size"
          :aria-label="t('pageSize')"
          @change="
            setQuery({
              page_size: Number(($event.target as HTMLSelectElement).value),
              p: 1,
            })
          "
        >
          <option v-for="n in [10, 20, 50, 100]" :key="n" :value="n">
            {{ n }}
          </option></select
        ><Button
          variant="outline"
          size="sm"
          :disabled="page <= 1"
          @click="setQuery({ p: page - 1 })"
          >{{ t("previous") }}</Button
        ><span class="numeric">{{ page }} / {{ pages }}</span
        ><Button
          variant="outline"
          size="sm"
          :disabled="page >= pages"
          @click="setQuery({ p: page + 1 })"
          >{{ t("next") }}</Button
        >
      </div>
    </div>
  </section>
  <p class="help" style="margin-top: 14px">{{ t("localTime") }}</p>
  <Dialog :open="editor" @update:open="closeEditor"
    ><DialogContent class="form-modal"
      ><DialogTitle
        >{{ t(original ? "edit" : "create") }} · {{ t(kind) }}</DialogTitle
      ><DialogDescription>{{
        t(kind === "tokens" ? "tokenHelp" : `${kind}Desc`)
      }}</DialogDescription>
      <form @submit.prevent="save">
        <div class="form-grid">
          <FormField
            v-for="field in config.fields.filter((f) => !f.advanced)"
            :key="field.key"
            v-model="draft[field.key]!"
            :field="field"
            :disabled="
              busy ||
              (field.key === 'type' && Boolean(original)) ||
              (field.key === 'remain_quota' &&
                Boolean(draft.unlimited_quota)) ||
              (kind === 'redemptions' &&
                field.key === 'code' &&
                Number(draft.count) > 1)
            "
          />
          <div v-if="balanceChanged" class="balance-review">
            <p>
              {{ t("before") }}:
              {{ quota(original?.[balanceKey], ui.locale, ui.unit) }} →
              {{ t("after") }}:
              {{
                quota(
                  draft.unlimited_quota ? 0 : Number(draft[balanceKey]),
                  ui.locale,
                  ui.unit,
                )
              }}
            </p>
            <p>
              {{ t("balanceDifference") }}:
              {{
                quota(
                  (draft.unlimited_quota ? 0 : Number(draft[balanceKey])) -
                    Number(original?.[balanceKey]),
                  ui.locale,
                  ui.unit,
                )
              }}
            </p>
            <p>{{ t("balanceHint") }}</p>
            <label class="check-field"
              ><input v-model="balanceConfirmed" type="checkbox" />{{
                t("balanceConfirm")
              }}</label
            >
          </div>
        </div>
        <details
          v-if="config.fields.some((f) => f.advanced)"
          class="details-panel"
        >
          <summary>{{ t("advanced") }}</summary>
          <div class="form-grid">
            <FormField
              v-for="field in config.fields.filter((f) => f.advanced)"
              :key="field.key"
              v-model="draft[field.key]!"
              :field="field"
              :disabled="busy"
            />
          </div>
        </details>
        <p
          v-if="original && kind === 'channels'"
          class="help"
          style="margin-top: 15px"
        >
          {{ t("keyReplace") }}
        </p>
        <p v-if="formError" class="error-message section-gap" role="alert">
          {{ formError }}
        </p>
        <div class="form-footer">
          <Button
            type="button"
            variant="outline"
            :disabled="busy"
            @click="closeEditor(false)"
            >{{ t("cancel") }}</Button
          ><Button
            type="submit"
            :disabled="busy || (balanceChanged && !balanceConfirmed)"
            >{{ t(busy ? "saving" : "save") }}</Button
          >
        </div>
      </form></DialogContent
    ></Dialog
  >
  <DiscardDialog :open="discardOpen" @answer="answer" />
  <Dialog
    :open="Boolean(secret)"
    @update:open="
      (open) => {
        if (!open) secret = '';
      }
    "
    ><DialogContent
      ><DialogTitle>{{
        t(kind === "tokens" ? "secretTitle" : "redemptions")
      }}</DialogTitle
      ><DialogDescription>{{
        t(kind === "tokens" ? "secretHelp" : "redemptionsDesc")
      }}</DialogDescription>
      <pre class="secret">{{ secret }}</pre>
      <div class="form-footer">
        <Button variant="outline" @click="secret = ''">{{ t("close") }}</Button
        ><Button @click="copyText(secret)"
          ><Copy :size="15" />{{ t("copy") }}</Button
        >
      </div></DialogContent
    ></Dialog
  >
  <Dialog
    :open="Boolean(deletion)"
    @update:open="
      (open) => {
        if (!open && !busy) deletion = undefined;
      }
    "
    ><DialogContent
      ><DialogTitle>{{ t("confirmDelete") }}</DialogTitle
      ><DialogDescription
        >{{ deletion?.name ?? deletion?.id }} ·
        {{ t("deleteDesc") }}</DialogDescription
      >
      <p v-if="deleteError" class="error-message">{{ deleteError }}</p>
      <div class="form-footer">
        <Button
          variant="outline"
          :disabled="busy"
          @click="deletion = undefined"
          >{{ t("cancel") }}</Button
        ><Button variant="destructive" :disabled="busy" @click="deleteRow">{{
          t("delete")
        }}</Button>
      </div></DialogContent
    ></Dialog
  >
  <Dialog
    :open="Boolean(probing)"
    @update:open="
      (open) => {
        if (!open && !probeBusy) probing = undefined;
      }
    "
    ><DialogContent
      ><DialogTitle>{{ t("probe") }} · {{ probing?.name }}</DialogTitle
      ><DialogDescription>{{ t("probeHelp") }}</DialogDescription
      ><label for="probe-model">{{ t("model") }}</label
      ><select id="probe-model" v-model="probeModel">
        <option
          v-for="model in String(probing?.models ?? '').split(',')"
          :key="model"
        >
          {{ model }}
        </option>
      </select>
      <div class="form-footer">
        <Button
          variant="outline"
          :disabled="probeBusy"
          @click="probing = undefined"
          >{{ t("cancel") }}</Button
        ><Button :disabled="probeBusy || !probeModel" @click="testChannel">{{
          t(probeBusy ? "loading" : "probe")
        }}</Button>
      </div></DialogContent
    ></Dialog
  >
</template>
