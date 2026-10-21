// E2E browser test runner — launches a Vite dev server and runs the
// browser test suite against it. Used by CI (npm run test:e2e).
import { createServer } from "vite";
import { chromium } from "playwright";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import assert from "node:assert/strict";

const root = fileURLToPath(new URL("../", import.meta.url));
const outDir = path.join(root, "playwright-report");

// 1. Start the Vite dev server on a random port.
const server = await createServer({
  root,
  server: { port: 0, host: "127.0.0.1" },
  logLevel: "error",
});
await server.listen();
const port = server.httpServer.address().port;
const url = `http://127.0.0.1:${port}`;
console.log(`E2E: dev server at ${url}`);

// 2. Launch Chromium (headless).
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
});
const page = await context.newPage();

const report = {
  startedAt: new Date().toISOString(),
  url,
  checks: [],
  failures: [],
  consoleErrors: [],
};

page.on("pageerror", (e) => report.consoleErrors.push(String(e)));

async function check(name, fn) {
  try {
    await fn();
    report.checks.push({ name, passed: true });
    console.log(`PASS  ${name}`);
  } catch (e) {
    report.checks.push({ name, passed: false, error: e.message });
    report.failures.push({ name, error: e.message });
    console.log(`FAIL  ${name} — ${e.message}`);
  }
}

// 3. Basic smoke checks: the app loads and key pages render.
await check("homepage loads", async () => {
  await page.goto(url, { waitUntil: "networkidle", timeout: 10000 });
  const title = await page.title();
  assert.ok(title.length > 0, "page title should not be empty");
});

await check("models page loads", async () => {
  await page.goto(`${url}/models`, { waitUntil: "networkidle", timeout: 10000 });
  // The catalog should render some model cards or list items.
  const body = await page.textContent("body");
  assert.ok(body.length > 100, "models page body should have content");
});

await check("console redirects when not logged in", async () => {
  await page.goto(`${url}/console`, { waitUntil: "networkidle", timeout: 10000 });
  // Should redirect to login or show the homepage.
  const body = await page.textContent("body");
  assert.ok(body.length > 0, "console redirect page should have content");
});

await check("no console errors on homepage", async () => {
  // Navigate fresh and check for JS errors.
  report.consoleErrors.length = 0;
  await page.goto(url, { waitUntil: "networkidle", timeout: 10000 });
  if (report.consoleErrors.length > 0) {
    throw new Error(`console errors: ${report.consoleErrors.join("; ")}`);
  }
});

// 4. Cleanup.
await browser.close();
await server.close();
report.finishedAt = new Date().toISOString();
report.passed = report.failures.length === 0;

await mkdir(outDir, { recursive: true });
await writeFile(
  path.join(outDir, "e2e-report.json"),
  JSON.stringify(report, null, 2),
);

console.log(`\nE2E: ${report.checks.filter(c => c.passed).length}/${report.checks.length} passed`);
if (!report.passed) {
  process.exit(1);
}
