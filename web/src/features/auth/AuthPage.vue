<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import {
  Orbit,
  ArrowRight,
  ShieldCheck,
  Layers3,
  Activity,
} from "lucide-vue-next";
import { toast } from "vue-sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import { api, errorMessage } from "@/lib/http";
import { safeRedirect } from "@/router/capabilities";
const route = useRoute();
const router = useRouter();
const auth = useAuth();
const ui = useUI();
const { t } = useI18n();
const registering = computed(() => route.path === "/register");
const username = ref(String(route.query.username ?? ""));
const password = ref("");
const email = ref("");
const invite = ref(String(route.query.aff_code ?? ""));
const busy = ref(false);
const error = ref("");
async function submit() {
  error.value = "";
  if (!username.value || !password.value) {
    error.value = t("required");
    return;
  }
  if (registering.value) {
    if (!/^[A-Za-z0-9_.-]{3,64}$/.test(username.value)) {
      error.value = t("usernameRule");
      return;
    }
    const size = new TextEncoder().encode(password.value).length;
    if (size < 6 || size > 64) {
      error.value = t("passwordRule");
      return;
    }
    if (email.value && !z.email().safeParse(email.value).success) {
      error.value = t("invalidEmail");
      return;
    }
  }
  busy.value = true;
  try {
    if (registering.value) {
      await api("/user/register", {
        method: "POST",
        body: {
          username: username.value,
          password: password.value,
          email: email.value,
          aff_code: invite.value,
        },
      });
      toast.success(t("registered"));
      password.value = "";
      await router.push({
        path: "/login",
        query: { username: username.value },
      });
    } else {
      await auth.login(username.value, password.value);
      password.value = "";
      await router.replace(safeRedirect(route.query.redirect));
    }
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <div class="auth-shell">
    <section class="auth-brand">
      <div class="brand">
        <span class="brand-icon"><Orbit :size="24" /></span><span>StarHub</span>
      </div>
      <div class="auth-story">
        <span class="eyebrow">MODEL GATEWAY / CONSOLE</span>
        <h1>{{ t("tagline") }}</h1>
        <p>{{ t("overviewDesc") }}</p>
        <div class="orbit-art" aria-hidden="true">
          <div class="orbit-ring ring-one"></div>
          <div class="orbit-ring ring-two"></div>
          <div class="orbit-core"><Orbit :size="48" /></div>
          <span class="orbit-node node-one"><Layers3 :size="22" /></span
          ><span class="orbit-node node-two"><Activity :size="22" /></span
          ><span class="orbit-node node-three"><ShieldCheck :size="22" /></span>
        </div>
      </div>
      <span class="auth-footer">StarHub · {{ t("console") }}</span>
    </section>
    <section class="auth-form-area">
      <div class="auth-top">
        <select v-model="ui.locale" :aria-label="t('language')">
          <option value="zh-CN">简体中文</option>
          <option value="en-US">English</option>
        </select>
      </div>
      <form class="auth-form" @submit.prevent="submit">
        <span class="eyebrow">{{ t("workspace") }}</span>
        <h2>{{ t(registering ? "register" : "login") }}</h2>
        <p class="muted">{{ t(registering ? "registerDesc" : "loginDesc") }}</p>
        <label for="username">{{ t("username") }}</label
        ><input
          id="username"
          v-model="username"
          autocomplete="username"
          required
          :maxlength="64"
        />
        <label for="password">{{ t("password") }}</label
        ><input
          id="password"
          v-model="password"
          type="password"
          :autocomplete="registering ? 'new-password' : 'current-password'"
          required
        />
        <template v-if="registering"
          ><label for="email"
            >{{ t("email") }}
            <span class="muted">({{ t("optional") }})</span></label
          ><input
            id="email"
            v-model="email"
            type="email"
            autocomplete="email"
            maxlength="64" /><label for="invite"
            >{{ t("aff_code") }}
            <span class="muted">({{ t("optional") }})</span></label
          ><input id="invite" v-model="invite"
        /></template>
        <p v-if="error" class="error-message" role="alert">{{ error }}</p>
        <Button class="auth-submit" :disabled="busy" type="submit"
          >{{ t(busy ? "loading" : registering ? "register" : "login")
          }}<ArrowRight :size="16"
        /></Button>
        <p class="auth-switch">
          {{ t(registering ? "loginPrompt" : "registerPrompt") }}
          <RouterLink :to="registering ? '/login' : '/register'">{{
            t(registering ? "login" : "register")
          }}</RouterLink>
        </p>
      </form>
      <p class="auth-caption">{{ t("currencyNote") }}</p>
    </section>
  </div>
</template>
