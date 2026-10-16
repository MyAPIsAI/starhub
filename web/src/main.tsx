import React, { useEffect, useRef, lazy, Suspense } from "react";
import ReactDOM from "react-dom/client";
import {
  BrowserRouter,
  Navigate,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DirectionProvider } from "@radix-ui/react-direction";
import { TooltipProvider } from "@radix-ui/react-tooltip";
import { MotionConfig } from "framer-motion";
import { Toaster } from "sonner";
import "@fontsource-variable/inter";
import "@fontsource/ibm-plex-sans-arabic/400.css";
import "@fontsource/ibm-plex-sans-arabic/500.css";
import "@fontsource/ibm-plex-sans-arabic/600.css";
import "@fontsource/jetbrains-mono/400.css";
import "./tokens/theme.css";
import "./components/system.css";
import "./components/product.css";
import "./components/refinement.css";
import "./components/select-controls.css";
import i18n from "./i18n";
import { usePreferences } from "./lib/preferences";
import { useSystemAccess, isAdmin, ROLE_USER } from "./lib/system-access";
import { useProductPreview } from "./lib/product-preview";
import { onAuthLoss, ApiError } from "./lib/api/http";
import {
  SessionRestoreProvider,
  useSessionRestore,
} from "./lib/session-restore";
import { Button } from "./components/ui/button";

const DesignSystem = lazy(() => import("./pages/design-system"));
const BrandHome = lazy(() => import("./pages/brand-home"));
const ModelSquare = lazy(() => import("./pages/models/index"));
const ModelDetail = lazy(() => import("./pages/models/detail"));
const StatusPage = lazy(() => import("./pages/status"));
const DocsPage = lazy(() => import("./pages/docs/index"));
const ConsolePreview = lazy(() => import("./pages/console-preview"));
const AdminPortal = lazy(() => import("./pages/admin"));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60000,
      // Retry on transient server errors (5xx) or network
      // failures (fetch throws TypeError, not ApiError). Never
      // retry 4xx client errors. (B14)
      retry: (failureCount, error) =>
        (!(error instanceof ApiError) || error.status >= 500) &&
        failureCount < 1,
      refetchOnWindowFocus: false,
    },
  },
});

function RouteContext() {
  const { pathname, hash } = useLocation();
  const language = usePreferences((s) => s.language);
  const previousPath = useRef(pathname);

  useEffect(() => {
    let key = "pHome";
    if (pathname.includes("design-system")) key = "designSystem";
    else if (pathname.includes("service-quality")) key = "navServiceQuality";
    else if (pathname.includes("smart-routing")) key = "navSmartRouting";
    else if (pathname.endsWith("/billing/recharge")) key = "recharge";
    else if (pathname.endsWith("/billing")) key = "billingTitle";
    else if (pathname.includes("console")) key = "pConsole";
    else if (pathname.includes("models")) key = "navModels";
    else if (pathname.includes("status")) key = "navStatus";
    else if (pathname.includes("docs")) key = "navDocs";
    else if (pathname.includes("admin")) key = "navAdmin";
    else if (pathname.includes("login")) key = "navLogin";
    else if (pathname.includes("register")) key = "navRegister";
    else if (pathname.includes("forgot-password")) key = "resetPassword";

    document.title = `${i18n.t("brand", { lng: language })} · ${i18n.t(key, { lng: language })}`;
  }, [pathname, language]);

  useEffect(() => {
    const authPaths = ["/login", "/register", "/forgot-password"];
    const homePaths = ["/", "/preview/home", ...authPaths];
    const keepHomePosition =
      homePaths.includes(pathname) &&
      homePaths.includes(previousPath.current) &&
      (authPaths.includes(pathname) ||
        authPaths.includes(previousPath.current));
    previousPath.current = pathname;
    if (keepHomePosition) return;
    if (hash)
      document
        .getElementById(hash.slice(1))
        ?.scrollIntoView({ behavior: "instant" });
    else window.scrollTo({ top: 0, behavior: "instant" });
  }, [pathname, hash]);

  return null;
}

/**
 * The operations back office is a system-administrator surface, matching how
 * /console gates its SYSTEM SETTINGS section (src/lib/system-access.ts).
 */
function AdminOnly({ children }: { children: React.ReactNode }) {
  const role = useSystemAccess((s) => s.role);
  if (!isAdmin(role)) return <Navigate to="/console" replace />;
  return <>{children}</>;
}

/**
 * RequireAuth gates console routes on login state. During the
 * initial session-restore it shows a loader; once ready,
 * unauthenticated users are redirected to the landing page. (GAP-2)
 */
