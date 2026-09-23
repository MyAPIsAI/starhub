<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import {
  Orbit,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  LogOut,
  ChevronRight,
} from "lucide-vue-next";
import { toast } from "vue-sonner";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import { errorMessage } from "@/lib/http";
import NavLinks from "./NavLinks.vue";
const auth = useAuth();
const ui = useUI();
const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const mobile = ref(false);
const leaving = ref(false);
const section = computed(() => route.path.split("/")[2] ?? "console");
const role = computed(() =>
  (auth.user?.role ?? 0) >= 100
    ? "roleRoot"
    : (auth.user?.role ?? 0) >= 10
      ? "roleAdmin"
      : "roleUser",
);
async function logout() {
  leaving.value = true;
  try {
    await auth.logout();
    await router.replace("/login");
  } catch (e) {
    toast.error(errorMessage(e));
  } finally {
    leaving.value = false;
  }
}
</script>
<template>
  <div class="app-shell" :class="{ collapsed: ui.collapsed }">
    <aside class="sidebar">
      <RouterLink class="brand" to="/console/overview"
        ><span class="brand-icon"><Orbit :size="23" /></span
        ><span v-if="!ui.collapsed"
          >StarHub<small>MODEL GATEWAY</small></span
        ></RouterLink
      ><NavLinks :collapsed="ui.collapsed" />
      <div class="sidebar-bottom">
        <span v-if="!ui.collapsed" class="sidebar-note">{{ t("tagline") }}</span
        ><button
          class="icon-button"
          :aria-label="t(ui.collapsed ? 'expand' : 'collapse')"
          @click="ui.collapsed = !ui.collapsed"
        >
          <PanelLeftOpen v-if="ui.collapsed" :size="18" /><PanelLeftClose
            v-else
            :size="18"
          />
        </button>
      </div>
    </aside>
    <div class="workspace">
      <header class="app-header">
        <button
          class="icon-button mobile-toggle"
          :aria-label="t('openMenu')"
          @click="mobile = true"
        >
          <Menu :size="20" />
        </button>
        <div class="breadcrumb">
          <span>StarHub</span><ChevronRight :size="13" /><strong>{{
            t(section)
          }}</strong>
        </div>
        <div class="header-controls">
          <select
            v-model="ui.locale"
            :aria-label="t('language')"
            class="small-select"
          >
            <option value="zh-CN">简体中文</option>
            <option value="en-US">English</option>
          </select>
          <select
            v-model="ui.theme"
            :aria-label="t('theme')"
            class="small-select"
          >
            <option value="system">{{ t("system") }}</option>
            <option value="light">{{ t("light") }}</option>
            <option value="dark">{{ t("dark") }}</option>
          </select>
          <div class="account-chip">
            <span class="avatar">{{
              auth.user?.username.slice(0, 1).toUpperCase()
            }}</span
            ><span class="account-name"
              >{{ auth.user?.username }}<small>{{ t(role) }}</small></span
            >
          </div>
          <button
            class="icon-button"
            :disabled="leaving"
            :aria-label="t('logout')"
            @click="logout"
          >
            <LogOut :size="17" />
          </button>
        </div>
      </header>
      <main id="main-content" class="main-content"><slot /></main>
      <footer class="app-footer">
        <span>StarHub</span><span>{{ t("currencyNote") }}</span>
      </footer>
    </div>
  </div>
  <Dialog v-model:open="mobile"
    ><DialogContent class="mobile-nav"
      ><DialogTitle>StarHub</DialogTitle
      ><DialogDescription>{{ t("navigation") }}</DialogDescription
      ><NavLinks @navigate="mobile = false" /></DialogContent
  ></Dialog>
</template>
