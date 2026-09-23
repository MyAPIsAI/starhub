<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";
import { RouterView, useRouter } from "vue-router";
import { Toaster } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import AppLayout from "@/components/layout/AppLayout.vue";
import { Button } from "@/components/ui/button";
const auth = useAuth();
const ui = useUI();
const router = useRouter();
const { t } = useI18n();
let channel: BroadcastChannel | undefined;
function expire() {
  auth.clear();
  void router.replace("/login");
}
onMounted(() => {
  window.addEventListener("starhub:unauthorized", expire);
  try {
    channel = new BroadcastChannel("starhub-session");
    channel.onmessage = (e) => {
      if (e.data === "logout") expire();
    };
  } catch {
    /* Unsupported browser. */
  }
});
onUnmounted(() => {
  window.removeEventListener("starhub:unauthorized", expire);
  channel?.close();
});
async function retry() {
  await auth.bootstrap();
  await router.replace(router.currentRoute.value.fullPath);
}
</script>
<template>
  <div v-if="auth.failure" class="center-screen">
    <div class="panel state">
      <h1>{{ t("sessionFailure") }}</h1>
      <p>{{ auth.failure }}</p>
      <Button @click="retry">{{ t("retry") }}</Button>
    </div>
  </div>
  <AppLayout v-else-if="auth.user" :key="auth.user.id"
    ><RouterView :key="$route.path"
  /></AppLayout>
  <RouterView v-else :key="$route.path" />
  <Toaster
    :theme="
      ui.theme === 'system' ? 'system' : ui.theme === 'dark' ? 'dark' : 'light'
    "
    rich-colors
    position="top-right"
  />
</template>
