import { defineStore } from "pinia";
import { ref } from "vue";
import {
  api,
  ApiError,
  errorMessage,
  invalidateSessionRequests,
} from "@/lib/http";
import { queryClient } from "@/lib/query";
import type { User } from "@/lib/types";

export const useAuth = defineStore("auth", () => {
  const user = ref<User | null>(null);
  const ready = ref(false);
  const failure = ref("");
  let pending: Promise<void> | undefined;
  let revision = 0;
  async function refresh() {
    const current = revision;
    const next = await api<User>("/user/self", { quiet401: true });
    if (current === revision) user.value = next;
  }
  async function bootstrap() {
    if (ready.value) return;
    if (!pending)
      pending = (async () => {
        failure.value = "";
        try {
          await refresh();
          ready.value = true;
        } catch (e) {
          if (e instanceof ApiError && e.status === 401) {
            user.value = null;
            ready.value = true;
          } else failure.value = errorMessage(e);
        } finally {
          pending = undefined;
        }
      })();
    await pending;
  }
  function clear() {
    revision++;
    invalidateSessionRequests();
    void queryClient.cancelQueries();
    queryClient.clear();
    user.value = null;
    ready.value = true;
    failure.value = "";
  }
  async function login(username: string, password: string) {
    const result = await api<{ user: User }>("/user/login", {
      method: "POST",
      body: { username, password },
      quiet401: true,
    });
    clear();
    user.value = result.user;
  }
  async function logout() {
    try {
      await api("/user/logout", { method: "POST", quiet401: true });
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 401)) throw e;
    }
    clear();
    try {
      const channel = new BroadcastChannel("starhub-session");
      channel.postMessage("logout");
      channel.close();
    } catch {
      /* Broadcast is optional. */
    }
  }
  return { user, ready, failure, bootstrap, refresh, login, logout, clear };
});
