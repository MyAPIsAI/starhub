<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { Inbox, CircleAlert } from "lucide-vue-next";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/http";
defineProps<{
  loading?: boolean;
  error?: unknown;
  empty?: boolean;
  filtered?: boolean;
}>();
defineEmits<{ retry: [] }>();
const { t } = useI18n();
</script>
<template>
  <div
    v-if="loading"
    class="skeleton-list"
    role="status"
    :aria-label="t('loading')"
  >
    <div v-for="n in 5" :key="n" class="skeleton-row"></div>
    <span class="sr-only">{{ t("loading") }}</span>
  </div>
  <div v-else-if="error" class="state" role="alert">
    <CircleAlert :size="28" />
    <h2>{{ t("loadError") }}</h2>
    <p>{{ errorMessage(error) }}</p>
    <Button variant="outline" @click="$emit('retry')">{{ t("retry") }}</Button>
  </div>
  <div v-else-if="empty" class="state">
    <span class="empty-icon"><Inbox :size="28" /></span>
    <h2>{{ t(filtered ? "noResults" : "noData") }}</h2>
    <p>{{ t(filtered ? "noResultsDesc" : "noDataDesc") }}</p>
    <slot name="action" />
  </div>
  <slot v-else />
</template>
