import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": "http://127.0.0.1:3000",
      "/v1": "http://127.0.0.1:3000",
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
});
