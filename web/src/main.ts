import { createApp } from "vue";
import { createPinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { queryClient } from "@/lib/query";
import { router } from "@/router";
import { i18n } from "@/locales";
import App from "./App.vue";
import "./styles/index.css";
createApp(App)
  .use(createPinia())
  .use(VueQueryPlugin, { queryClient })
  .use(i18n)
  .use(router)
  .mount("#app");
