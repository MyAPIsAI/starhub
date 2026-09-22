import { createRouter, createWebHistory } from "vue-router";
import { useAuth } from "@/stores/auth";
import { canView } from "./capabilities";
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: "/console/overview" },
    { path: "/login", component: () => import("@/features/auth/AuthPage.vue") },
    {
      path: "/register",
      component: () => import("@/features/auth/AuthPage.vue"),
    },
    { path: "/console", redirect: "/console/overview" },
    {
      path: "/console/overview",
      component: () => import("@/features/dashboard/DashboardPage.vue"),
    },
    {
      path: "/console/profile",
      component: () => import("@/features/profile/ProfilePage.vue"),
    },
    {
      path: "/console/options",
      component: () => import("@/features/options/OptionsPage.vue"),
    },
    {
      path: "/console/:section(tokens|channels|users|logs|audit|redemptions)",
      component: () => import("@/features/resources/ResourcePage.vue"),
    },
    {
      path: "/forbidden",
      component: () => import("@/components/states/NotFound.vue"),
      props: { forbidden: true },
    },
    {
      path: "/:pathMatch(.*)*",
      component: () => import("@/components/states/NotFound.vue"),
    },
  ],
});
router.beforeEach(async (to) => {
  const auth = useAuth();
  await auth.bootstrap();
  if (auth.failure) return true;
  const isPublic = ["/login", "/register"].includes(to.path);
  if (!auth.user && !isPublic)
    return { path: "/login", query: { redirect: to.path } };
  if (auth.user && isPublic) return "/console/overview";
  if (auth.user && to.path.startsWith("/console/")) {
    const section = to.path.split("/")[2]!;
    if (
      !canView(auth.user.role, section) &&
      ["channels", "users", "options", "redemptions", "audit"].includes(section)
    )
      return "/forbidden";
  }
});
