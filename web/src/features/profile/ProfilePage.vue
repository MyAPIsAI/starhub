<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useClipboard } from "@vueuse/core";
import { toast } from "vue-sonner";
import { UserRound, Ticket, Copy, Wallet } from "lucide-vue-next";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import { quota, date, number } from "@/lib/format";
import { errorMessage } from "@/lib/http";
import { queryClient } from "@/lib/query";
import PageHeading from "@/components/layout/PageHeading.vue";
import { Button } from "@/components/ui/button";
import { redeem } from "./api";
const auth = useAuth();
const ui = useUI();
const { t } = useI18n();
const { copy } = useClipboard();
const code = ref("");
const busy = ref(false);
const error = ref("");
const credited = ref<number>();
const invite = computed(() =>
  auth.user?.aff_code
    ? `${location.origin}/register?aff_code=${encodeURIComponent(auth.user.aff_code)}`
    : "",
);
async function submit() {
  error.value = "";
  credited.value = undefined;
  if (!code.value.trim()) return;
  busy.value = true;
  try {
    const result = await redeem(code.value.trim());
    credited.value = result.quota;
    code.value = "";
    await auth.refresh();
    await queryClient.invalidateQueries({
      predicate: (q) =>
        q.queryKey.includes("self") || q.queryKey.includes("usage"),
    });
  } catch (e) {
    error.value = errorMessage(e);
    try {
      await auth.refresh();
    } catch {
      /* Keep the original redemption error. */
    }
  } finally {
    busy.value = false;
  }
}
async function copyInvite() {
  try {
    await copy(invite.value);
    toast.success(t("copied"));
  } catch {
    toast.error(t("copyFailed"));
  }
}
</script>
<template>
  <PageHeading :title="t('profile')" :description="t('profileDesc')" />
  <div class="profile-grid">
    <section class="panel">
      <div class="panel-header">
        <h2>
          <UserRound :size="18" style="display: inline; margin-right: 8px" />{{
            auth.user?.username
          }}
        </h2>
        <span class="badge">{{
          t(
            (auth.user?.role ?? 0) >= 100
              ? "roleRoot"
              : (auth.user?.role ?? 0) >= 10
                ? "roleAdmin"
                : "roleUser",
          )
        }}</span>
      </div>
      <div class="panel-body">
        <dl class="detail-list">
          <div
            v-for="key in ['email', 'group', 'aff_code', 'inviter_id'] as const"
            :key="key"
            class="detail-row"
          >
            <dt>{{ t(key) }}</dt>
            <dd>{{ auth.user?.[key] || "—" }}</dd>
          </div>
          <div class="detail-row">
            <dt>{{ t("created_at") }}</dt>
            <dd>{{ date(auth.user?.created_at, ui.locale) }}</dd>
          </div>
          <div class="detail-row">
            <dt>{{ t("used_quota") }}</dt>
            <dd>{{ quota(auth.user?.used_quota, ui.locale, ui.unit) }}</dd>
          </div>
          <div class="detail-row">
            <dt>{{ t("requests") }}</dt>
            <dd>{{ number(auth.user?.request_count, ui.locale) }}</dd>
          </div>
        </dl>
      </div>
    </section>
    <div>
      <section class="panel stat-card featured">
        <p class="stat-label"><Wallet :size="17" />{{ t("balance") }}</p>
        <p class="stat-value">
          {{ quota(auth.user?.quota, ui.locale, ui.unit) }}
        </p>
        <p class="stat-caption">{{ t("currencyNote") }}</p>
      </section>
      <section class="panel section-gap">
        <div class="panel-body">
          <h2>
            <Ticket :size="18" style="display: inline; margin-right: 8px" />{{
              t("redeemTitle")
            }}
          </h2>
          <p class="help" style="margin-top: 8px">{{ t("redeemHelp") }}</p>
          <form class="inline-form" @submit.prevent="submit">
            <input
              v-model="code"
              :aria-label="t('code')"
              autocomplete="off"
              required
              placeholder="RD-…"
            /><Button :disabled="busy" type="submit">{{
              t(busy ? "loading" : "redeemAction")
            }}</Button>
          </form>
          <p v-if="error" class="error-message section-gap" role="alert">
            {{ error }}
          </p>
          <p
            v-if="credited !== undefined"
            class="notice section-gap"
            role="status"
          >
            {{ t("credited") }}: {{ quota(credited, ui.locale, ui.unit) }}
          </p>
        </div>
      </section>
    </div>
    <section class="panel">
      <div class="panel-body">
        <h2>{{ t("inviteTitle") }}</h2>
        <p class="help" style="margin-top: 8px">{{ t("inviteHelp") }}</p>
        <div v-if="invite" class="inline-form">
          <input :value="invite" :aria-label="t('aff_code')" readonly /><Button
            variant="outline"
            @click="copyInvite"
            ><Copy :size="15" />{{ t("copy") }}</Button
          >
        </div>
        <p v-else class="help section-gap">{{ t("noInvite") }}</p>
      </div>
    </section>
  </div>
</template>
