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
import { DemoControlBar } from "./components/demo-control-bar";
import { useSystemAccess } from "./lib/system-access";

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
    queries: { staleTime: 60000, retry: false, refetchOnWindowFocus: false },
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
  if (role !== "superAdmin") return <Navigate to="/console" replace />;
  return <>{children}</>;
}

function App() {
  const { language, theme } = usePreferences();

  useEffect(() => {
    document.documentElement.lang = language;
    document.documentElement.dir = language === "ar" ? "rtl" : "ltr";
    document.documentElement.dataset.theme = theme;
    void i18n.changeLanguage(language);
  }, [language, theme]);

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
                <Route path="/console/*" element={<ConsolePreview />} />
                <Route path="/preview/console/*" element={<ConsolePreview />} />

                {/* Operations back office (M9): reachable only as the system admin. */}
                <Route
                  path="/admin/*"
                  element={
                    <AdminOnly>
                      <AdminPortal />
                    </AdminOnly>
                  }
                />

                {/* Design System */}
                <Route path="/design-system" element={<DesignSystem />} />

                {/* Catch-all */}
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Suspense>

            {/* Global floating Demo Control Panel */}
            <DemoControlBar />
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
      <App />
    </QueryClientProvider>
  </React.StrictMode>,
);