function RequireAuth({ children }: { children: React.ReactNode }) {
  const { status, retry } = useSessionRestore();
  const language = usePreferences((s) => s.language);
  const isLoggedIn = useProductPreview((s) => s.user.isLoggedIn);
  if (status === "unavailable") {
    const message =
      language === "zh"
        ? "暂时无法确认登录状态，请重试。"
        : language === "ar"
          ? "تعذر التحقق من حالة تسجيل الدخول مؤقتًا. حاول مجددًا."
          : "Unable to check your session. Please retry.";
    return (
      <div className="route-loading" role="alert">
        {message}
        <Button variant="ghost" onClick={retry}>
          {i18n.t("retry")}
        </Button>
      </div>
    );
  }
  if (status === "checking") {
    return (
      <div className="route-loading" role="status">
        {i18n.t("loading")}
      </div>
    );
  }
  if (!isLoggedIn) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function App() {
  const { language, theme } = usePreferences();

  // Allow clearing the demo-auth lockout via ?unlock — visit
  // /login?unlock to reset the client-side auth state. (demoAuth)
  useEffect(() => {
    if (window.location.search.includes("unlock")) {
      try {
        sessionStorage.removeItem("veyra.demo-auth.v2");
      } catch {
        /* Storage may be blocked; continue. */
      }
      window.location.replace("/login");
    }
  }, []);

  useEffect(() => {
    document.documentElement.lang = language;
    document.documentElement.dir = language === "ar" ? "rtl" : "ltr";
    document.documentElement.dataset.theme = theme;
    void i18n.changeLanguage(language);
  }, [language, theme]);

  // Wire up the 401 auth-loss listener: when the HTTP client gets
  // a 401 outside auth pages, silently redirect to /login. (B8/B9)
  useEffect(() => {
    return onAuthLoss(() => {
      useProductPreview.getState().logout();
      useSystemAccess.getState().setRole(ROLE_USER);
      queryClient.clear();
      // Hard redirect: the session is gone, so the full app state
      // must be discarded. This also avoids stale React Query caches.
      if (window.location.pathname !== "/login") {
        window.location.href = "/login";
      }
    });
  }, []);

  return (
    <DirectionProvider dir={language === "ar" ? "rtl" : "ltr"}>
      <TooltipProvider delayDuration={250}>
        <MotionConfig reducedMotion="user">
          <BrowserRouter
            future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
          >
            <RouteContext />
            <Suspense
              fallback={
                <div className="route-loading" role="status">
                  {i18n.t("loading")}
                </div>
              }
            >
              <Routes>
                {/* Public Marketing & Specs */}
                <Route path="/" element={<BrandHome />} />
                <Route path="/preview/home" element={<BrandHome />} />
                <Route path="/models" element={<ModelSquare />} />
                <Route path="/models/:id" element={<ModelDetail />} />
                <Route path="/status" element={<StatusPage />} />
                <Route path="/docs" element={<DocsPage />} />

                {/* Authentication */}
                <Route path="/login" element={<BrandHome />} />
                <Route path="/register" element={<BrandHome />} />
                <Route path="/forgot-password" element={<BrandHome />} />

                {/* Console */}
                <Route
                  path="/service-quality"
                  element={<Navigate to="/console/service-quality" replace />}
                />
                <Route
                  path="/smart-routing"
                  element={<Navigate to="/console/smart-routing" replace />}
                />
                <Route
                  path="/console/*"
                  element={
                    <RequireAuth>
                      <ConsolePreview />
                    </RequireAuth>
                  }
                />
                <Route
                  path="/preview/console/*"
                  element={
                    <RequireAuth>
                      <ConsolePreview />
                    </RequireAuth>
                  }
                />

                {/* Operations back office (M9): reachable only as the system admin. */}
                <Route
                  path="/admin/*"
                  element={
                    <RequireAuth>
                      <AdminOnly>
                        <AdminPortal />
                      </AdminOnly>
                    </RequireAuth>
                  }
                />

                {/* Design System */}
                <Route path="/design-system" element={<DesignSystem />} />

                {/* Catch-all */}
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Suspense>
          </BrowserRouter>

          <Toaster
            theme={theme}
            position={language === "ar" ? "bottom-left" : "bottom-right"}
            dir={language === "ar" ? "rtl" : "ltr"}
            closeButton
            toastOptions={{ className: "xy-toast" }}
          />
        </MotionConfig>
      </TooltipProvider>
    </DirectionProvider>
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <SessionRestoreProvider>
        <App />
      </SessionRestoreProvider>
    </QueryClientProvider>
  </React.StrictMode>,
);
