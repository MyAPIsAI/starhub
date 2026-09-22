import { defineConfig, devices } from "@playwright/test";
import { existsSync } from "node:fs";

const chrome = "C:/Program Files/Google/Chrome/Application/chrome.exe";
export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  workers: 1,
  timeout: 30000,
  expect: { timeout: 8000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:39781",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: existsSync(chrome) ? { executablePath: chrome } : {},
  },
  webServer: {
    command:
      "go test -tags frontend -run ^TestP6BrowserServer$ -count=1 -timeout=15m -v .",
    cwd: "..",
    env: { STARHUB_P6_E2E: "1", STARHUB_COOKIE_SECURE: "false" },
    url: "http://127.0.0.1:39781/login",
    timeout: 120000,
    reuseExistingServer: false,
  },
});
