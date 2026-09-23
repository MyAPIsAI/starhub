import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    proxy: {
      "/api": {
        target: process.env.STARHUB_API_URL || "http://127.0.0.1:3000",
        changeOrigin: false,
      },
    },
  },
  build: { sourcemap: false },
  test: {
    environment: "jsdom",
    include: ["tests/unit/**/*.test.ts"],
    restoreMocks: true,
  },
});
