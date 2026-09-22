<script setup lang="ts">
import { computed, ref, defineAsyncComponent, watch } from "vue";
import { useQuery } from "@tanstack/vue-query";
import { useDocumentVisibility } from "@vueuse/core";
import { useI18n } from "vue-i18n";
import {
  Wallet,
  ArrowUpRight,
  ReceiptText,
  Activity,
  Coins,
  RefreshCw,
} from "lucide-vue-next";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import { number, quota, date, fillDays } from "@/lib/format";
import PageHeading from "@/components/layout/PageHeading.vue";
import AsyncState from "@/components/states/AsyncState.vue";
import { Button } from "@/components/ui/button";
import * as data from "./api";
const TrendChart = defineAsyncComponent(() => import("./TrendChart.vue"));
const auth = useAuth();
const ui = useUI();
const { t } = useI18n();
const days = ref(7);
const scope = ref("personal");
const visibility = useDocumentVisibility();
const admin = computed(() => (auth.user?.role ?? 0) >= 10);
const self = useQuery({
  queryKey: computed(() => [auth.user?.id, "self"]),
  queryFn: ({ signal }) => data.self(signal),
});
watch(self.data, (value) => {
  if (value && auth.user?.id === value.id) auth.user = value;
});
const usage = useQuery({
  queryKey: computed(() => [auth.user?.id, "usage", days.value, scope.value]),
  queryFn: ({ signal }) =>
    data.usage(days.value, scope.value === "global", signal),
});
const metrics = useQuery({
  queryKey: computed(() => [auth.user?.id, "metrics"]),
  enabled: admin,
  queryFn: ({ signal }) => data.metrics(signal),
  refetchInterval: computed(() =>
    visibility.value === "visible" ? 30000 : false,
  ),
});
const channels = useQuery({
  queryKey: computed(() => [auth.user?.id, "channels"]),
  enabled: admin,
  queryFn: ({ signal }) => data.channels(signal),
});
const chartItems = computed(() =>
  fillDays(usage.data.value?.items ?? [], days.value),
);
const totals = computed(() =>
  (usage.data.value?.items ?? []).reduce(
    (sum, row) => ({
      quota: sum.quota + row.quota,
      requests: sum.requests + row.requests,
      unfunded: sum.unfunded + row.unfunded_quota,
      tokens: sum.tokens + row.prompt_tokens + row.completion_tokens,
    }),
    { quota: 0, requests: 0, unfunded: 0, tokens: 0 },
  ),
);
const stats = computed(() =>
  (metrics.data.value?.channel_stats ?? [])
    .slice()
    .sort((a, b) => a.channel_id - b.channel_id),
);
const channelNames = computed(
  () => new Map((channels.data.value ?? []).map((c) => [c.id, c.name])),
);
function refresh() {
  void self.refetch();
  void usage.refetch();
  if (admin.value) {
    void metrics.refetch();
    void channels.refetch();
  }
}
</script>
<template>
  <PageHeading :title="t('greeting')" :description="t('overviewDesc')"
    ><Button variant="outline" @click="refresh"
      ><RefreshCw :size="15" />{{ t("refresh") }}</Button
    ><Button as-child
      ><RouterLink to="/console/tokens"
        >{{ t("tokens") }}<ArrowUpRight :size="15" /></RouterLink></Button
  ></PageHeading>
  <div v-if="auth.user?.quota === 0" class="notice warning">
    {{ t("zeroBalance") }} · {{ t("zeroHelp") }}
    <RouterLink to="/console/profile">{{ t("redeemTitle") }} →</RouterLink>
  </div>
  <div class="dashboard-scope section-gap">
    <select v-if="admin" v-model="scope" :aria-label="t('user_id')">
      <option value="personal">{{ t("personal") }}</option>
      <option value="global">{{ t("global") }}</option></select
    ><select v-model.number="days" :aria-label="t('utc')">
      <option :value="7">{{ t("sevenDays") }}</option>
      <option :value="30">{{ t("thirtyDays") }}</option>
      <option :value="90">{{ t("ninetyDays") }}</option></select
    ><select v-model="ui.unit" :aria-label="t('unit')">
      <option value="usd">USD</option>
      <option value="quota">Quota</option>
    </select>
  </div>
  <p
    v-if="usage.isError.value && usage.data.value"
    class="notice warning section-gap"
  >
    {{ t("stale") }}
  </p>
  <AsyncState
    :loading="usage.isPending.value"
    :error="!usage.data.value && usage.error.value"
    @retry="usage.refetch()"
  >
    <div class="stats-grid">
      <article class="panel stat-card featured">
        <p class="stat-label">
          <Wallet :size="16" />{{
            t(scope === "personal" ? "balance" : "charged")
          }}
        </p>
        <p class="stat-value">
          {{
            quota(
              scope === "personal" ? auth.user?.quota : totals.quota,
              ui.locale,
              ui.unit,
            )
          }}
        </p>
        <p class="stat-caption">
          {{ t(scope === "personal" ? "personal" : "global") }}
        </p>
      </article>
      <article class="panel stat-card">
        <p class="stat-label"><ReceiptText :size="16" />{{ t("requests") }}</p>
        <p class="stat-value">{{ number(totals.requests, ui.locale) }}</p>
        <p class="stat-caption">{{ days }} · {{ t("utc") }}</p>
      </article>
      <article class="panel stat-card">
        <p class="stat-label"><Activity :size="16" />{{ t("totalTokens") }}</p>
        <p class="stat-value">{{ number(totals.tokens, ui.locale) }}</p>
        <p class="stat-caption">
          {{ t("prompt_tokens") }} + {{ t("completion_tokens") }}
        </p>
      </article>
      <article class="panel stat-card">
        <p class="stat-label"><Coins :size="16" />{{ t("unfunded_quota") }}</p>
        <p class="stat-value">
          {{ quota(totals.unfunded, ui.locale, ui.unit) }}
        </p>
        <p class="stat-caption">{{ t("collectionNote") }}</p>
      </article>
    </div>
    <section class="panel">
      <div class="panel-header">
        <div>
          <h2>{{ t("spendTrend") }}</h2>
          <p class="help">
            {{ t("utc") }} · {{ t("charged") }} / {{ t("unfunded_quota") }}
          </p>
        </div>
        <RouterLink class="text-button" to="/console/logs"
          >{{ t("viewLogs") }} ↗</RouterLink
        >
      </div>
      <div class="panel-body">
        <TrendChart :items="chartItems" />
        <details class="details-panel">
          <summary>{{ t("records") }}</summary>
          <div class="table-wrap">
            <table class="data-table">
              <thead>
                <tr>
                  <th>{{ t("utc") }}</th>
                  <th>{{ t("requests") }}</th>
                  <th>{{ t("charged") }}</th>
                  <th>{{ t("unfunded_quota") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in chartItems" :key="item.date">
                  <td>{{ item.date }}</td>
                  <td>{{ number(item.requests, ui.locale) }}</td>
                  <td>{{ quota(item.quota, ui.locale, ui.unit) }}</td>
                  <td>{{ quota(item.unfunded_quota, ui.locale, ui.unit) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </details>
      </div>
    </section>
  </AsyncState>
  <section v-if="admin" class="panel section-gap">
    <div class="panel-header">
      <div>
        <h2>{{ t("runtime") }}</h2>
        <p class="help">
          {{ t("runtimeHelp", { window: metrics.data.value?.window ?? 100 }) }}
        </p>
      </div>
      <span class="help"
        >{{ t("lastUpdated") }}
        {{ date(metrics.data.value?.generated_at, ui.locale) }}</span
      >
    </div>
    <p
      v-if="metrics.isError.value && metrics.data.value"
      class="notice warning"
    >
      {{ t("stale") }}
    </p>
    <AsyncState
      :loading="metrics.isPending.value"
      :error="!metrics.data.value && metrics.error.value"
      @retry="metrics.refetch()"
      ><div class="metric-outcomes" :aria-label="t('metricsCounts')">
        <div v-for="item in metrics.data.value?.requests" :key="item.class">
          <span class="muted">{{ t(item.class) }}</span
          ><strong>{{ number(item.count, ui.locale) }}</strong>
        </div>
      </div>
      <AsyncState :empty="stats.length === 0"
        ><div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ t("channels") }}</th>
                <th>{{ t("samples") }}</th>
                <th>{{ t("successRate") }}</th>
                <th>{{ t("avgTTFT") }}</th>
                <th>{{ t("p95TTFT") }}</th>
                <th>{{ t("latency") }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="stat in stats" :key="stat.channel_id">
                <td>
                  {{
                    channelNames.get(stat.channel_id) ?? `#${stat.channel_id}`
                  }}
                </td>
                <td>{{ stat.samples }}</td>
                <td>
                  <span
                    class="badge"
                    :class="stat.success_rate >= 0.95 ? 'success' : 'warning'"
                    >{{
                      stat.samples
                        ? `${(stat.success_rate * 100).toFixed(1)}%`
                        : t("noSamples")
                    }}</span
                  >
                </td>
                <td>{{ stat.avg_ttft_ms.toFixed(1) }} ms</td>
                <td>{{ stat.p95_ttft_ms.toFixed(1) }} ms</td>
                <td>{{ stat.avg_latency_ms.toFixed(1) }} ms</td>
              </tr>
            </tbody>
          </table>
        </div></AsyncState
      >
      <p class="help panel-body">{{ t("ttftHelp") }}</p></AsyncState
    >
  </section>
</template>
