import { ref, onMounted, onUnmounted, type Ref } from "vue";
import { onBeforeRouteLeave, onBeforeRouteUpdate } from "vue-router";
export function useUnsaved(dirty: Ref<boolean>) {
  const discardOpen = ref(false);
  let settle: ((value: boolean) => void) | undefined;
  function ask() {
    if (!dirty.value) return Promise.resolve(true);
    discardOpen.value = true;
    return new Promise<boolean>((resolve) => {
      settle = resolve;
    });
  }
  function answer(value: boolean) {
    discardOpen.value = false;
    settle?.(value);
    settle = undefined;
  }
  function unload(event: BeforeUnloadEvent) {
    if (dirty.value) {
      event.preventDefault();
      event.returnValue = "";
    }
  }
  onBeforeRouteLeave(ask);
  onBeforeRouteUpdate((to, from) => (to.path !== from.path ? ask() : true));
  onMounted(() => window.addEventListener("beforeunload", unload));
  onUnmounted(() => {
    window.removeEventListener("beforeunload", unload);
    settle?.(false);
  });
  return { discardOpen, ask, answer };
}
