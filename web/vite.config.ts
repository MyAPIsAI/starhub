import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv, searchForWorkspaceRoot } from "vite";
import react from "@vitejs/plugin-react";
import { configDefaults } from "vitest/config";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, import.meta.dirname, "STARHUB_");
  const apiTarget = env.STARHUB_API_URL?.trim() || "http://127.0.0.1:3000";

  return {
    plugins: [react()],
    server: {
      fs: {
        allow: [
          searchForWorkspaceRoot(import.meta.dirname),
          fileURLToPath(new URL("../plugins/tasks", import.meta.url)),
        ],
      },
      port: 5173,
      strictPort: true,
      proxy: {
        "/api": apiTarget,
        "/v1": apiTarget,
      },
    },
    build: {
      sourcemap: true,
      rollupOptions: {
        output: {
          manualChunks: { charts: ["recharts"], motion: ["framer-motion"] },
        },
      },
    },
    test: {
      exclude: [...configDefaults.exclude, "test-results/**"],
      coverage: {
        provider: "v8",
        reporter: ["text", "json", "html"],
        include: ["src/**/*.{ts,tsx}"],
        exclude: [
          "src/**/*.test.{ts,tsx}",
          "src/**/*.d.ts",
          "src/main.tsx",
          "src/vite-env.d.ts",
        ],
      },
    },
  };
});
