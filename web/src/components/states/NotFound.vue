<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { Button } from "@/components/ui/button";
defineProps<{ forbidden?: boolean }>();
const { t } = useI18n();
</script>
<template>
  <div class="state panel">
    <span class="error-code">{{ forbidden ? "403" : "404" }}</span>
    <h1>{{ t(forbidden ? "forbidden" : "notFound") }}</h1>
    <p>{{ t(forbidden ? "forbiddenHelp" : "notFoundHelp") }}</p>
    <Button as-child
      ><RouterLink to="/console/overview">{{
        t("backHome")
      }}</RouterLink></Button
    >
  </div>
</template>
