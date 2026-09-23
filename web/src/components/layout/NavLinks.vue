<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import {
  LayoutDashboard,
  KeyRound,
  ScrollText,
  UserRound,
  Network,
  UsersRound,
  Settings2,
  Ticket,
  ShieldCheck,
} from "lucide-vue-next";
import { sections, canView } from "@/router/capabilities";
import { useAuth } from "@/stores/auth";
defineProps<{ collapsed?: boolean }>();
const emit = defineEmits<{ navigate: [] }>();
const { t } = useI18n();
const auth = useAuth();
const icons = {
  overview: LayoutDashboard,
  tokens: KeyRound,
  logs: ScrollText,
  profile: UserRound,
  channels: Network,
  users: UsersRound,
  options: Settings2,
  redemptions: Ticket,
  audit: ShieldCheck,
};
const visible = computed(() =>
  sections.filter((s) => canView(auth.user?.role ?? 0, s)),
);
</script>
<template>
  <nav :aria-label="t('navigation')" class="nav-links">
    <template v-for="section in visible" :key="section">
      <p
        v-if="section === 'overview' || section === 'channels'"
        class="nav-group"
      >
        {{
          collapsed
            ? "···"
            : t(section === "overview" ? "workspace" : "operations")
        }}
      </p>
      <RouterLink
        :to="`/console/${section}`"
        :title="t(section)"
        @click="emit('navigate')"
        ><component :is="icons[section]" :size="18" /><span v-if="!collapsed">{{
          t(section)
        }}</span></RouterLink
      >
    </template>
  </nav>
</template>
