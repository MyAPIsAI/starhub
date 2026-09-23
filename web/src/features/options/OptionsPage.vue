<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useQuery } from "@tanstack/vue-query";
import { useI18n } from "vue-i18n";
import { toast } from "vue-sonner";
import { Save, RefreshCw } from "lucide-vue-next";
import { useAuth } from "@/stores/auth";
import { errorMessage } from "@/lib/http";
import { useUnsaved } from "@/composables/useUnsaved";
import PageHeading from "@/components/layout/PageHeading.vue";
import DiscardDialog from "@/components/layout/DiscardDialog.vue";
import AsyncState from "@/components/states/AsyncState.vue";
import { Button } from "@/components/ui/button";
import { serializeOption, optionDraft } from "./validation";
import * as data from "./api";
const { t } = useI18n();
const auth = useAuth();
const tab = ref("settingsPricing");
const drafts = ref<Record<string, string>>({});
const errors = ref<Record<string, string>>({});
const busy = ref<string>();
const reveal = ref(false);
const groups: Record<string, string[]> = {
  settingsPricing: [
    "model_ratio",
    "completion_ratio",
    "model_price",
    "model_price_expr",
    "cache_ratio",
    "audio_ratio",
  ],
  settingsGroups: ["group_ratio", "group_group_ratio"],
  settingsAffiliate: ["aff_rebate_ratio", "aff_register_bonus"],
  settingsAlerts: ["alert_webhook_url", "alert_error_rate"],
};
const query = useQuery({
  queryKey: computed(() => [auth.user?.id, "options"]),
  queryFn: ({ signal }) => data.list(signal),
});
watch(query.data, (rows, previous) => {
  for (const row of rows ?? [])
    if (
      !(row.key in drafts.value) ||
      drafts.value[row.key] ===
        optionDraft(
          row.key,
          previous?.find((item) => item.key === row.key)?.value ?? "",
        )
    )
      drafts.value[row.key] = optionDraft(row.key, row.value);
});
const rows = computed(() =>
  (query.data.value ?? []).filter((o) => groups[tab.value]!.includes(o.key)),
);
const changed = (key: string) =>
  drafts.value[key] !==
  optionDraft(key, query.data.value?.find((o) => o.key === key)?.value ?? "");
const dirty = computed(() => Object.keys(drafts.value).some(changed));
const { discardOpen, answer } = useUnsaved(dirty);
async function save(key: string) {
  errors.value[key] = "";
  let value: string;
  try {
    value = serializeOption(key, drafts.value[key] ?? "");
  } catch {
    errors.value[key] = t("invalidJson");
    return;
  }
  busy.value = key;
  try {
    await data.save(key, value);
    const result = await query.refetch();
    if (result.isError) throw new Error(t("stale"));
    const current = result.data?.find((o) => o.key === key);
    if (current) drafts.value[key] = optionDraft(key, current.value);
    toast.success(t("updated"));
  } catch (e) {
    errors.value[key] = errorMessage(e);
  } finally {
    busy.value = undefined;
  }
}
</script>
<template>
  <PageHeading :title="t('options')" :description="t('optionsDesc')"
    ><Button variant="outline" @click="query.refetch()"
      ><RefreshCw :size="15" />{{ t("refresh") }}</Button
    ></PageHeading
  >
  <p class="notice" style="margin-bottom: 12px">{{ t("optionHelp") }}</p>
  <p class="help" style="margin-bottom: 24px">{{ t("defaultsHelp") }}</p>
  <div class="settings-tabs" role="tablist">
    <Button
      v-for="(_, key) in groups"
      :key="key"
      :variant="tab === key ? 'default' : 'outline'"
      role="tab"
      :aria-selected="tab === key"
      @click="tab = String(key)"
      >{{ t(String(key)) }}</Button
    >
  </div>
  <p v-if="query.isError.value && query.data.value" class="notice warning">
    {{ t("stale") }}
  </p>
  <AsyncState
    :loading="query.isPending.value"
    :error="!query.data.value && query.error.value"
    @retry="query.refetch()"
    ><section
      v-for="option in rows"
      :key="option.key"
      class="panel option-card"
    >
      <div class="panel-header">
        <div class="option-title">
          <h2>{{ t(option.key) }}</h2>
          <span class="mono muted">{{ option.key }}</span
          ><span v-if="changed(option.key)" class="badge warning">{{
            t("draft")
          }}</span>
        </div>
      </div>
      <form class="panel-body option-body" @submit.prevent="save(option.key)">
        <label class="sr-only" :for="`option-${option.key}`">{{
          t(option.key)
        }}</label
        ><template v-if="option.key === 'alert_webhook_url'"
          ><input
            :id="`option-${option.key}`"
            v-model="drafts[option.key]"
            :type="reveal ? 'text' : 'password'"
            autocomplete="off"
          /><button class="text-button" type="button" @click="reveal = !reveal">
            {{ t(reveal ? "hide" : "show") }}
          </button></template
        ><textarea
          v-else-if="option.value.trim().startsWith('{')"
          :id="`option-${option.key}`"
          v-model="drafts[option.key]"
          spellcheck="false"
        /><input
          v-else
          :id="`option-${option.key}`"
          v-model="drafts[option.key]"
          inputmode="decimal"
        />
        <details
          v-if="changed(option.key) && option.key !== 'alert_webhook_url'"
        >
          <summary class="help">{{ t("current") }}</summary>
          <pre class="option-current">{{
            optionDraft(option.key, option.value)
          }}</pre>
        </details>
        <p v-if="errors[option.key]" class="error-message" role="alert">
          {{ errors[option.key] }}
        </p>
        <div class="form-footer">
          <Button
            v-if="changed(option.key)"
            type="button"
            variant="outline"
            @click="drafts[option.key] = optionDraft(option.key, option.value)"
            >{{ t("discard") }}</Button
          ><Button
            type="submit"
            :disabled="Boolean(busy) || !changed(option.key)"
            ><Save :size="15" />{{
              t(busy === option.key ? "saving" : "save")
            }}</Button
          >
        </div>
      </form>
    </section></AsyncState
  ><DiscardDialog :open="discardOpen" @answer="answer" />
</template>
